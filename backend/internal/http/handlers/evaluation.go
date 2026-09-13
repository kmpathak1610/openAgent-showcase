package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"openagent/internal/evaluation"
	"openagent/internal/http/middleware"
	"openagent/internal/repository"
)

type EvaluationHandler struct {
	repo *repository.DB
	svc  *evaluation.Service
}

func NewEvaluationHandler(repo *repository.DB) *EvaluationHandler {
	return &EvaluationHandler{repo: repo, svc: evaluation.New(repo)}
}

func (h *EvaluationHandler) EvaluateTask(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	taskID, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.repo.GetTask(claims.OrganizationID, taskID); err != nil {
		writeError(w, 404, "NOT_FOUND", "task not found")
		return
	}
	result, err := h.svc.Evaluate(r.Context(), claims.OrganizationID, taskID)
	if err != nil {
		writeError(w, 500, "INTERNAL", err.Error())
		return
	}
	writeData(w, 200, result, nil)
}

func (h *EvaluationHandler) GetEvaluation(w http.ResponseWriter, r *http.Request) {
	h.EvaluateTask(w, r)
}

func (h *EvaluationHandler) TaskMetrics(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	taskID, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.repo.GetTask(claims.OrganizationID, taskID); err != nil {
		writeError(w, 404, "NOT_FOUND", "task not found")
		return
	}
	result, err := h.svc.Evaluate(r.Context(), claims.OrganizationID, taskID)
	if err != nil {
		writeError(w, 500, "INTERNAL", err.Error())
		return
	}
	writeData(w, 200, map[string]any{
		"taskId": result.TaskID,
		"success": result.Success,
		"score": result.Score,
		"durationMs": result.DurationMs,
		"toolCalls": result.ToolCalls,
		"delegations": result.Delegations,
		"tokenUsage": result.TokenUsage,
		"cost": result.EstimatedCost,
	}, nil)
}
