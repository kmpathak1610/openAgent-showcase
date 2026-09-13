package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"openagent/internal/http/middleware"
	"openagent/internal/repository"
)

type OrgHandler struct { db *repository.DB }

func NewOrgHandler(db *repository.DB) *OrgHandler { return &OrgHandler{db: db} }

func (h *OrgHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	orgs, err := h.db.ListOrganizationsForUser(claims.UserID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if orgs == nil {
		writeData(w, 200, []any{}, nil)
		return
	}
	writeData(w, 200, orgs, nil)
}

func (h *OrgHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	var req struct{ Name string `json:"name"`; Slug string `json:"slug"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return }
	if req.Name == "" { writeError(w, 400, "VALIDATION_ERROR", "name required"); return }
	org, err := h.db.CreateOrganization(req.Name, req.Slug, claims.UserID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 201, org, nil)
}

func (h *OrgHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	orgID, _ := uuid.Parse(chi.URLParam(r, "id"))
	if orgID != claims.OrganizationID {
		// allow access if user is member of that org (for switching UI)
		if _, ok := h.db.IsOrgMember(orgID, claims.UserID); !ok {
			writeError(w, 403, "FORBIDDEN", "not a member")
			return
		}
	}
	org, err := h.db.GetOrganization(orgID)
	if err == sql.ErrNoRows { writeError(w, 404, "NOT_FOUND", "organization not found"); return }
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, org, nil)
}

func (h *OrgHandler) Update(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	orgID, _ := uuid.Parse(chi.URLParam(r, "id"))
	if orgID != claims.OrganizationID { writeError(w, 403, "FORBIDDEN", "cannot update other organization"); return }
	role, _ := h.db.IsOrgMember(orgID, claims.UserID)
	if role != "owner" && role != "admin" { writeError(w, 403, "FORBIDDEN", "requires owner/admin"); return }
	var req struct{ Name string `json:"name"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return }
	org, err := h.db.UpdateOrganization(orgID, req.Name)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, org, nil)
}

func (h *OrgHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	orgID, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, ok := h.db.IsOrgMember(orgID, claims.UserID); !ok { writeError(w, 403, "FORBIDDEN", "not a member"); return }
	members, err := h.db.ListOrgMembers(orgID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, members, nil)
}

func (h *OrgHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	orgID, _ := uuid.Parse(chi.URLParam(r, "id"))
	role, _ := h.db.IsOrgMember(orgID, claims.UserID)
	if role != "owner" && role != "admin" { writeError(w, 403, "FORBIDDEN", "requires owner/admin"); return }
	var req struct{ Email string `json:"email"`; Role string `json:"role"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return }
	user, err := h.db.FindUserByEmail(req.Email)
	if err == sql.ErrNoRows { writeError(w, 404, "NOT_FOUND", "user not found"); return }
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if err := h.db.AddOrgMember(orgID, user.ID, req.Role); err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 201, map[string]any{"userId": user.ID, "role": req.Role}, nil)
}

func (h *OrgHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	orgID, _ := uuid.Parse(chi.URLParam(r, "id"))
	userID, _ := uuid.Parse(chi.URLParam(r, "userId"))
	role, _ := h.db.IsOrgMember(orgID, claims.UserID)
	if role != "owner" && role != "admin" { writeError(w, 403, "FORBIDDEN", "requires owner/admin"); return }
	if userID == claims.UserID { writeError(w, 400, "VALIDATION_ERROR", "cannot remove yourself"); return }
	if err := h.db.RemoveOrgMember(orgID, userID); err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, map[string]any{"removed": true}, nil)
}
