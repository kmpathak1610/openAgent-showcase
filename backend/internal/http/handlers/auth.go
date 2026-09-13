package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"openagent/internal/assistant"
	"openagent/internal/auth"
	"openagent/internal/http/middleware"
	"openagent/internal/repository"
)

type AuthHandler struct {
	auth      *auth.Service
	db        *repository.DB
	assistant *assistant.Service
}

func NewAuthHandler(a *auth.Service, db *repository.DB) *AuthHandler {
	return &AuthHandler{auth: a, db: db}
}

func (h *AuthHandler) SetAssistant(s *assistant.Service) { h.assistant = s }

type registerReq struct {
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
	OrgName     string `json:"orgName"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON")
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.Email == "" || req.Password == "" || req.DisplayName == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "email, displayName and password are required")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "password must be at least 8 characters")
		return
	}
	if h.db == nil {
		// stub mode (no DB) — keep foundation behavior for frontend dev
		_, _ = h.auth.HashPassword(req.Password)
		userID := uuid.New()
		orgID := uuid.New()
		tok, _ := h.auth.CreateToken(userID, orgID, "owner", req.Email, 24*time.Hour)
		writeData(w, http.StatusCreated, map[string]any{
			"user":         map[string]any{"id": userID, "email": req.Email, "displayName": req.DisplayName},
			"organization": map[string]any{"id": orgID},
			"token":        tok,
		}, nil)
		return
	}
	// check existing
	if _, err := h.db.FindUserByEmail(req.Email); err == nil {
		writeError(w, http.StatusConflict, "CONFLICT", "email already registered")
		return
	}
	hash, err := h.auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to hash password")
		return
	}
	user, err := h.db.CreateUser(req.Email, req.DisplayName, hash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to create user")
		return
	}
	orgName := req.OrgName
	if orgName == "" {
		orgName = req.DisplayName + "'s Workspace"
	}
	org, err := h.db.CreateOrganization(orgName, "", user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to create organization")
		return
	}
	// Ensure default assistant (idempotent) — Phase 6
	if h.assistant != nil {
		if _, err := h.assistant.EnsureDefaultAssistant(r.Context(), org.ID, user.ID); err != nil {
			// Log but don't fail signup — assistant is non-critical for auth
			_ = err
		}
	}
	// Ensure default home channel (idempotent) — never fail signup
	if _, err := h.db.EnsureGeneralChannel(org.ID, user.ID); err != nil {
		_ = err
	}
	tok, _ := h.auth.CreateToken(user.ID, org.ID, "owner", user.Email, 24*time.Hour)
	writeData(w, http.StatusCreated, map[string]any{
		"user":         map[string]any{"id": user.ID, "email": user.Email, "displayName": user.DisplayName},
		"organization": org,
		"token":        tok,
	}, nil)
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON")
		return
	}
	if h.db == nil {
		if req.Email == "" || req.Password == "" {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "email and password required")
			return
		}
		userID := uuid.New()
		orgID := uuid.New()
		tok, _ := h.auth.CreateToken(userID, orgID, "member", req.Email, 24*time.Hour)
		writeData(w, http.StatusOK, map[string]any{"token": tok, "user": map[string]any{"id": userID, "email": req.Email}}, nil)
		return
	}
	user, err := h.db.FindUserByEmail(req.Email)
	if err == sql.ErrNoRows || err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid credentials")
		return
	}
	if !h.auth.CheckPassword(user.PasswordHash, req.Password) {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid credentials")
		return
	}
	orgs, _ := h.db.ListOrganizationsForUser(user.ID)
	if len(orgs) == 0 {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "user has no organization")
		return
	}
	// pick first org; role lookup
	role, _ := h.db.IsOrgMember(orgs[0].ID, user.ID)
	if role == "" { role = "member" }
	tok, _ := h.auth.CreateToken(user.ID, orgs[0].ID, role, user.Email, 24*time.Hour)
	writeData(w, http.StatusOK, map[string]any{
		"token":        tok,
		"user":         map[string]any{"id": user.ID, "email": user.Email, "displayName": user.DisplayName},
		"organization": orgs[0],
		"organizations": orgs,
	}, nil)
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "not authenticated")
		return
	}
	if h.db == nil {
		writeData(w, http.StatusOK, map[string]any{"authenticated": true, "claims": claims}, nil)
		return
	}
	user, err := h.db.FindUserByID(claims.UserID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
		return
	}
	orgs, _ := h.db.ListOrganizationsForUser(claims.UserID)
	var currentOrg any
	if org, err := h.db.GetOrganization(claims.OrganizationID); err == nil {
		currentOrg = org
	}
	writeData(w, http.StatusOK, map[string]any{
		"user":            map[string]any{"id": user.ID, "email": user.Email, "displayName": user.DisplayName, "avatarUrl": user.AvatarURL},
		"organization":    currentOrg,
		"organizations":   orgs,
		"claims":          claims,
	}, nil)
}

func (h *AuthHandler) SwitchOrg(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "not authenticated")
		return
	}
	var req struct{ OrganizationID string `json:"organizationId"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON")
		return
	}
	orgID, err := uuid.Parse(req.OrganizationID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid organizationId")
		return
	}
	role, isMember := h.db.IsOrgMember(orgID, claims.UserID)
	if !isMember {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "not a member of organization")
		return
	}
	user, _ := h.db.FindUserByID(claims.UserID)
	tok, _ := h.auth.CreateToken(user.ID, orgID, role, user.Email, 24*time.Hour)
	writeData(w, http.StatusOK, map[string]any{"token": tok, "organizationId": orgID, "role": role}, nil)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Stateless JWT: client discards token. We just respond.
	writeData(w, http.StatusOK, map[string]any{"message": "logged out"}, nil)
}
