package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"openagent/internal/guardrail"
	"openagent/internal/http/middleware"
	"openagent/internal/repository"
)

type StatusHandler struct {
	repo      *repository.DB
	guardrail *guardrail.Service
}

func NewStatusHandler(repo *repository.DB, g *guardrail.Service) *StatusHandler {
	return &StatusHandler{repo: repo, guardrail: g}
}

func (h *StatusHandler) AgentStatus(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	agentID, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.repo.GetAgent(claims.OrganizationID, agentID); err != nil {
		writeError(w, 404, "NOT_FOUND", "agent not found"); return
	}
	status, err := h.guardrail.GetAgentStatus(claims.OrganizationID, agentID)
	if err != nil { status = "offline" }
	writeData(w, 200, map[string]any{"agentId": agentID, "status": status}, nil)
}

func (h *StatusHandler) ListStatuses(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	agents, _ := h.repo.ListAgents(claims.OrganizationID)
	var out []map[string]any
	for _, a := range agents {
		status, _ := h.guardrail.GetAgentStatus(claims.OrganizationID, a.ID)
		out = append(out, map[string]any{"agentId": a.ID, "name": a.Name, "status": status, "autonomy": a.AutonomyLevel})
	}
	if out == nil { out = []map[string]any{} }
	writeData(w, 200, out, nil)
}
