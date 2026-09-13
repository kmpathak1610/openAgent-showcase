package http

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"openagent/internal/approval"
	"openagent/internal/auth"
	"openagent/internal/autonomy"
	"openagent/internal/builder"
	"openagent/internal/guardrail"
	"openagent/internal/http/handlers"
	"openagent/internal/http/middleware"
	"openagent/internal/integration"
	"openagent/internal/assistant"
	"openagent/internal/browser"
	"openagent/internal/knowledge"
	"openagent/internal/llm"
	"openagent/internal/memory"
	"openagent/internal/repository"
	"openagent/internal/runtime"
	"openagent/internal/scheduler"
	"openagent/internal/team"
	"openagent/internal/tool"
	"openagent/internal/worker"
	"openagent/internal/ws"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func NewRouter(log *slog.Logger, authSvc *auth.Service, hub *ws.Hub, db *sql.DB, llmRegistry *llm.Registry, workerPool *worker.Pool, jwtSecret string, dbPing func() error, openRouterModel string, openRouterKeySet bool, llmProvider, llmModel string) http.Handler {
	rateLimiter := middleware.NewRateLimiter(10, 20) // 10 req/s burst 20 per IP/org/path
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logging(log))
	r.Use(middleware.Recover)
	r.Use(middleware.CORS())
	r.Use(middleware.RateLimit(rateLimiter))

	r.Get("/health", handlers.Health)
	r.Get("/ready", handlers.Ready(dbPing))
	r.Get("/metrics", handlers.Metrics)
	settingsH := handlers.NewSettingsHandler(openRouterModel, openRouterKeySet, llmProvider, llmModel)
	r.Get("/api/v1/settings/llm", settingsH.GetLLM)

	// if no DB (dev mode without postgres), keep minimal stubs so frontend can still start
	if db == nil {
		authH := handlers.NewAuthHandler(authSvc, nil)
		r.Route("/api/v1", func(api chi.Router) {
			api.Post("/auth/register", authH.Register)
			api.Post("/auth/login", authH.Login)
			api.Group(func(pr chi.Router) {
				pr.Use(middleware.Auth(authSvc))
				pr.Get("/auth/me", authH.Me)
			})
		})
		r.Get("/ws", wsHandler(authSvc, hub))
		return r
	}

	repo := repository.New(db)
	builderSvc := builder.New(llmRegistry)
	// knowledge RAG: embedder from registry — prefer real providers, stub only for dev/test
	var embedder llm.Embedder
	if llmRegistry != nil {
		// Prefer openrouter (real) → openai (real if configured) → stub (dev only)
		if e, err := llmRegistry.GetEmbedder("openrouter"); err == nil {
			embedder = e
			// Verify real provider has credentials in production (check type)
			if eName := "openrouter"; eName == "openrouter" && os.Getenv("ENV") == "production" {
				// If embedder is stub masquerading, fail readiness will catch; just log
			}
		} else if e, err := llmRegistry.GetEmbedder("openai"); err == nil {
			embedder = e
		} else if e, err := llmRegistry.GetEmbedder("stub"); err == nil {
			if os.Getenv("ENV") == "production" {
				log.Error("stub embedder requested in production — refusing, embedding will be unavailable; set OPENROUTER_API_KEY or OPENAI_API_KEY and EMBEDDING_PROVIDER to openrouter/openai")
			} else {
				embedder = e
			}
		}
	}
	if embedder == nil {
		if os.Getenv("ENV") == "production" {
			log.Error("no embedding provider configured for production — set OPENROUTER_API_KEY or EMBEDDING_PROVIDER; RAG embedding will fail until configured (no silent stub fallback)")
			// Do NOT fallback to stub in production — leave nil so ingest fails visibly
		} else {
			embedder = llm.NewStubEmbedder(1536)
		}
	}
	knowledgeSvc := knowledge.NewService(repo, embedder)
	retriever := knowledge.NewRetriever(repo, embedder, nil)
	knowledgeH := handlers.NewKnowledgeHandler(repo, knowledgeSvc, retriever, workerPool)

	// Tool + Integration + Approval
	toolReg := tool.NewRegistry(repo)
	integReg := integration.NewRegistry()
	// Register real LinkedIn provider (production-grade), rest as mocks but real providers can override via env
	integReg.Register(integration.NewLinkedIn())
	for _, name := range []string{"x", "instagram", "facebook", "email", "calendar", "google_drive", "slack", "http_api", "web_search"} {
		integReg.Register(integration.NewMock(name))
	}
	if jwtSecret == "" { jwtSecret = "dev-jwt-secret-change-in-prod-32chars" }
	integSvc := integration.NewService(repo, jwtSecret, integReg)
	approvalSvc := approval.New(repo)
	toolExec := tool.NewExecutor(repo, toolReg, integReg, approvalSvc, jwtSecret)
	// Browser capability foundation
	browserMgr := browser.NewManager(repo)
	toolExec.SetBrowserManager(browserMgr)

	// Phase 7: Memory, Triggers, Scheduling, Guardrails, Autonomy
	memorySvc := memory.New(repo, embedder)
	rt := runtime.NewWithMemoryAndWorker(repo, llmRegistry, retriever, hub, toolExec, approvalSvc, memorySvc, workerPool)
	schedulerSvc := scheduler.New(repo, workerPool)
	guardrailSvc := guardrail.New(repo)
	autonomySvc := autonomy.New(repo, workerPool, hub)
	rt.SetAutonomy(autonomySvc)
	// background scheduler for due triggers every 30s
	if workerPool != nil {
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				if _, err := schedulerSvc.ProcessDueTriggers(context.Background()); err != nil {
					log.Error("scheduler tick failed", "err", err)
				}
			}
		}()
	}
	// background memory consolidation: staleness + periodic duplicate consolidation (bounded batches)
	if workerPool != nil && memorySvc != nil {
		go func() {
			// initial delay
			time.Sleep(10 * time.Second)
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				if n, err := memorySvc.MarkStale(context.Background(), 50); err == nil && n > 0 {
					log.Info("memory staleness marked", "count", n)
				}
				if n, err := memorySvc.BackgroundConsolidation(context.Background(), 20); err == nil && n > 0 {
					log.Info("memory background consolidation", "merged", n)
				}
				if _, err := memorySvc.CleanupExpired(context.Background()); err != nil {
					log.Error("memory cleanup failed", "err", err)
				}
			}
		}()
		// async consolidation job via durable worker (bounded batches)
		workerPool.Register("memory_consolidation", func(ctx context.Context, job worker.Job) error {
			_, _ = memorySvc.BackgroundConsolidation(ctx, 20)
			_, _ = memorySvc.MarkStale(ctx, 50)
			return nil
		})
	}
	memoryH := handlers.NewMemoryHandler(repo, memorySvc)
	triggerH := handlers.NewTriggerHandler(repo, schedulerSvc)
	statusH := handlers.NewStatusHandler(repo, guardrailSvc)
	evalH := handlers.NewEvaluationHandler(repo)
	browserH := handlers.NewBrowserHandler(repo, browserMgr)
	assistantSvc := assistant.New(repo)
	assistantH := handlers.NewAssistantHandler(repo, assistantSvc)
	_ = autonomySvc
	_ = guardrailSvc
	// P1-4: start durable worker if repo available
	if repo != nil && workerPool != nil {
		durable := worker.NewDurable(workerPool, repo)
		go durable.StartDurable(context.Background(), 2)
	}
	// register async jobs
	if workerPool != nil {
		workerPool.Register("ingest_document", func(ctx context.Context, job worker.Job) error {
			docIDStr, _ := job.Payload["document_id"].(string)
			orgIDStr, _ := job.Payload["organization_id"].(string)
			content, _ := job.Payload["content"].(string)
			docID, err := uuid.Parse(docIDStr)
			if err != nil { return err }
			orgID, err := uuid.Parse(orgIDStr)
			if err != nil { return err }
			if err := knowledgeSvc.IngestDocument(ctx, orgID, docID, content); err != nil {
				return err
			}
			// Autonomy: document.ready
			if doc, err := repo.GetDocument(orgID, docID); err == nil && doc.ProjectID != nil {
				go autonomySvc.HandleProjectEvent(context.Background(), orgID, *doc.ProjectID, "document.ready", map[string]any{"documentId": docID.String(), "title": doc.Title})
			}
			return nil
		})
		workerPool.Register("agent_run", func(ctx context.Context, job worker.Job) error {
			orgIDStr, _ := job.Payload["organization_id"].(string)
			agentIDStr, _ := job.Payload["agent_id"].(string)
			taskIDStr, _ := job.Payload["task_id"].(string)
			corrStr, _ := job.Payload["correlation_id"].(string)
			orgID, _ := uuid.Parse(orgIDStr)
			agentID, _ := uuid.Parse(agentIDStr)
			taskID, _ := uuid.Parse(taskIDStr)
			var corrID *uuid.UUID
			if corrStr != "" {
				if cid, err := uuid.Parse(corrStr); err == nil { corrID = &cid }
			}
			return rt.ExecuteTask(ctx, orgID, agentID, taskID, corrID, "task_assigned")
		})
	}

	authH := handlers.NewAuthHandler(authSvc, repo)
	authH.SetAssistant(assistantSvc)
	orgH := handlers.NewOrgHandler(repo)
	projectH := handlers.NewProjectHandler(repo)
	channelH := handlers.NewChannelHandler(repo)
	messageH := handlers.NewMessageHandlerWithAutonomy(repo, hub, autonomySvc, workerPool)
	agentH := handlers.NewAgentHandler(repo, builderSvc)
	taskH := handlers.NewTaskHandlerWithAutonomy(repo, workerPool, hub, autonomySvc)
	runH := handlers.NewRunHandler(repo)
	toolH := handlers.NewToolHandler(repo, toolReg, toolExec)
	integH := handlers.NewIntegrationHandler(integSvc)
	approvalH := handlers.NewApprovalHandlerWithWorker(repo, approvalSvc, toolExec, hub, workerPool)
	teamBuilder := team.New(llmRegistry)
	teamOrchestrator := team.NewOrchestrator(repo, workerPool, hub)
	teamH := handlers.NewTeamHandler(repo, teamBuilder, teamOrchestrator)

	r.Route("/api/v1", func(api chi.Router) {
		api.Post("/auth/register", authH.Register)
		api.Post("/auth/login", authH.Login)

		api.Group(func(pr chi.Router) {
			pr.Use(middleware.Auth(authSvc))
			pr.Get("/auth/me", authH.Me)
			pr.Get("/settings/llm", settingsH.GetLLM)
			pr.Post("/auth/switch", authH.SwitchOrg)
			pr.Post("/auth/logout", authH.Logout)

			// Organizations
			pr.Get("/organizations", orgH.List)
			pr.Post("/organizations", orgH.Create)
			pr.Get("/organizations/{id}", orgH.Get)
			pr.Patch("/organizations/{id}", orgH.Update)
			pr.Get("/organizations/{id}/members", orgH.ListMembers)
			pr.Post("/organizations/{id}/members", orgH.AddMember)
			pr.Delete("/organizations/{id}/members/{userId}", orgH.RemoveMember)

			// Projects
			pr.Get("/projects", projectH.List)
			pr.Post("/projects", projectH.Create)
			pr.Get("/projects/{id}", projectH.Get)
			pr.Patch("/projects/{id}", projectH.Update)
			pr.Post("/projects/{id}/archive", projectH.Archive)
			pr.Get("/projects/{id}/members", projectH.ListMembers)
			pr.Post("/projects/{id}/members", projectH.AddMember)
			pr.Delete("/projects/{id}/members/{userId}", projectH.RemoveMember)
			// Project ↔ Agent assignment (first-class)
			pr.Get("/projects/{id}/agents", projectH.ListAgents)
			pr.Post("/projects/{id}/agents", projectH.AddAgent)
			pr.Delete("/projects/{id}/agents/{agentId}", projectH.RemoveAgent)

			// Channels
			pr.Get("/channels", channelH.List)
			pr.Post("/channels", channelH.Create)
			pr.Get("/channels/{id}", channelH.Get)
			pr.Patch("/channels/{id}", channelH.Update)
			pr.Post("/channels/{id}/rename", channelH.Rename)
			pr.Post("/channels/{id}/archive", channelH.Archive)
			pr.Get("/channels/{id}/members", channelH.ListMembers)
			pr.Post("/channels/{id}/members", channelH.AddMember)
			pr.Delete("/channels/{id}/members/{userId}", channelH.RemoveMember)

			// Messages
			pr.Get("/messages", messageH.List)
			pr.Post("/messages", messageH.Create)
			pr.Get("/messages/{id}", messageH.Get)
			pr.Patch("/messages/{id}", messageH.Update)
			pr.Delete("/messages/{id}", messageH.Delete)
			pr.Get("/messages/{id}/replies", messageH.ListReplies)
			pr.Post("/messages/{id}/reactions", messageH.AddReaction)
			pr.Delete("/messages/{id}/reactions/{emoji}", messageH.RemoveReaction)

			// Agents — Builder + CRUD + Versions
			pr.Post("/agents/builder/preview", agentH.BuilderPreview)
			pr.Post("/agents", agentH.Create)
			pr.Get("/agents", agentH.List)
			pr.Get("/agents/{id}", agentH.Get)
			pr.Put("/agents/{id}", agentH.Update)
			pr.Patch("/agents/{id}", agentH.Update)
			pr.Get("/agents/{id}/versions", agentH.ListVersions)
			pr.Get("/agents/{id}/versions/{version}", agentH.GetVersion)
			pr.Get("/agents/{id}/permissions", agentH.ListPermissions)
			pr.Post("/agents/{id}/permissions", agentH.AddPermission)

			// Knowledge — Sources, Collections, Documents, Search
			pr.Post("/knowledge/sources", knowledgeH.CreateSource)
			pr.Get("/knowledge/sources", knowledgeH.ListSources)
			pr.Post("/knowledge/collections", knowledgeH.CreateCollection)
			pr.Get("/knowledge/collections", knowledgeH.ListCollections)
			pr.Post("/knowledge/documents", knowledgeH.CreateDocument)
			pr.Get("/knowledge/documents", knowledgeH.ListDocuments)
			pr.Get("/knowledge/documents/{id}", knowledgeH.GetDocument)
			pr.Delete("/knowledge/documents/{id}", knowledgeH.DeleteDocument)
			pr.Get("/knowledge/documents/{id}/chunks", knowledgeH.ListDocumentChunks)
			pr.Post("/knowledge/search", knowledgeH.Search)
			// Agent knowledge attach
			pr.Post("/agents/{id}/knowledge", knowledgeH.AttachAgentKnowledge)
			pr.Get("/agents/{id}/knowledge", knowledgeH.ListAgentKnowledge)
			pr.Delete("/agents/{id}/knowledge/{knowledgeId}", knowledgeH.DetachAgentKnowledge)
			// Project knowledge attach
			pr.Post("/projects/{id}/knowledge", knowledgeH.AttachProjectKnowledge)
			pr.Get("/projects/{id}/knowledge", knowledgeH.ListProjectKnowledge)
			pr.Delete("/projects/{id}/knowledge/{knowledgeId}", knowledgeH.DetachProjectKnowledge)

			// Tasks — Phase 4
			pr.Get("/tasks", taskH.List)
			pr.Post("/tasks", taskH.Create)
			pr.Get("/tasks/{id}", taskH.Get)
			pr.Patch("/tasks/{id}", taskH.UpdateStatus)
			pr.Put("/tasks/{id}", taskH.UpdateStatus)
			pr.Post("/tasks/{id}/assign", taskH.Assign)
			pr.Post("/tasks/{id}/delegate", taskH.Delegate)
			pr.Post("/tasks/{id}/cancel", taskH.Cancel)
			pr.Get("/tasks/{id}/events", taskH.ListEvents)
			pr.Get("/tasks/{id}/runs", runH.ListByTask)
			// Runs
			pr.Get("/runs", runH.List)
			pr.Get("/runs/{id}", runH.Get)
			pr.Get("/agents/{id}/runs", runH.ListByAgent)

			// Tools — Phase 5
			pr.Get("/tools", toolH.List)
			pr.Get("/tools/{name}", toolH.Get)
			pr.Post("/tools/{name}/execute", toolH.Execute)
			pr.Get("/tool-executions", toolH.ListExecutions)
			pr.Post("/agents/{id}/tools", toolH.AssignToAgent)
			// Integrations
			pr.Get("/integrations", integH.List)
			pr.Post("/integrations", integH.Create)
			pr.Get("/integrations/{id}", integH.Get)
			pr.Delete("/integrations/{id}", integH.Delete)
			// Approvals
			pr.Get("/approvals", approvalH.List)
			pr.Get("/approvals/{id}", approvalH.Get)
			pr.Post("/approvals/{id}/approve", approvalH.Approve)
			pr.Post("/approvals/{id}/reject", approvalH.Reject)
			pr.Post("/approvals/{id}/cancel", approvalH.Cancel)

			// Teams — Phase 6
			pr.Post("/teams/builder/preview", teamH.BuilderPreview)
			pr.Post("/teams", teamH.Create)
			pr.Get("/teams", teamH.List)
			pr.Get("/teams/{id}", teamH.Get)
			pr.Get("/teams/{id}/members", teamH.ListMembers)
			pr.Post("/teams/{id}/members", teamH.AddMember)
			pr.Delete("/teams/{id}/members/{agentId}", teamH.RemoveMember)
			pr.Post("/teams/{id}/start", teamH.StartTask)
			pr.Get("/teams/{id}/dashboard", teamH.Dashboard)
			pr.Get("/teams/{id}/activity", teamH.ListActivity)
			// Task dependencies
			pr.Post("/tasks/{id}/dependencies", teamH.AddDependency)
			pr.Get("/tasks/{id}/dependencies", teamH.ListDependencies)
			pr.Get("/tasks/{id}/blocked", teamH.IsBlocked)

			// Browser capability (persistent profiles, sessions, audit, tool integration)
			pr.Post("/browser/profiles", browserH.CreateProfile)
			pr.Get("/browser/profiles", browserH.ListProfiles)
			pr.Get("/browser/profiles/{id}", browserH.GetProfile)
			pr.Delete("/browser/profiles/{id}", browserH.DeleteProfile)
			pr.Patch("/browser/profiles/{id}", browserH.UpdateProfileStatus)
			pr.Post("/browser/profiles/{id}/sessions", browserH.CreateSession)
			pr.Get("/browser/sessions", browserH.ListSessions)
			pr.Get("/browser/sessions/{id}", browserH.GetSession)
			pr.Post("/browser/sessions/{id}/close", browserH.CloseSession)
			pr.Get("/browser/audit", browserH.ListAudit)
			// Default Assistant (onboarding, guidance, creation, troubleshooting)
			pr.Get("/assistant", assistantH.Get)
			pr.Post("/assistant/ensure", assistantH.Ensure)
			pr.Get("/assistant/workspace", assistantH.InspectWorkspace)
			pr.Get("/assistant/onboarding", assistantH.Onboarding)
			pr.Post("/assistant/diagnose", assistantH.Diagnose)
			// Phase 7: Memory (consolidation lifecycle), Triggers, Scheduling, Agent Status, Autonomy
			pr.Post("/memories", memoryH.Create)
			pr.Get("/memories", memoryH.List)
			pr.Get("/memories/{id}", memoryH.Get)
			pr.Get("/memories/{id}/versions", memoryH.Versions)
			pr.Post("/memories/{id}/archive", memoryH.Archive)
			pr.Post("/memories/{id}/restore", memoryH.Restore)
			pr.Post("/memories/search", memoryH.Search)
			pr.Delete("/memories/{id}", memoryH.Delete)
			pr.Post("/triggers", triggerH.Create)
			pr.Get("/triggers", triggerH.List)
			pr.Get("/triggers/{id}", triggerH.Get)
			pr.Delete("/triggers/{id}", triggerH.Delete)
			pr.Get("/schedules", triggerH.ListSchedules)
			pr.Get("/agents/{id}/status", statusH.AgentStatus)
			pr.Get("/agents/status", statusH.ListStatuses)
			// P2-11: Task-specific evaluation
			pr.Get("/tasks/{id}/evaluation", evalH.GetEvaluation)
			pr.Post("/tasks/{id}/evaluate", evalH.EvaluateTask)
			pr.Get("/tasks/{id}/metrics", evalH.TaskMetrics)
			pr.Get("/tasks/{id}/evaluation", evalH.EvaluateTask)
			pr.Get("/tasks/{id}/metrics", evalH.TaskMetrics)
		})
	})

	r.Get("/ws", wsHandler(authSvc, hub))
	return r
}

func wsHandler(authSvc *auth.Service, hub *ws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			h := r.Header.Get("Authorization")
			if len(h) > 7 && h[:7] == "Bearer " { token = h[7:] }
		}
		if token == "" { http.Error(w, "missing token", http.StatusUnauthorized); return }
		claims, err := authSvc.VerifyToken(token)
		if err != nil { http.Error(w, "invalid token", http.StatusUnauthorized); return }
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil { return }
		client := &ws.Client{
			ID:             claims.UserID.String() + "-" + r.RemoteAddr,
			OrganizationID: claims.OrganizationID,
			UserID:         claims.UserID,
			Conn:           conn,
			Send:           make(chan ws.Event, 64),
			Hub:            hub,
		}
		hub.Register(client)
		go client.WritePump()
		client.ReadPump()
	}
}
