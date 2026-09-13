package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"openagent/internal/http/middleware"
	"openagent/internal/integration"
)

type IntegrationHandler struct {
	svc *integration.Service
}

func NewIntegrationHandler(svc *integration.Service) *IntegrationHandler {
	return &IntegrationHandler{svc: svc}
}

func (h *IntegrationHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	intgs, err := h.svc.List(claims.OrganizationID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if intgs == nil { writeData(w, 200, []any{}, nil); return }
	writeData(w, 200, intgs, nil)
}

func (h *IntegrationHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	var req struct {
		Name        string         `json:"name"`
		Provider    string         `json:"provider"`
		Config      map[string]any `json:"config"`
		Credentials map[string]any `json:"credentials"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return
	}
	if req.Name == "" || req.Provider == "" {
		writeError(w, 400, "VALIDATION_ERROR", "name and provider required"); return
	}
	// Credential isolation: credentials are encrypted at rest, never logged raw
	integ, err := h.svc.Create(claims.OrganizationID, req.Name, req.Provider, req.Config, req.Credentials, claims.UserID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 201, integ, nil)
}

func (h *IntegrationHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	integ, err := h.svc.Get(claims.OrganizationID, id)
	if err != nil { writeError(w, 404, "NOT_FOUND", "integration not found"); return }
	writeData(w, 200, integ, nil)
}

func (h *IntegrationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	if err := h.svc.Delete(claims.OrganizationID, id); err != nil {
		writeError(w, 500, "INTERNAL", err.Error()); return
	}
	writeData(w, 200, map[string]any{"deleted": true}, nil)
}
