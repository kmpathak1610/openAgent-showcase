package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"openagent/internal/domain"
	"openagent/internal/http/middleware"
	"openagent/internal/repository"
	"openagent/internal/team"
)

type TeamHandler struct {
	repo       *repository.DB
	builder    *team.Service
	orchestrator *team.Orchestrator
}

func NewTeamHandler(repo *repository.DB, builder *team.Service, orch *team.Orchestrator) *TeamHandler {
	return &TeamHandler{repo: repo, builder: builder, orchestrator: orch}
}

func (h *TeamHandler) BuilderPreview(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	if _, ok := h.repo.IsOrgMember(claims.OrganizationID, claims.UserID); !ok {
		writeError(w, 403, "FORBIDDEN", "not org member"); return
	}
	var req team.Input
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return
	}
	if req.Outcome == "" { writeError(w, 400, "VALIDATION_ERROR", "outcome required"); return }
	preview, err := h.builder.Propose(r.Context(), req)
	if err != nil { writeError(w, 400, "VALIDATION_ERROR", err.Error()); return }
	writeData(w, 200, preview, nil)
}

func (h *TeamHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	var raw struct {
		Preview domain.TeamBuilderPreview `json:"preview"`
		ProjectID *string `json:"projectId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return
	}
	preview := raw.Preview
	if err := team.ValidatePreview(&preview); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", err.Error()); return
	}
	// Create team
	teamObj, err := h.repo.CreateTeam(claims.OrganizationID, preview.Name, preview.Objective, preview.Description, claims.UserID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	// Create version 1
	previewMap, _ := json.Marshal(preview)
	var previewGeneric map[string]any
	_ = json.Unmarshal(previewMap, &previewGeneric)
	_, err = h.repo.CreateTeamVersion(teamObj.ID, 1, preview.Objective, preview.Workflow, preview.CommunicationRules, preview.DelegationRules, preview.Permissions, preview.ApprovalPolicy, previewGeneric, claims.UserID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	// Create agents for each proposal (if not exists, create)
	for _, agentProp := range preview.Agents {
		agents, _ := h.repo.ListAgents(claims.OrganizationID)
		var agentID uuid.UUID
		found := false
		for _, a := range agents {
			if a.Name == agentProp.Name { agentID = a.ID; found = true; break }
		}
		if !found {
			agentPreview := domain.AgentBuilderPreview{
				Name: agentProp.Name, Description: agentProp.Responsibilities, Purpose: agentProp.Responsibilities,
				Role: agentProp.Role, Objective: agentProp.Responsibilities, AutonomyLevel: agentProp.Autonomy,
			}
			createdAgent, err := h.repo.CreateAgent(claims.OrganizationID, &agentPreview, claims.UserID, agentProp.Responsibilities)
			if err == nil {
				agentID = createdAgent.ID
			} else {
				continue
			}
		}
		_, _ = h.repo.AddTeamMember(teamObj.ID, agentID, agentProp.Role, agentProp.Responsibilities, agentProp.Dependencies, agentProp.Tools, agentProp.Knowledge)
	}
	if raw.ProjectID != nil && *raw.ProjectID != "" {
		if pid, err := uuid.Parse(*raw.ProjectID); err == nil {
			_ = h.repo.AssignTeamToProject(pid, teamObj.ID, claims.UserID)
		}
	}
	full, _ := h.repo.GetTeam(claims.OrganizationID, teamObj.ID)
	writeData(w, 201, full, nil)
}

func (h *TeamHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	teams, err := h.repo.ListTeams(claims.OrganizationID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if teams == nil { writeData(w, 200, []any{}, nil); return }
	writeData(w, 200, teams, nil)
}

func (h *TeamHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	team, err := h.repo.GetTeam(claims.OrganizationID, id)
	if err != nil { writeError(w, 404, "NOT_FOUND", "team not found"); return }
	writeData(w, 200, team, nil)
}

func (h *TeamHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.repo.GetTeam(claims.OrganizationID, id); err != nil { writeError(w, 404, "NOT_FOUND", "team not found"); return }
	members, _ := h.repo.ListTeamMembers(id)
	writeData(w, 200, members, nil)
}

func (h *TeamHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.repo.GetTeam(claims.OrganizationID, id); err != nil { writeError(w, 404, "NOT_FOUND", "team not found"); return }
	var req struct{ AgentID string `json:"agentId"`; Role string `json:"role"`; Responsibilities string `json:"responsibilities"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return }
	aid, _ := uuid.Parse(req.AgentID)
	m, err := h.repo.AddTeamMember(id, aid, req.Role, req.Responsibilities, nil, nil, nil)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 201, m, nil)
}

func (h *TeamHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	teamID, _ := uuid.Parse(chi.URLParam(r, "id"))
	agentID, _ := uuid.Parse(chi.URLParam(r, "agentId"))
	if _, err := h.repo.GetTeam(claims.OrganizationID, teamID); err != nil { writeError(w, 404, "NOT_FOUND", "team not found"); return }
	if err := h.repo.RemoveTeamMember(teamID, agentID); err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, map[string]any{"removed": true}, nil)
}

func (h *TeamHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	teamID, _ := uuid.Parse(chi.URLParam(r, "id"))
	team, err := h.repo.GetTeam(claims.OrganizationID, teamID)
	if err != nil { writeError(w, 404, "NOT_FOUND", "team not found"); return }
	members, _ := h.repo.ListTeamMembers(teamID)
	// Active tasks: tasks with team_id = teamID and status in pending/assigned/running/waiting
	tasks, _ := h.repo.ListTasks(claims.OrganizationID, nil, nil, "", nil, 100)
	var active, waiting, blocked, completed []*domain.Task
	for _, t := range tasks {
		if t.TeamID != nil && *t.TeamID == teamID {
			switch t.Status {
			case "pending", "assigned", "running":
				active = append(active, t)
			case "waiting", "blocked", "approval_required":
				waiting = append(waiting, t)
				// check if blocked via dependencies
				if blockedFlag, _ := h.repo.IsTaskBlocked(t.ID); blockedFlag {
					blocked = append(blocked, t)
				}
			case "completed":
				completed = append(completed, t)
			}
		}
	}
	// Agents status
	agentsWorking := 0
	agentsWaiting := 0
	for _, m := range members {
		// check if agent has active task
		hasActive := false
		hasWaiting := false
		for _, t := range active { if t.AssignedToAgent != nil && *t.AssignedToAgent == m.AgentID { hasActive = true } }
		for _, t := range waiting { if t.AssignedToAgent != nil && *t.AssignedToAgent == m.AgentID { hasWaiting = true } }
		if hasActive { agentsWorking++ }
		if hasWaiting { agentsWaiting++ }
	}
	// Pending approvals for team tasks
	approvals, _ := h.repo.Query(`SELECT COUNT(*) FROM approvals WHERE organization_id=$1 AND status='pending'`, claims.OrganizationID)
	var pendingApprovals int
	if approvals != nil {
		// we need to count, but for dashboard we can just query task-related approvals
		// Simplified: count pending approvals where payload contains team tasks
		// For Phase 6, just return count from approvals where status pending
		rows, _ := h.repo.Query(`SELECT COUNT(*) FROM approvals WHERE organization_id=$1 AND status='pending'`, claims.OrganizationID)
		if rows != nil {
			for rows.Next() { rows.Scan(&pendingApprovals) }
			rows.Close()
		}
	}
	activity, _ := h.repo.ListTeamActivity(teamID, 20)
	_ = team
	writeData(w, 200, map[string]any{
		"team": team,
		"members": members,
		"activeTasks": active,
		"waitingTasks": waiting,
		"blockedTasks": blocked,
		"completedTasks": completed,
		"agentsWorking": agentsWorking,
		"agentsWaiting": agentsWaiting,
		"pendingApprovals": pendingApprovals,
		"recentActivity": activity,
	}, nil)
}

func (h *TeamHandler) StartTask(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	teamID, _ := uuid.Parse(chi.URLParam(r, "id"))
	var req struct{
		ProjectID string `json:"projectId"`
		ChannelID *string `json:"channelId"`
		Title string `json:"title"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return
	}
	projectID, _ := uuid.Parse(req.ProjectID)
	var channelID *uuid.UUID
	if req.ChannelID != nil && *req.ChannelID != "" {
		cid, _ := uuid.Parse(*req.ChannelID)
		channelID = &cid
	}
	task, err := h.orchestrator.StartTeamTask(r.Context(), claims.OrganizationID, teamID, projectID, channelID, req.Title, req.Description, claims.UserID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 201, task, nil)
}

func (h *TeamHandler) AddDependency(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	taskID, _ := uuid.Parse(chi.URLParam(r, "id"))
	var req struct{ DependsOn string `json:"dependsOn"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return
	}
	depID, _ := uuid.Parse(req.DependsOn)
	// verify both tasks belong to same org
	if _, err := h.repo.GetTask(claims.OrganizationID, taskID); err != nil { writeError(w, 404, "NOT_FOUND", "task not found"); return }
	if _, err := h.repo.GetTask(claims.OrganizationID, depID); err != nil { writeError(w, 404, "NOT_FOUND", "dependsOn task not found"); return }
	if err := h.repo.AddTaskDependency(taskID, depID); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", err.Error()); return
	}
	writeData(w, 201, map[string]any{"taskId": taskID, "dependsOn": depID}, nil)
}

func (h *TeamHandler) ListDependencies(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	taskID, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.repo.GetTask(claims.OrganizationID, taskID); err != nil { writeError(w, 404, "NOT_FOUND", "task not found"); return }
	deps, _ := h.repo.ListTaskDependencies(taskID)
	writeData(w, 200, deps, nil)
}

func (h *TeamHandler) IsBlocked(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	taskID, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.repo.GetTask(claims.OrganizationID, taskID); err != nil { writeError(w, 404, "NOT_FOUND", "task not found"); return }
	blocked, _ := h.repo.IsTaskBlocked(taskID)
	writeData(w, 200, map[string]any{"taskId": taskID, "blocked": blocked}, nil)
}

func (h *TeamHandler) ListActivity(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	teamID, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.repo.GetTeam(claims.OrganizationID, teamID); err != nil { writeError(w, 404, "NOT_FOUND", "team not found"); return }
	activity, _ := h.repo.ListTeamActivity(teamID, 20)
	writeData(w, 200, activity, nil)
}
