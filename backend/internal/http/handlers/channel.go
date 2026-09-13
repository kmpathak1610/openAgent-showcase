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

type ChannelHandler struct { db *repository.DB }

func NewChannelHandler(db *repository.DB) *ChannelHandler { return &ChannelHandler{db: db} }

func (h *ChannelHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	if _, ok := h.db.IsOrgMember(claims.OrganizationID, claims.UserID); !ok { writeError(w, 403, "FORBIDDEN", "not a member"); return }
	q := r.URL.Query()
	var projectID *uuid.UUID
	if pidStr := q.Get("projectId"); pidStr != "" {
		pid, err := uuid.Parse(pidStr)
		if err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid projectId"); return }
		// verify project belongs to org
		if _, err := h.db.GetProject(claims.OrganizationID, pid); err != nil { writeError(w, 404, "NOT_FOUND", "project not found"); return }
		projectID = &pid
	}
	channels, err := h.db.ListChannels(claims.OrganizationID, projectID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, channels, nil)
}

func (h *ChannelHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	if _, ok := h.db.IsOrgMember(claims.OrganizationID, claims.UserID); !ok { writeError(w, 403, "FORBIDDEN", "not a member"); return }
	var req struct {
		Name        string  `json:"name"`
		DisplayName string  `json:"displayName"`
		Description string  `json:"description"`
		Topic       string  `json:"topic"`
		ChannelType string  `json:"channelType"`
		ProjectID   *string `json:"projectId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return }
	if req.Name == "" { writeError(w, 400, "VALIDATION_ERROR", "name required"); return }
	var projectID *uuid.UUID
	if req.ProjectID != nil && *req.ProjectID != "" {
		pid, err := uuid.Parse(*req.ProjectID)
		if err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid projectId"); return }
		if _, err := h.db.GetProject(claims.OrganizationID, pid); err != nil { writeError(w, 404, "NOT_FOUND", "project not found"); return }
		projectID = &pid
	}
	// org channels have projectID nil, project channels require projectID; enforce per type
	if projectID == nil && req.ChannelType == "" {
		// allow org channel
	}
	ch, err := h.db.CreateChannel(claims.OrganizationID, projectID, req.Name, req.DisplayName, req.Description, req.Topic, req.ChannelType, claims.UserID)
	if err != nil {
		// unique violation
		writeError(w, 400, "VALIDATION_ERROR", err.Error())
		return
	}
	writeData(w, 201, ch, nil)
}

func (h *ChannelHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	cid, _ := uuid.Parse(chi.URLParam(r, "id"))
	ch, err := h.db.GetChannel(claims.OrganizationID, cid)
	if err == sql.ErrNoRows { writeError(w, 404, "NOT_FOUND", "channel not found"); return }
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, ch, nil)
}

func (h *ChannelHandler) Update(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	cid, _ := uuid.Parse(chi.URLParam(r, "id"))
	var req struct {
		DisplayName *string `json:"displayName"`
		Description *string `json:"description"`
		Topic       *string `json:"topic"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return }
	ch, err := h.db.UpdateChannel(claims.OrganizationID, cid, req.DisplayName, req.Description, req.Topic)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, ch, nil)
}

func (h *ChannelHandler) Rename(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	cid, _ := uuid.Parse(chi.URLParam(r, "id"))
	var req struct{ Name string `json:"name"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return }
	ch, err := h.db.RenameChannel(claims.OrganizationID, cid, req.Name)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, ch, nil)
}

func (h *ChannelHandler) Archive(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	cid, _ := uuid.Parse(chi.URLParam(r, "id"))
	if err := h.db.ArchiveChannel(claims.OrganizationID, cid); err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, map[string]any{"archived": true}, nil)
}

func (h *ChannelHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	cid, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.db.GetChannel(claims.OrganizationID, cid); err != nil { writeError(w, 404, "NOT_FOUND", "channel not found"); return }
	members, err := h.db.ListChannelMembers(cid)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, members, nil)
}

func (h *ChannelHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	cid, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.db.GetChannel(claims.OrganizationID, cid); err != nil { writeError(w, 404, "NOT_FOUND", "channel not found"); return }
	var req struct{ Email string `json:"email"`; Role string `json:"role"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return }
	user, err := h.db.FindUserByEmail(req.Email)
	if err == sql.ErrNoRows { writeError(w, 404, "NOT_FOUND", "user not found"); return }
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if _, ok := h.db.IsOrgMember(claims.OrganizationID, user.ID); !ok { writeError(w, 400, "VALIDATION_ERROR", "user not in organization"); return }
	if err := h.db.AddChannelMember(cid, user.ID, req.Role); err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 201, map[string]any{"userId": user.ID}, nil)
}

func (h *ChannelHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	_ = claims
	cid, _ := uuid.Parse(chi.URLParam(r, "id"))
	uid, _ := uuid.Parse(chi.URLParam(r, "userId"))
	if err := h.db.RemoveChannelMember(cid, uid); err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, map[string]any{"removed": true}, nil)
}
