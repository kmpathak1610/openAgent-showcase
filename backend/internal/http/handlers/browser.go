package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"openagent/internal/browser"
	"openagent/internal/http/middleware"
	"openagent/internal/repository"
)

type BrowserHandler struct {
	repo    *repository.DB
	manager *browser.Manager
}

func NewBrowserHandler(repo *repository.DB, m *browser.Manager) *BrowserHandler {
	return &BrowserHandler{repo: repo, manager: m}
}

func (h *BrowserHandler) CreateProfile(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	var req struct {
		Provider string         `json:"provider"`
		Name     string         `json:"name"`
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return
	}
	p, err := h.manager.CreateProfile(r.Context(), claims.OrganizationID, claims.UserID, req.Provider, req.Name, req.Metadata)
	if err != nil { writeError(w, 400, "VALIDATION_ERROR", err.Error()); return }
	writeData(w, 201, p, nil)
}

func (h *BrowserHandler) ListProfiles(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	list, err := h.manager.ListProfiles(r.Context(), claims.OrganizationID, claims.UserID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, list, nil)
}

func (h *BrowserHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	p, err := h.manager.GetProfile(r.Context(), claims.OrganizationID, claims.UserID, id)
	if err != nil { writeError(w, 404, "NOT_FOUND", err.Error()); return }
	// return safe view (no storageKey)
	writeData(w, 200, map[string]any{
		"id": p.ID, "provider": p.Provider, "name": p.Name, "status": p.Status,
		"organizationId": p.OrganizationID, "ownerUserId": p.OwnerUserID,
		"metadata": p.Metadata, "createdAt": p.CreatedAt, "lastUsedAt": p.LastUsedAt,
	}, nil)
}

func (h *BrowserHandler) DeleteProfile(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	if err := h.manager.DeleteProfile(r.Context(), claims.OrganizationID, claims.UserID, id); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", err.Error()); return
	}
	writeData(w, 200, map[string]any{"deleted": true}, nil)
}

func (h *BrowserHandler) UpdateProfileStatus(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	var req struct{ Status string `json:"status"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return }
	if err := h.manager.UpdateProfileStatus(r.Context(), claims.OrganizationID, claims.UserID, id, req.Status); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", err.Error()); return
	}
	writeData(w, 200, map[string]any{"updated": true}, nil)
}

func (h *BrowserHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	profileID, _ := uuid.Parse(chi.URLParam(r, "id"))
	s, err := h.manager.CreateSession(r.Context(), claims.OrganizationID, claims.UserID, profileID)
	if err != nil { writeError(w, 400, "VALIDATION_ERROR", err.Error()); return }
	// safe view
	writeData(w, 201, map[string]any{
		"id": s.ID, "browserProfileId": s.BrowserProfileID, "status": s.Status,
		"startedAt": s.StartedAt, "expiresAt": s.ExpiresAt,
	}, nil)
}

func (h *BrowserHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	list, err := h.manager.ListSessions(r.Context(), claims.OrganizationID, claims.UserID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	// safe view
	safe := []map[string]any{}
	for _, s := range list {
		safe = append(safe, map[string]any{"id": s.ID, "browserProfileId": s.BrowserProfileID, "status": s.Status, "startedAt": s.StartedAt, "expiresAt": s.ExpiresAt})
	}
	writeData(w, 200, safe, nil)
}

func (h *BrowserHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	s, err := h.manager.GetSession(r.Context(), claims.OrganizationID, claims.UserID, id)
	if err != nil { writeError(w, 404, "NOT_FOUND", err.Error()); return }
	writeData(w, 200, map[string]any{"id": s.ID, "browserProfileId": s.BrowserProfileID, "status": s.Status, "startedAt": s.StartedAt, "expiresAt": s.ExpiresAt}, nil)
}

func (h *BrowserHandler) CloseSession(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	if err := h.manager.CloseSession(r.Context(), claims.OrganizationID, claims.UserID, id); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", err.Error()); return
	}
	writeData(w, 200, map[string]any{"closed": true}, nil)
}

func (h *BrowserHandler) ListAudit(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	rows, err := h.repo.Query(`SELECT id, organization_id, owner_user_id, agent_id, task_id, browser_profile_id, browser_session_id, tool_name, action, target, domain, result_status, created_at FROM browser_audit_logs WHERE organization_id=$1 ORDER BY created_at DESC LIMIT 50`, claims.OrganizationID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, orgID uuid.UUID
		var ownerNS, agentNS, taskNS, profileNS, sessionNS sql.NullString
		var tool, action string
		var target, domain, result sql.NullString
		var created time.Time
		if err := rows.Scan(&id, &orgID, &ownerNS, &agentNS, &taskNS, &profileNS, &sessionNS, &tool, &action, &target, &domain, &result, &created); err != nil { continue }
		entry := map[string]any{
			"id": id, "toolName": tool, "action": action, "createdAt": created,
		}
		if target.Valid { entry["target"] = target.String }
		if domain.Valid { entry["domain"] = domain.String }
		if result.Valid { entry["resultStatus"] = result.String }
		out = append(out, entry)
	}
	if out == nil { out = []map[string]any{} }
	writeData(w, 200, out, nil)
}
