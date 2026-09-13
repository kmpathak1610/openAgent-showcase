package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"openagent/internal/http/middleware"
	"openagent/internal/repository"
)

type RunHandler struct {
	db *repository.DB
}

func NewRunHandler(db *repository.DB) *RunHandler { return &RunHandler{db: db} }

func (h *RunHandler) List(w http.ResponseWriter, r *http.Request) {
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
	runs, err := h.db.ListAgentRuns(claims.OrganizationID, agentID, taskID, 20)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if runs == nil { writeData(w, 200, []any{}, nil); return }
	writeData(w, 200, runs, nil)
}

func (h *RunHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	rid, _ := uuid.Parse(chi.URLParam(r, "id"))
	run, err := h.db.GetAgentRun(claims.OrganizationID, rid)
	if err != nil { writeError(w, 404, "NOT_FOUND", "run not found"); return }
	writeData(w, 200, run, nil)
}

func (h *RunHandler) ListByTask(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	tid, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.db.GetTask(claims.OrganizationID, tid); err != nil {
		writeError(w, 404, "NOT_FOUND", "task not found"); return
	}
	runs, err := h.db.ListAgentRuns(claims.OrganizationID, nil, &tid, 20)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, runs, nil)
}

func (h *RunHandler) ListByAgent(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	aid, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.db.GetAgent(claims.OrganizationID, aid); err != nil {
		writeError(w, 404, "NOT_FOUND", "agent not found"); return
	}
	runs, err := h.db.ListAgentRuns(claims.OrganizationID, &aid, nil, 20)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, runs, nil)
}
