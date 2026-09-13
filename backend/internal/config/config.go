package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port        string
	DatabaseURL string
	JWTSecret   string
	LogLevel    string
	Env         string
	// LLM
	LLMProvider string
	LLMModel    string
	OpenAIKey   string
	OpenAIModel string
	AnthropicKey string
	AnthropicModel string
	GoogleKey   string
	GoogleModel string
	OpenRouterKey string
	OpenRouterModel string
	LocalModel  string
	LocalBaseURL string
	EmbeddingProvider string
	EmbeddingModel    string
	LLMFallbackProvider string
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:        envOr("PORT", "8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		LogLevel:    envOr("LOG_LEVEL", "info"),
		Env:         envOr("ENV", "development"),
		LLMProvider: envOr("LLM_PROVIDER", "stub"),
		LLMModel:    envOr("LLM_MODEL", "openai/gpt-4o-mini"),
		OpenAIKey:   os.Getenv("OPENAI_API_KEY"),
		OpenAIModel: envOr("OPENAI_MODEL", "gpt-4o-mini"),
		AnthropicKey: os.Getenv("ANTHROPIC_API_KEY"),
		AnthropicModel: envOr("ANTHROPIC_MODEL", "claude-3-5-sonnet-20241022"),
		GoogleKey:   os.Getenv("GOOGLE_API_KEY"),
		GoogleModel: envOr("GOOGLE_MODEL", "gemini-1.5-flash"),
		OpenRouterKey: os.Getenv("OPENROUTER_API_KEY"),
		OpenRouterModel: envOr("OPENROUTER_MODEL", "anthropic/claude-3.5-sonnet"),
		LocalModel:  envOr("LOCAL_MODEL", "llama3.1:8b"),
		LocalBaseURL: envOr("LOCAL_BASE_URL", "http://localhost:11434/v1"),
		EmbeddingProvider: envOr("EMBEDDING_PROVIDER", "openrouter"),
		EmbeddingModel:    envOr("EMBEDDING_MODEL", "openai/text-embedding-3-small"),
		LLMFallbackProvider: os.Getenv("LLM_FALLBACK_PROVIDER"),
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required (min 32 chars)")
	}
	if len(cfg.JWTSecret) < 32 && cfg.Env == "production" {
		return nil, fmt.Errorf("JWT_SECRET must be >=32 chars in production")
	}
	// allow numeric port via env
	if _, err := strconv.Atoi(cfg.Port); err != nil {
		return nil, fmt.Errorf("invalid PORT: %w", err)
	}
	if cfg.Env == "production" {
		hasRealLLM := cfg.OpenRouterKey != "" || cfg.OpenAIKey != "" || cfg.AnthropicKey != "" || cfg.GoogleKey != ""
		if !hasRealLLM && cfg.LLMProvider == "stub" && cfg.LLMFallbackProvider == "" {
			return nil, fmt.Errorf("production requires real LLM provider: set OPENROUTER_API_KEY, OPENAI_API_KEY, or LLM_FALLBACK_PROVIDER (current LLM_PROVIDER=stub)")
		}
		if cfg.EmbeddingProvider == "stub" {
			return nil, fmt.Errorf("production requires real embedding provider: set EMBEDDING_PROVIDER to openrouter/openai (current stub)")
		}
		if cfg.EmbeddingProvider == "openrouter" && cfg.OpenRouterKey == "" && cfg.OpenAIKey == "" {
			return nil, fmt.Errorf("production embedding provider %s requires OPENROUTER_API_KEY or OPENAI_API_KEY", cfg.EmbeddingProvider)
		}
	}
	return cfg, nil
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
