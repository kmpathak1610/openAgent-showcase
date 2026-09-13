package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"openagent/internal/http/middleware"
	"openagent/internal/repository"
	"openagent/internal/security"
	"openagent/internal/tool"
)

type ToolHandler struct {
	repo     *repository.DB
	registry *tool.Registry
	executor *tool.Executor
}

func NewToolHandler(repo *repository.DB, reg *tool.Registry, exec *tool.Executor) *ToolHandler {
	return &ToolHandler{repo: repo, registry: reg, executor: exec}
}

func (h *ToolHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	tools, err := h.registry.List(claims.OrganizationID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if tools == nil { writeData(w, 200, []any{}, nil); return }
	writeData(w, 200, tools, nil)
}

func (h *ToolHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	name := chi.URLParam(r, "name")
	tl, err := h.registry.Get(&claims.OrganizationID, name)
	if err != nil {
		// try global
		tl, err = h.registry.Get(nil, name)
		if err != nil { writeError(w, 404, "NOT_FOUND", "tool not found"); return }
	}
	writeData(w, 200, tl, nil)
}

func (h *ToolHandler) Execute(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	name := chi.URLParam(r, "name")
	var req struct {
		Input          map[string]any `json:"input"`
		TaskID         *string        `json:"taskId"`
		RunID          *string        `json:"runId"`
		AgentID        *string        `json:"agentId"`
		IdempotencyKey string         `json:"idempotencyKey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return
	}
	// Determine agent: if agentId provided, use it, else use a dummy or first agent? For human-initiated, we can use a system agent or require agentId
	var agentID uuid.UUID
	if req.AgentID != nil && *req.AgentID != "" {
		aid, _ := uuid.Parse(*req.AgentID)
		agentID = aid
	} else {
		// try to find an agent in org to attribute
		if agents, err := h.repo.ListAgents(claims.OrganizationID); err == nil && len(agents) >0 {
			agentID = agents[0].ID
		} else {
			writeError(w, 400, "VALIDATION_ERROR", "agentId required"); return
		}
	}
	// Verify agent belongs to org
	if _, err := h.repo.GetAgent(claims.OrganizationID, agentID); err != nil {
		writeError(w, 403, "FORBIDDEN", "agent not in organization"); return
	}
	var taskID *uuid.UUID
	if req.TaskID != nil && *req.TaskID != "" {
		tid, _ := uuid.Parse(*req.TaskID)
		taskID = &tid
	}
	var runID *uuid.UUID
	if req.RunID != nil && *req.RunID != "" {
		rid, _ := uuid.Parse(*req.RunID)
		runID = &rid
	}
	// Prompt injection check on tool input
	if err := security.ValidateToolInput(req.Input); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", "tool input blocked: "+err.Error()); return
	}
	exec, err := h.executor.Execute(r.Context(), tool.ExecuteParams{
		OrganizationID: claims.OrganizationID,
		AgentID:        agentID,
		TaskID:         taskID,
		RunID:          runID,
		ToolName:       name,
		Input:          req.Input,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		// check if approval required
		if exec != nil && exec.Status == "approval_required" {
			writeData(w, 202, map[string]any{"status": "approval_required", "execution": exec, "approvalId": exec.ApprovalID}, nil)
			return
		}
		writeError(w, 400, "VALIDATION_ERROR", err.Error())
		return
	}
	// If execution required approval but was returned as pending, handle
	if exec.Status == "approval_required" {
		writeData(w, 202, exec, nil)
		return
	}
	writeData(w, 200, exec, nil)
}

func (h *ToolHandler) ListExecutions(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	q := r.URL.Query()
	var agentID *uuid.UUID
	if aidStr := q.Get("agentId"); aidStr != "" {
		aid, _ := uuid.Parse(aidStr)
		agentID = &aid
	}
	var taskID *uuid.UUID
	if tidStr := q.Get("taskId"); tidStr != "" {
		tid, _ := uuid.Parse(tidStr)
		taskID = &tid
	}
	execs, err := h.repo.ListToolExecutions(claims.OrganizationID, agentID, taskID, 20)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if execs == nil { writeData(w, 200, []any{}, nil); return }
	writeData(w, 200, execs, nil)
}

func (h *ToolHandler) AssignToAgent(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	// Only owner/admin can grant tools (prevent self-grant)
	if role, ok := h.repo.IsOrgMember(claims.OrganizationID, claims.UserID); !ok || (role != "owner" && role != "admin") {
		writeError(w, 403, "FORBIDDEN", "only owner/admin can assign tools"); return
	}
	agentID, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.repo.GetAgent(claims.OrganizationID, agentID); err != nil {
		writeError(w, 404, "NOT_FOUND", "agent not found"); return
	}
	var req struct{ ToolID string `json:"toolId"`; ToolName string `json:"toolName"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return
	}
	var toolID uuid.UUID
	if req.ToolID != "" {
		tid, _ := uuid.Parse(req.ToolID)
		toolID = tid
	} else if req.ToolName != "" {
		tl, err := h.registry.Get(&claims.OrganizationID, req.ToolName)
		if err != nil {
			tl, err = h.registry.Get(nil, req.ToolName)
			if err != nil { writeError(w, 404, "NOT_FOUND", "tool not found"); return }
		}
		toolID = tl.ID
	} else {
		writeError(w, 400, "VALIDATION_ERROR", "toolId or toolName required"); return
	}
	if err := h.repo.AssignToolToAgent(agentID, toolID); err != nil {
		writeError(w, 500, "INTERNAL", err.Error()); return
	}
	// audit
	_, _ = h.repo.Exec(`INSERT INTO audit_logs (organization_id, actor_type, actor_user_id, action, entity_type, entity_id) VALUES ($1,'user',$2,'tool_assigned',$3,$4)`, claims.OrganizationID, claims.UserID, "agent_tool", agentID)
	writeData(w, 201, map[string]any{"agentId": agentID, "toolId": toolID}, nil)
}
