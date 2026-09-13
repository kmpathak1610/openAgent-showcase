package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"openagent/internal/http/middleware"
	"openagent/internal/memory"
	"openagent/internal/repository"
)

type MemoryHandler struct {
	repo   *repository.DB
	memory *memory.Service
}

func NewMemoryHandler(repo *repository.DB, svc *memory.Service) *MemoryHandler {
	return &MemoryHandler{repo: repo, memory: svc}
}

func (h *MemoryHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	var req struct {
		AgentID        *string        `json:"agentId"`
		ProjectID      *string        `json:"projectId"`
		ConversationID *string        `json:"conversationId"`
		TaskID         *string        `json:"taskId"`
		MemoryType     string         `json:"memoryType"`
		Scope          string         `json:"scope"`
		Source         string         `json:"source"`
		SourceEventID  *string        `json:"sourceEventId"`
		Content        string         `json:"content"`
		Importance     float64        `json:"importance"`
		Confidence     float64        `json:"confidence"`
		Metadata       map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return
	}
	var agentID *uuid.UUID
	if req.AgentID != nil && *req.AgentID != "" {
		uid, _ := uuid.Parse(*req.AgentID)
		if _, err := h.repo.GetAgent(claims.OrganizationID, uid); err != nil {
			writeError(w, 404, "NOT_FOUND", "agent not found"); return
		}
		agentID = &uid
	}
	var projectID *uuid.UUID
	if req.ProjectID != nil && *req.ProjectID != "" {
		uid, _ := uuid.Parse(*req.ProjectID)
		projectID = &uid
	}
	var convID *uuid.UUID
	if req.ConversationID != nil && *req.ConversationID != "" {
		uid, _ := uuid.Parse(*req.ConversationID)
		convID = &uid
	}
	var taskID *uuid.UUID
	if req.TaskID != nil && *req.TaskID != "" {
		uid, _ := uuid.Parse(*req.TaskID)
		taskID = &uid
	}
	var sourceEventID *uuid.UUID
	if req.SourceEventID != nil && *req.SourceEventID != "" {
		uid, _ := uuid.Parse(*req.SourceEventID)
		sourceEventID = &uid
	}
	if req.Scope == "" {
		switch req.MemoryType {
		case "working": req.Scope = "task"
		case "project": req.Scope = "project"
		case "agent": req.Scope = "agent"
		case "conversation": req.Scope = "conversation"
		case "task": req.Scope = "task"
		default: req.Scope = "agent"
		}
	}
	if req.Source == "" { req.Source = "manual" }
	if req.Importance == 0 { req.Importance = 0.5 }
	// Build consolidate input directly for richer fields
	in := memory.ConsolidateInput{
		OrganizationID: claims.OrganizationID,
		AgentID:        agentID,
		ProjectID:      projectID,
		ConversationID: convID,
		TaskID:         taskID,
		MemoryType:     req.MemoryType,
		Scope:          req.Scope,
		Source:         req.Source,
		SourceEventID:  sourceEventID,
		Content:        req.Content,
		Importance:     req.Importance,
		Confidence:     req.Confidence,
		Metadata:       req.Metadata,
	}
	out, err := h.memory.Consolidate(r.Context(), in)
	if err != nil {
		writeError(w, 400, "VALIDATION_ERROR", err.Error()); return
	}
	if out.Result == memory.ResultRejected {
		writeError(w, 400, "REJECTED", out.Reason); return
	}
	// Return with result metadata
	resp := map[string]any{
		"memory": out.Memory,
		"result": string(out.Result),
		"reason": out.Reason,
	}
	if out.RelatedID != nil {
		resp["relatedId"] = out.RelatedID.String()
	}
	// If created/merged, return 201/200 accordingly
	status := 201
	if out.Result == memory.ResultMerged || out.Result == memory.ResultUpdated || out.Result == memory.ResultConflict {
		status = 200
	}
	writeData(w, status, resp, nil)
}

func (h *MemoryHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	q := r.URL.Query()
	var agentID *uuid.UUID
	if s := q.Get("agentId"); s != "" {
		uid, _ := uuid.Parse(s)
		agentID = &uid
	}
	var projectID *uuid.UUID
	if s := q.Get("projectId"); s != "" {
		uid, _ := uuid.Parse(s)
		projectID = &uid
	}
	scope := q.Get("scope")
	memoryType := q.Get("memoryType")
	status := q.Get("status")
	// If status explicitly requested as all, include archived etc. Otherwise default active
	if q.Get("all") == "true" {
		status = ""
	}
	mems, err := h.memory.List(r.Context(), claims.OrganizationID, agentID, projectID, status, scope, memoryType, 50)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, mems, nil)
}

func (h *MemoryHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	mem, err := h.memory.Get(r.Context(), claims.OrganizationID, id)
	if err != nil { writeError(w, 404, "NOT_FOUND", err.Error()); return }
	// Fetch versions for audit
	versions, _ := h.memory.ListVersions(r.Context(), id)
	resp := map[string]any{
		"memory":   mem,
		"versions": versions,
	}
	writeData(w, 200, resp, nil)
}

func (h *MemoryHandler) Search(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	var req struct {
		Query         string  `json:"query"`
		AgentID       *string `json:"agentId"`
		ProjectID     *string `json:"projectId"`
		Limit         int     `json:"limit"`
		MinImportance float64 `json:"minImportance"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return
	}
	var agentID *uuid.UUID
	if req.AgentID != nil && *req.AgentID != "" {
		uid, _ := uuid.Parse(*req.AgentID)
		agentID = &uid
	}
	var projectID *uuid.UUID
	if req.ProjectID != nil && *req.ProjectID != "" {
		uid, _ := uuid.Parse(*req.ProjectID)
		projectID = &uid
	}
	results, err := h.memory.Retrieve(r.Context(), claims.OrganizationID, agentID, projectID, req.Query, req.Limit, req.MinImportance)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, results, nil)
}

func (h *MemoryHandler) Archive(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	if err := h.memory.Archive(r.Context(), claims.OrganizationID, id); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", err.Error()); return
	}
	writeData(w, 200, map[string]any{"archived": true}, nil)
}

func (h *MemoryHandler) Restore(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	if err := h.memory.Restore(r.Context(), claims.OrganizationID, id); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", err.Error()); return
	}
	writeData(w, 200, map[string]any{"restored": true}, nil)
}

func (h *MemoryHandler) Versions(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	// Ensure org access
	if _, err := h.memory.Get(r.Context(), claims.OrganizationID, id); err != nil {
		writeError(w, 404, "NOT_FOUND", err.Error()); return
	}
	vers, err := h.memory.ListVersions(r.Context(), id)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, vers, nil)
}

func (h *MemoryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	// Prefer soft archive over hard delete for traceability
	if err := h.memory.Archive(r.Context(), claims.OrganizationID, id); err != nil {
		// Fallback to hard delete if archive fails (e.g., legacy rows without status column)
		_, err2 := h.repo.Exec(`DELETE FROM memories WHERE id=$1 AND organization_id=$2`, id, claims.OrganizationID)
		if err2 != nil { writeError(w, 500, "INTERNAL", err2.Error()); return }
	}
	writeData(w, 200, map[string]any{"deleted": true, "archived": true}, nil)
}
