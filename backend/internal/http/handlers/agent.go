package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"openagent/internal/builder"
	"openagent/internal/domain"
	"openagent/internal/http/middleware"
	"openagent/internal/repository"
)

type AgentHandler struct {
	db      *repository.DB
	builder *builder.Service
}

func NewAgentHandler(db *repository.DB, b *builder.Service) *AgentHandler {
	return &AgentHandler{db: db, builder: b}
}

// POST /agents/builder/preview
func (h *AgentHandler) BuilderPreview(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	if _, ok := h.db.IsOrgMember(claims.OrganizationID, claims.UserID); !ok {
		writeError(w, 403, "FORBIDDEN", "not org member")
		return
	}
	var in builder.Input
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", "invalid JSON")
		return
	}
	if strings.TrimSpace(in.Description) == "" {
		writeError(w, 400, "VALIDATION_ERROR", "description required")
		return
	}
	preview, err := h.builder.GeneratePreview(r.Context(), in)
	if err != nil {
		writeError(w, 400, "VALIDATION_ERROR", err.Error())
		return
	}
	writeData(w, 200, preview, nil)
}

// POST /agents
func (h *AgentHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	if _, ok := h.db.IsOrgMember(claims.OrganizationID, claims.UserID); !ok {
		writeError(w, 403, "FORBIDDEN", "not org member")
		return
	}
	var req struct {
		Preview domain.AgentBuilderPreview `json:"preview"`
		Intent  string                     `json:"intent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", "invalid JSON")
		return
	}
	preview := req.Preview
	// if preview empty, try to decode as flat (allow direct builder input fallback)
	if preview.Name == "" {
		writeError(w, 400, "VALIDATION_ERROR", "preview.name required")
		return
	}
	// Enforce safeguards: validate
	if err := builder.ValidatePreview(&preview); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", err.Error())
		return
	}
	// Never allow autonomous with dangerous caps silently
	if preview.AutonomyLevel == "autonomous" {
		for _, c := range preview.Capabilities {
			if c.Name == "publish_content" || c.Name == "external_api" || c.Name == "send_email" {
				writeError(w, 400, "VALIDATION_ERROR", "autonomous autonomy not allowed with publishing/external capabilities; use collaborative")
				return
			}
		}
	}
	intent := req.Intent
	if intent == "" { intent = preview.Description }
	agent, err := h.db.CreateAgent(claims.OrganizationID, &preview, claims.UserID, intent)
	if err != nil {
		writeError(w, 500, "INTERNAL", err.Error())
		return
	}
	// Auto grant minimal permissions (read org)
	_, _ = h.db.AddAgentPermission(agent.ID, "organization", &claims.OrganizationID, "read", claims.UserID)
	writeData(w, 201, agent, nil)
}

func (h *AgentHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	if _, ok := h.db.IsOrgMember(claims.OrganizationID, claims.UserID); !ok {
		writeError(w, 403, "FORBIDDEN", "not org member")
		return
	}
	agents, err := h.db.ListAgents(claims.OrganizationID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if agents == nil { agents = []*domain.Agent{} }
	writeData(w, 200, agents, nil)
}

func (h *AgentHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	aid, _ := uuid.Parse(chi.URLParam(r, "id"))
	agent, err := h.db.GetAgent(claims.OrganizationID, aid)
	if err == sql.ErrNoRows { writeError(w, 404, "NOT_FOUND", "agent not found"); return }
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	// enrich with projectAssignments? For detail page we also return projects via repo
	writeData(w, 200, agent, nil)
}

func (h *AgentHandler) Update(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	aid, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.db.GetAgent(claims.OrganizationID, aid); err != nil {
		writeError(w, 404, "NOT_FOUND", "agent not found")
		return
	}
	var req struct {
		Name          *string `json:"name"`
		Description   *string `json:"description"`
		Purpose       *string `json:"purpose"`
		AutonomyLevel *string `json:"autonomyLevel"`
		Status        *string `json:"status"`
		Version *domain.AgentVersion `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", "invalid JSON")
		return
	}
	// Security: only owner/admin can modify security policies
	if req.AutonomyLevel != nil || req.Status != nil || req.Version != nil {
		if role, ok := h.db.IsOrgMember(claims.OrganizationID, claims.UserID); !ok || (role != "owner" && role != "admin") {
			writeError(w, 403, "FORBIDDEN", "only owner/admin can modify agent security policies")
			return
		}
	}
	updates := map[string]any{}
	if req.Name != nil { updates["name"] = *req.Name }
	if req.Description != nil { updates["description"] = *req.Description }
	if req.Purpose != nil { updates["purpose"] = *req.Purpose }
	if req.AutonomyLevel != nil {
		if !domain.ValidAutonomyLevels[*req.AutonomyLevel] {
			writeError(w, 400, "VALIDATION_ERROR", "invalid autonomy_level")
			return
		}
		updates["autonomy_level"] = *req.AutonomyLevel
	}
	if req.Status != nil {
		if !domain.ValidAgentStatuses[*req.Status] {
			writeError(w, 400, "VALIDATION_ERROR", "invalid status")
			return
		}
		updates["status"] = *req.Status
	}
	if req.Version != nil {
		preview := domain.AgentBuilderPreview{
			Name:          "validation",
			AutonomyLevel: "assistant",
			Capabilities:  req.Version.Capabilities,
			ApprovalPolicy: req.Version.ApprovalPolicy,
		}
		if req.AutonomyLevel != nil { preview.AutonomyLevel = *req.AutonomyLevel }
		if err := builder.ValidatePreview(&preview); err != nil {
			writeError(w, 400, "VALIDATION_ERROR", err.Error())
			return
		}
	}
	agent, err := h.db.UpdateAgent(claims.OrganizationID, aid, updates, req.Version)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, agent, nil)
}

func (h *AgentHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	aid, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.db.GetAgent(claims.OrganizationID, aid); err != nil { writeError(w, 404, "NOT_FOUND", "agent not found"); return }
	versions, err := h.db.ListAgentVersions(aid)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, versions, nil)
}

func (h *AgentHandler) GetVersion(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	aid, _ := uuid.Parse(chi.URLParam(r, "id"))
	verStr := chi.URLParam(r, "version")
	var vnum int
	if _, err := json.Marshal(verStr); err != nil { _ = err }
	// parse int
	for _, c := range verStr { if c < '0' || c > '9' { writeError(w, 400, "VALIDATION_ERROR", "invalid version"); return } }
	// simple atoi
	vnum = 0
	for _, c := range verStr { vnum = vnum*10 + int(c-'0') }
	if _, err := h.db.GetAgent(claims.OrganizationID, aid); err != nil { writeError(w, 404, "NOT_FOUND", "agent not found"); return }
	ver, err := h.db.GetAgentVersion(aid, vnum)
	if err == sql.ErrNoRows { writeError(w, 404, "NOT_FOUND", "version not found"); return }
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, ver, nil)
}

func (h *AgentHandler) ListPermissions(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	aid, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.db.GetAgent(claims.OrganizationID, aid); err != nil { writeError(w, 404, "NOT_FOUND", "agent not found"); return }
	perms, err := h.db.ListAgentPermissions(aid)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, perms, nil)
}

func (h *AgentHandler) AddPermission(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	aid, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.db.GetAgent(claims.OrganizationID, aid); err != nil { writeError(w, 404, "NOT_FOUND", "agent not found"); return }
	var req struct {
		ResourceType string  `json:"resourceType"`
		ResourceID   *string `json:"resourceId"`
		Permission   string  `json:"permission"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return }
	var rid *uuid.UUID
	if req.ResourceID != nil && *req.ResourceID != "" {
		uid, err := uuid.Parse(*req.ResourceID)
		if err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid resourceId"); return }
		rid = &uid
	}
	perm, err := h.db.AddAgentPermission(aid, req.ResourceType, rid, req.Permission, claims.UserID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 201, perm, nil)
}
