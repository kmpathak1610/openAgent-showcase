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

type ProjectHandler struct { db *repository.DB }

func NewProjectHandler(db *repository.DB) *ProjectHandler { return &ProjectHandler{db: db} }

func (h *ProjectHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	if _, ok := h.db.IsOrgMember(claims.OrganizationID, claims.UserID); !ok {
		writeError(w, 403, "FORBIDDEN", "not a member of organization")
		return
	}
	projects, err := h.db.ListProjects(claims.OrganizationID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if projects == nil {
		writeData(w, 200, []any{}, nil)
		return
	}
	writeData(w, 200, projects, nil)
}

func (h *ProjectHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	if _, ok := h.db.IsOrgMember(claims.OrganizationID, claims.UserID); !ok {
		writeError(w, 403, "FORBIDDEN", "not a member")
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Objective   string `json:"objective"`
		Icon        string `json:"icon"`
		Slug        string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return }
	if req.Name == "" { writeError(w, 400, "VALIDATION_ERROR", "name required"); return }
	if req.Icon == "" { req.Icon = "📁" }
	p, err := h.db.CreateProject(claims.OrganizationID, req.Name, req.Description, req.Objective, req.Icon, req.Slug, claims.UserID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 201, p, nil)
}

func (h *ProjectHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	pid, _ := uuid.Parse(chi.URLParam(r, "id"))
	p, err := h.db.GetProject(claims.OrganizationID, pid)
	if err == sql.ErrNoRows { writeError(w, 404, "NOT_FOUND", "project not found"); return }
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, p, nil)
}

func (h *ProjectHandler) Update(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	pid, _ := uuid.Parse(chi.URLParam(r, "id"))
	// must be org member; for now any member can update, future: project member check
	if _, ok := h.db.IsOrgMember(claims.OrganizationID, claims.UserID); !ok { writeError(w, 403, "FORBIDDEN", "forbidden"); return }
	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Objective   *string `json:"objective"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return }
	p, err := h.db.UpdateProject(claims.OrganizationID, pid, req.Name, req.Description, req.Objective)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, p, nil)
}

func (h *ProjectHandler) Archive(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	pid, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, ok := h.db.IsOrgMember(claims.OrganizationID, claims.UserID); !ok { writeError(w, 403, "FORBIDDEN", "forbidden"); return }
	if err := h.db.ArchiveProject(claims.OrganizationID, pid); err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, map[string]any{"archived": true}, nil)
}

func (h *ProjectHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	pid, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.db.GetProject(claims.OrganizationID, pid); err != nil { writeError(w, 404, "NOT_FOUND", "project not found"); return }
	members, err := h.db.ListProjectMembers(pid)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, members, nil)
}

func (h *ProjectHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	pid, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.db.GetProject(claims.OrganizationID, pid); err != nil { writeError(w, 404, "NOT_FOUND", "project not found"); return }
	var req struct{ Email string `json:"email"`; Role string `json:"role"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return }
	user, err := h.db.FindUserByEmail(req.Email)
	if err == sql.ErrNoRows { writeError(w, 404, "NOT_FOUND", "user not found"); return }
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	// user must be org member
	if _, ok := h.db.IsOrgMember(claims.OrganizationID, user.ID); !ok { writeError(w, 400, "VALIDATION_ERROR", "user not in organization"); return }
	if err := h.db.AddProjectMember(pid, user.ID, req.Role); err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 201, map[string]any{"userId": user.ID}, nil)
}

func (h *ProjectHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	pid, _ := uuid.Parse(chi.URLParam(r, "id"))
	uid, _ := uuid.Parse(chi.URLParam(r, "userId"))
	if _, ok := h.db.IsOrgMember(claims.OrganizationID, claims.UserID); !ok {
		writeError(w, 403, "FORBIDDEN", "not org member")
		return
	}
	if err := h.db.RemoveProjectMember(pid, uid); err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, map[string]any{"removed": true}, nil)
}

func (h *ProjectHandler) ListAgents(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	pid, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.db.GetProject(claims.OrganizationID, pid); err != nil { writeError(w, 404, "NOT_FOUND", "project not found"); return }
	agents, err := h.db.ListProjectAgents(pid)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if agents == nil { writeData(w, 200, []any{}, nil); return }
	writeData(w, 200, agents, nil)
}

func (h *ProjectHandler) AddAgent(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	pid, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.db.GetProject(claims.OrganizationID, pid); err != nil { writeError(w, 404, "NOT_FOUND", "project not found"); return }
	var req struct {
		AgentID string `json:"agentId"`
		Role    string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return }
	aid, err := uuid.Parse(req.AgentID)
	if err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid agentId"); return }
	if _, err := h.db.GetAgent(claims.OrganizationID, aid); err != nil { writeError(w, 404, "NOT_FOUND", "agent not found"); return }
	if req.Role == "" { req.Role = "collaborator" }
	if err := h.db.AddProjectAgent(pid, aid, req.Role, claims.UserID); err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 201, map[string]any{"projectId": pid, "agentId": aid, "role": req.Role}, nil)
}

func (h *ProjectHandler) RemoveAgent(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	pid, _ := uuid.Parse(chi.URLParam(r, "id"))
	aid, _ := uuid.Parse(chi.URLParam(r, "agentId"))
	if _, err := h.db.GetProject(claims.OrganizationID, pid); err != nil { writeError(w, 404, "NOT_FOUND", "project not found"); return }
	if err := h.db.RemoveProjectAgent(pid, aid); err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, map[string]any{"removed": true}, nil)
}
