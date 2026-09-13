package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/jackc/pgx/v5/stdlib"

	"openagent/internal/auth"
	"openagent/internal/config"
	httprouter "openagent/internal/http"
	"openagent/internal/llm"
	"openagent/internal/logger"
	"openagent/internal/worker"
	"openagent/internal/ws"
)

func main() {
	_ = godotenv.Load()
	_ = godotenv.Load(".env")

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	log := logger.New(cfg.LogLevel)
	log.Info("starting openagent", "env", cfg.Env, "port", cfg.Port)

	// DB (optional at startup — allow running without postgres for frontend dev)
	var db *sql.DB
	if cfg.DatabaseURL != "" {
		db, err = sql.Open("pgx", cfg.DatabaseURL)
		if err != nil {
			log.Error("db open failed", "err", err)
		} else {
			db.SetMaxOpenConns(20)
			db.SetMaxIdleConns(5)
			db.SetConnMaxLifetime(30 * time.Minute)
			db.SetConnMaxIdleTime(5 * time.Minute)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := db.PingContext(ctx); err != nil {
				log.Warn("db ping failed (continuing, /ready will report not ready)", "err", err)
			} else {
				log.Info("db connected")
			}
			cancel()
		}
	}

	authSvc := auth.New(cfg.JWTSecret)

	// LLM registry — provider abstraction (OpenAI, Anthropic, Google, OpenRouter, local, stub)
	llmRegistry := llm.NewRegistry()
	// Register real providers first; stub only for development/test
	isProd := cfg.Env == "production"
	if !isProd {
		llmRegistry.Register(llm.NewStub("openai"))
		llmRegistry.Register(llm.NewStub("anthropic"))
		llmRegistry.Register(llm.NewStub("google"))
		llmRegistry.Register(llm.NewStub("stub"))
		llmRegistry.RegisterEmbedder("stub", llm.NewStubEmbedder(1536))
		llmRegistry.RegisterEmbedder("openai", llm.NewStubEmbedder(1536))
	} else {
		// Production: NEVER register stub silently. Explicit simulation requires ALLOW_STUB_IN_PROD=true
		if cfg.LLMProvider == "stub" {
			log.Error("production stub provider rejected: configure real LLM (OPENROUTER_API_KEY) or set ALLOW_STUB_IN_PROD=true for explicit simulation")
		}
	}

	// OpenRouter (single key for many models) — if key provided, use real provider
	if cfg.OpenRouterKey != "" {
		orProvider := llm.NewOpenRouter(cfg.OpenRouterKey, cfg.OpenRouterModel)
		llmRegistry.Register(orProvider)
		llmRegistry.RegisterEmbedder("openrouter", orProvider)
		log.Info("llm provider configured", "provider", "openrouter", "model", cfg.OpenRouterModel)
	} else if !isProd {
		llmRegistry.Register(llm.NewStub("openrouter"))
		log.Info("llm provider using stub", "provider", cfg.LLMProvider, "model", cfg.LLMModel, "openrouter_model", cfg.OpenRouterModel)
	} else {
		log.Error("production missing OPENROUTER_API_KEY and no LLM_FALLBACK_PROVIDER — readiness will fail")
	}
	// Also register explicit models for other providers if keys set (for future real providers)
	if cfg.OpenAIKey != "" {
		log.Info("openai key configured", "model", cfg.OpenAIModel)
	}
	if cfg.AnthropicKey != "" {
		log.Info("anthropic key configured", "model", cfg.AnthropicModel)
	}
	// Generic LLM_PROVIDER env can override default
	if cfg.LLMProvider != "stub" && cfg.LLMProvider != "" {
		log.Info("generic llm provider", "provider", cfg.LLMProvider, "model", cfg.LLMModel)
	}

	hub := ws.NewHub(log)
	go hub.Run()
	// Redis broadcaster for multi-instance WS (P2-13)
	if rb := ws.NewRedisBroadcaster(hub); rb != nil {
		log.Info("redis broadcaster enabled", "url", os.Getenv("REDIS_URL"))
		hub.SetRedisBroadcaster(rb)
		defer rb.Close()
	} else {
		log.Info("redis not configured, using single-instance hub")
	}

	workerPool := worker.New(256, log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbPing := func() error {
		if db == nil {
			return fmt.Errorf("db not configured")
		}
		c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return db.PingContext(c)
	}

	router := httprouter.NewRouter(log, authSvc, hub, db, llmRegistry, workerPool, cfg.JWTSecret, dbPing, cfg.OpenRouterModel, cfg.OpenRouterKey != "", cfg.LLMProvider, cfg.LLMModel)
	workerPool.Start(ctx, 2)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info("listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("listen error", "err", err)
			os.Exit(1)
		}
	}()

	// graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutting down...")
	cancel()
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	_ = srv.Shutdown(shutCtx)
	if db != nil {
		_ = db.Close()
	}
	log.Info("bye")
}
