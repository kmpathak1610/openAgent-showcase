package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"openagent/internal/assistant"
	"openagent/internal/http/middleware"
	"openagent/internal/repository"
)

type AssistantHandler struct {
	repo      *repository.DB
	assistant *assistant.Service
}

func NewAssistantHandler(repo *repository.DB, svc *assistant.Service) *AssistantHandler {
	return &AssistantHandler{repo: repo, assistant: svc}
}

func (h *AssistantHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	ag, err := h.assistant.GetDefaultAssistant(r.Context(), claims.OrganizationID)
	if err != nil {
		// Try to ensure
		ag, err = h.assistant.EnsureDefaultAssistant(r.Context(), claims.OrganizationID, claims.UserID)
		if err != nil {
			writeError(w, 404, "NOT_FOUND", "assistant not found and could not be created: "+err.Error()); return
		}
	}
	writeData(w, 200, ag, nil)
}

func (h *AssistantHandler) Ensure(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	ag, err := h.assistant.EnsureDefaultAssistant(r.Context(), claims.OrganizationID, claims.UserID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, ag, nil)
}

func (h *AssistantHandler) InspectWorkspace(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	// Bounded workspace state for assistant context
	projects, _ := h.repo.ListProjects(claims.OrganizationID)
	agents, _ := h.repo.ListAgents(claims.OrganizationID)
	// teams
	teams, _ := h.repo.ListTeams(claims.OrganizationID)
	// tasks (limit 5) - correct signature: ListTasks(orgID, projectID, channelID, status, assignedAgent, limit)
	tasks, _ := h.repo.ListTasks(claims.OrganizationID, nil, nil, "", nil, 5)
	// browser profiles
	var profiles []map[string]any
	rows, _ := h.repo.Query(`SELECT id, provider, name, status FROM browser_profiles WHERE organization_id=$1 AND owner_user_id=$2 LIMIT 10`, claims.OrganizationID, claims.UserID)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var id, provider, name, status string
			_ = rows.Scan(&id, &provider, &name, &status)
			profiles = append(profiles, map[string]any{"id": id, "provider": provider, "name": name, "status": status})
		}
	}
	if profiles == nil { profiles = []map[string]any{} }
	writeData(w, 200, map[string]any{
		"organizationId": claims.OrganizationID,
		"projects":       projects,
		"agents":         agents,
		"teams":          teams,
		"tasks":          tasks,
		"browserProfiles": profiles,
	}, nil)
}

func (h *AssistantHandler) Onboarding(w http.ResponseWriter, r *http.Request) {
	// Returns suggested actions for new user
	suggested := []map[string]any{
		{"id": "create_agent", "title": "Create my first agent", "description": "Build an AI agent for your workflow"},
		{"id": "build_team", "title": "Build an AI team", "description": "Coordinate multiple agents"},
		{"id": "setup_workflow", "title": "Set up a workflow", "description": "Every Friday research AI news and create posts"},
		{"id": "explore_workspace", "title": "Explore my workspace", "description": "See projects, channels, and capabilities"},
	}
	writeData(w, 200, map[string]any{
		"welcome": "Welcome to OpenAgent. I can help you create AI agents, build teams, create workflows, connect tools, and troubleshoot runs. Tell me what you want to accomplish.",
		"suggestedActions": suggested,
	}, nil)
}

// DiagnoseRequest for troubleshooting
type DiagnoseRequest struct {
	TaskID string `json:"taskId"`
	RunID  string `json:"runId"`
}

func (h *AssistantHandler) Diagnose(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	var req DiagnoseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return
	}
	result := map[string]any{"organizationId": claims.OrganizationID}
	// Inspect task
	if req.TaskID != "" {
		if tid, err := parseUUID(req.TaskID); err == nil {
			if task, err := h.repo.GetTask(claims.OrganizationID, tid); err == nil {
				result["task"] = task
				// inspect run
				if req.RunID != "" {
					if rid, err := parseUUID(req.RunID); err == nil {
						if run, err := h.repo.GetAgentRun(claims.OrganizationID, rid); err == nil {
							result["run"] = run
						}
					}
				} else {
					if runs, err := h.repo.ListAgentRuns(claims.OrganizationID, nil, &tid, 1); err == nil && len(runs) > 0 {
						result["latestRun"] = runs[0]
					}
				}
				// approval
				if task.Status == "approval_required" {
					result["diagnosis"] = "Task requires approval. Check /approvals."
				}
			}
		}
	}
	if result["diagnosis"] == nil {
		result["diagnosis"] = "Inspect task/run/provider/auth as above. Use task events and run actions for root cause."
	}
	writeData(w, 200, result, nil)
}

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}
