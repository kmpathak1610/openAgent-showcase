package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"openagent/internal/http/middleware"
	"openagent/internal/repository"
	"openagent/internal/scheduler"
)

type TriggerHandler struct {
	repo      *repository.DB
	scheduler *scheduler.Service
}

func NewTriggerHandler(repo *repository.DB, svc *scheduler.Service) *TriggerHandler {
	return &TriggerHandler{repo: repo, scheduler: svc}
}

func (h *TriggerHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	var req struct {
		Name        string         `json:"name"`
		TriggerType string         `json:"triggerType"`
		Config      map[string]any `json:"config"`
		AgentID     *string        `json:"agentId"`
		TeamID      *string        `json:"teamId"`
		ProjectID   *string        `json:"projectId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return
	}
	if req.Name == "" || req.TriggerType == "" {
		writeError(w, 400, "VALIDATION_ERROR", "name and triggerType required"); return
	}
	var agentID, teamID, projectID *uuid.UUID
	if req.AgentID != nil && *req.AgentID != "" {
		uid,_:=uuid.Parse(*req.AgentID)
		agentID=&uid
	}
	if req.TeamID != nil && *req.TeamID != "" {
		uid,_:=uuid.Parse(*req.TeamID)
		teamID=&uid
	}
	if req.ProjectID != nil && *req.ProjectID != "" {
		uid,_:=uuid.Parse(*req.ProjectID)
		projectID=&uid
	}
	trigger, err := h.scheduler.CreateTrigger(claims.OrganizationID, req.Name, req.TriggerType, req.Config, agentID, teamID, projectID, claims.UserID)
	if err != nil { writeError(w, 400, "VALIDATION_ERROR", err.Error()); return }
	writeData(w, 201, trigger, nil)
}

func (h *TriggerHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	triggers, err := h.scheduler.ListTriggers(claims.OrganizationID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if triggers == nil { writeData(w, 200, []any{}, nil); return }
	writeData(w, 200, triggers, nil)
}

func (h *TriggerHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	trig, err := h.scheduler.GetTrigger(claims.OrganizationID, id)
	if err != nil { writeError(w, 404, "NOT_FOUND", "trigger not found"); return }
	writeData(w, 200, trig, nil)
}

func (h *TriggerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	if err := h.scheduler.DeleteTrigger(claims.OrganizationID, id); err != nil {
		writeError(w, 500, "INTERNAL", err.Error()); return
	}
	writeData(w, 200, map[string]any{"deleted": true}, nil)
}

func (h *TriggerHandler) ListSchedules(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	// For Phase 7, scheduled_tasks are just a view of triggers with next_run_at
	triggers, _ := h.scheduler.ListTriggers(claims.OrganizationID)
	var schedules []map[string]any
	for _, t := range triggers {
		if t.TriggerType == "schedule" {
			schedules = append(schedules, map[string]any{
				"triggerId": t.ID,
				"name": t.Name,
				"nextRunAt": t.NextRunAt,
				"config": t.Config,
			})
		}
	}
	if schedules == nil { schedules = []map[string]any{} }
	writeData(w, 200, schedules, nil)
}
