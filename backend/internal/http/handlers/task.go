package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"openagent/internal/autonomy"
	"openagent/internal/http/middleware"
	"openagent/internal/repository"
	"openagent/internal/worker"
	"openagent/internal/ws"
)

type TaskHandler struct {
	db       *repository.DB
	worker   *worker.Pool
	hub      *ws.Hub
	autonomy *autonomy.Service
}

func NewTaskHandler(db *repository.DB, worker *worker.Pool, hub *ws.Hub) *TaskHandler {
	return &TaskHandler{db: db, worker: worker, hub: hub}
}

func NewTaskHandlerWithAutonomy(db *repository.DB, worker *worker.Pool, hub *ws.Hub, autonomy *autonomy.Service) *TaskHandler {
	return &TaskHandler{db: db, worker: worker, hub: hub, autonomy: autonomy}
}

func (h *TaskHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	if _, ok := h.db.IsOrgMember(claims.OrganizationID, claims.UserID); !ok {
		writeError(w, 403, "FORBIDDEN", "not org member"); return
	}
	var req struct {
		ProjectID      string  `json:"projectId"`
		ChannelID      *string `json:"channelId"`
		ParentTaskID   *string `json:"parentTaskId"`
		TeamID         *string `json:"teamId"`
		Title          string  `json:"title"`
		Description    string  `json:"description"`
		Priority       string  `json:"priority"`
		AssignedToAgent *string `json:"assignedToAgent"`
		AssignedToUser  *string `json:"assignedToUser"`
		Deadline       *string `json:"deadline"`
		CorrelationID  *string `json:"correlationId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return
	}
	if req.Title == "" { writeError(w, 400, "VALIDATION_ERROR", "title required"); return }
	if req.ProjectID == "" { writeError(w, 400, "VALIDATION_ERROR", "projectId required"); return }
	projectID, _ := uuid.Parse(req.ProjectID)
	if _, err := h.db.GetProject(claims.OrganizationID, projectID); err != nil {
		writeError(w, 404, "NOT_FOUND", "project not found"); return
	}
	var channelID *uuid.UUID
	if req.ChannelID != nil && *req.ChannelID != "" {
		cid, _ := uuid.Parse(*req.ChannelID)
		channelID = &cid
		if _, err := h.db.GetChannel(claims.OrganizationID, cid); err != nil {
			writeError(w, 404, "NOT_FOUND", "channel not found"); return
		}
	}
	var parentID *uuid.UUID
	if req.ParentTaskID != nil && *req.ParentTaskID != "" {
		pid, _ := uuid.Parse(*req.ParentTaskID)
		parentID = &pid
	}
	var assignedAgent *uuid.UUID
	if req.AssignedToAgent != nil && *req.AssignedToAgent != "" {
		aid, _ := uuid.Parse(*req.AssignedToAgent)
		if _, err := h.db.GetAgent(claims.OrganizationID, aid); err != nil {
			writeError(w, 404, "NOT_FOUND", "agent not found"); return
		}
		assignedAgent = &aid
	}
	var assignedUser *uuid.UUID
	if req.AssignedToUser != nil && *req.AssignedToUser != "" {
		uid, _ := uuid.Parse(*req.AssignedToUser)
		assignedUser = &uid
	}
	var deadline *time.Time
	if req.Deadline != nil && *req.Deadline != "" {
		if t, err := time.Parse(time.RFC3339, *req.Deadline); err == nil {
			deadline = &t
		}
	}
	var corrID *uuid.UUID
	if req.CorrelationID != nil && *req.CorrelationID != "" {
		cid, _ := uuid.Parse(*req.CorrelationID)
		corrID = &cid
	} else {
		gen := uuid.New()
		corrID = &gen
	}
	var teamID *uuid.UUID
	if req.TeamID != nil && *req.TeamID != "" {
		tid, _ := uuid.Parse(*req.TeamID)
		teamID = &tid
	}
	task, err := h.db.CreateTask(claims.OrganizationID, projectID, channelID, parentID, teamID, assignedAgent, assignedUser, req.Title, req.Description, req.Priority, deadline, corrID, claims.UserID)
	if err != nil {
		writeError(w, 500, "INTERNAL", err.Error()); return
	}
	// If assigned to agent, enqueue agent run
	if assignedAgent != nil && h.worker != nil {
		h.worker.Enqueue(worker.Job{
			ID:   task.ID.String(),
			Type: "agent_run",
			Payload: map[string]any{
				"organization_id": claims.OrganizationID.String(),
				"agent_id": assignedAgent.String(),
				"task_id": task.ID.String(),
				"correlation_id": corrID.String(),
				"trigger": "task_assigned",
			},
			Timeout: 60 * time.Second,
		})
		// WS: task created/assigned
		if h.hub != nil {
			h.hub.BroadcastToOrg(claims.OrganizationID, "agent.task_created", map[string]any{"task": task})
			h.hub.BroadcastToOrg(claims.OrganizationID, "agent.task_assigned", map[string]any{"taskId": task.ID.String(), "assignee": assignedAgent.String()})
			if channelID != nil {
				h.hub.Broadcast(ws.Event{Type: "agent.task_created", Payload: map[string]any{"task": task}, Room: "channel:" + channelID.String()})
			}
		}
	} else {
		if h.hub != nil {
			h.hub.BroadcastToOrg(claims.OrganizationID, "agent.task_created", map[string]any{"task": task})
		}
	}
	if h.autonomy != nil {
		go h.autonomy.HandleProjectEvent(context.Background(), claims.OrganizationID, projectID, "task.created", map[string]any{"taskId": task.ID.String(), "title": task.Title, "projectId": projectID.String()})
	}
	writeData(w, 201, task, nil)
}

func deadlineString(t *time.Time) *string {
	if t == nil { return nil }
	s := t.Format(time.RFC3339)
	return &s
}

func (h *TaskHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	if _, ok := h.db.IsOrgMember(claims.OrganizationID, claims.UserID); !ok {
		writeError(w, 403, "FORBIDDEN", "not org member"); return
	}
	q := r.URL.Query()
	var projectID *uuid.UUID
	if pidStr := q.Get("projectId"); pidStr != "" {
		pid, _ := uuid.Parse(pidStr)
		projectID = &pid
	}
	var channelID *uuid.UUID
	if cidStr := q.Get("channelId"); cidStr != "" {
		cid, _ := uuid.Parse(cidStr)
		channelID = &cid
	}
	status := q.Get("status")
	var assignedAgent *uuid.UUID
	if agStr := q.Get("assignedToAgent"); agStr != "" {
		aid, _ := uuid.Parse(agStr)
		assignedAgent = &aid
	}
	limit := 20
	tasks, err := h.db.ListTasks(claims.OrganizationID, projectID, channelID, status, assignedAgent, limit)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if tasks == nil { writeData(w, 200, []any{}, nil); return }
	writeData(w, 200, tasks, nil)
}

func (h *TaskHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	tid, _ := uuid.Parse(chi.URLParam(r, "id"))
	task, err := h.db.GetTask(claims.OrganizationID, tid)
	if err != nil { writeError(w, 404, "NOT_FOUND", "task not found"); return }
	writeData(w, 200, task, nil)
}

func (h *TaskHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	tid, _ := uuid.Parse(chi.URLParam(r, "id"))
	var req struct{ Status string `json:"status"`; Payload map[string]any `json:"payload"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return
	}
	// Need parent before update to capture parent ID
	parentTask, _ := h.db.GetTask(claims.OrganizationID, tid)
	task, err := h.db.UpdateTaskStatus(claims.OrganizationID, tid, req.Status, "user", claims.UserID, req.Payload)
	if err != nil { writeError(w, 400, "VALIDATION_ERROR", err.Error()); return }
	if h.hub != nil {
		h.hub.BroadcastToOrg(claims.OrganizationID, "agent.task_updated", map[string]any{"task": task})
	}
	// Parent aggregation: if this was a subtask and now completed, check parent
	if req.Status == "completed" && parentTask != nil && parentTask.ParentTaskID != nil {
		go func(pid uuid.UUID) {
			time.Sleep(600 * time.Millisecond)
			parent, err := h.db.GetTask(claims.OrganizationID, pid)
			if err != nil || parent.Status == "completed" || parent.Status == "failed" || parent.Status == "cancelled" {
				return
			}
			children, err := h.db.ListTasks(claims.OrganizationID, &parent.ProjectID, nil, "", nil, 100)
			if err != nil { return }
			hasPending := false
			allCompleted := 0
			total := 0
			for _, c := range children {
				if c.ParentTaskID != nil && *c.ParentTaskID == pid {
					total++
					if c.Status != "completed" && c.Status != "failed" && c.Status != "cancelled" { hasPending = true }
					if c.Status == "completed" { allCompleted++ }
				}
			}
			if total == 0 || hasPending { return }
			if allCompleted > 0 {
				if _, err := h.db.UpdateTaskStatus(claims.OrganizationID, pid, "completed", "user", claims.UserID, map[string]any{"aggregated": allCompleted}); err == nil && h.hub != nil {
					h.hub.BroadcastToOrg(claims.OrganizationID, "agent.completed", map[string]any{"taskId": pid.String(), "aggregated": true})
					if parent.ChannelID != nil {
						if m, _ := h.db.CreateMessage(claims.OrganizationID, *parent.ChannelID, nil, "system", nil, "✅ Parent completed — "+parent.Title); m != nil {
							h.hub.BroadcastToOrg(claims.OrganizationID, "agent.message", map[string]any{"message": m})
						}
					}
				}
			}
		}(*parentTask.ParentTaskID)
	}
	if h.autonomy != nil && (req.Status == "completed" || req.Status == "failed") {
		go h.autonomy.HandleProjectEvent(context.Background(), claims.OrganizationID, task.ProjectID, "task."+req.Status, map[string]any{"taskId": tid.String(), "status": req.Status, "projectId": task.ProjectID.String()})
	}
	writeData(w, 200, task, nil)
}

func (h *TaskHandler) Assign(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	tid, _ := uuid.Parse(chi.URLParam(r, "id"))
	var req struct {
		AgentID *string `json:"agentId"`
		UserID  *string `json:"userId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return
	}
	var agentID, userID *uuid.UUID
	if req.AgentID != nil && *req.AgentID != "" {
		aid, _ := uuid.Parse(*req.AgentID)
		if _, err := h.db.GetAgent(claims.OrganizationID, aid); err != nil {
			writeError(w, 404, "NOT_FOUND", "agent not found"); return
		}
		agentID = &aid
	}
	if req.UserID != nil && *req.UserID != "" {
		uid, _ := uuid.Parse(*req.UserID)
		userID = &uid
	}
	task, err := h.db.AssignTask(claims.OrganizationID, tid, agentID, userID, claims.UserID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	// enqueue if agent assigned
	if agentID != nil && h.worker != nil {
		corrStr := ""
		if task.CorrelationID != nil { corrStr = task.CorrelationID.String() } else { corrStr = uuid.New().String() }
		h.worker.Enqueue(worker.Job{
			ID:   task.ID.String(),
			Type: "agent_run",
			Payload: map[string]any{
				"organization_id": claims.OrganizationID.String(),
				"agent_id": agentID.String(),
				"task_id": task.ID.String(),
				"correlation_id": corrStr,
				"trigger": "task_assigned",
			},
		})
		if h.hub != nil {
			h.hub.BroadcastToOrg(claims.OrganizationID, "agent.task_assigned", map[string]any{"taskId": tid.String(), "assignee": agentID.String()})
		}
	}
	writeData(w, 200, task, nil)
}

func (h *TaskHandler) Delegate(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	tid, _ := uuid.Parse(chi.URLParam(r, "id"))
	var req struct {
		Title          string  `json:"title"`
		Description    string  `json:"description"`
		AssignedToAgent *string `json:"assignedToAgent"`
		SourceAgent    *string `json:"sourceAgent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return
	}
	parentTask, err := h.db.GetTask(claims.OrganizationID, tid)
	if err != nil { writeError(w, 404, "NOT_FOUND", "task not found"); return }
	var destAgent *uuid.UUID
	if req.AssignedToAgent != nil && *req.AssignedToAgent != "" {
		aid, _ := uuid.Parse(*req.AssignedToAgent)
		destAgent = &aid
	}
	var sourceAgentID *uuid.UUID
	if req.SourceAgent != nil && *req.SourceAgent != "" {
		aid, _ := uuid.Parse(*req.SourceAgent)
		sourceAgentID = &aid
	} else if parentTask.AssignedToAgent != nil {
		sourceAgentID = parentTask.AssignedToAgent
	}
	// correlation: use parent's correlation or generate
	corrID := parentTask.CorrelationID
	if corrID == nil {
		gen := uuid.New()
		corrID = &gen
	}
	subTask, err := h.db.CreateTask(claims.OrganizationID, parentTask.ProjectID, parentTask.ChannelID, &tid, parentTask.TeamID, destAgent, nil, req.Title, req.Description, "medium", nil, corrID, claims.UserID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	// event delegated
	_, _ = h.db.CreateTaskEvent(tid, "agent", sourceAgentID, "delegated", map[string]any{"subtask_id": subTask.ID.String(), "to": destAgent}, corrID)
	_, _ = h.db.CreateTaskEvent(subTask.ID, "agent", sourceAgentID, "created", map[string]any{"parent": tid.String()}, corrID)
	if h.hub != nil {
		h.hub.BroadcastToOrg(claims.OrganizationID, "agent.delegated", map[string]any{"parent": tid.String(), "subtask": subTask, "from": sourceAgentID, "to": destAgent, "correlationId": corrID.String()})
		h.hub.BroadcastToOrg(claims.OrganizationID, "agent.task_created", map[string]any{"task": subTask})
	}
	// enqueue delegated agent run if destAgent
	if destAgent != nil && h.worker != nil {
		h.worker.Enqueue(worker.Job{
			ID:   subTask.ID.String(),
			Type: "agent_run",
			Payload: map[string]any{
				"organization_id": claims.OrganizationID.String(),
				"agent_id": destAgent.String(),
				"task_id": subTask.ID.String(),
				"correlation_id": corrID.String(),
				"trigger": "delegated",
			},
		})
	}
	writeData(w, 201, subTask, nil)
}

func (h *TaskHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	tid, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.db.GetTask(claims.OrganizationID, tid); err != nil {
		writeError(w, 404, "NOT_FOUND", "task not found"); return
	}
	events, err := h.db.ListTaskEvents(tid)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, events, nil)
}

func (h *TaskHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	tid, _ := uuid.Parse(chi.URLParam(r, "id"))
	task, err := h.db.UpdateTaskStatus(claims.OrganizationID, tid, "cancelled", "user", claims.UserID, nil)
	if err != nil { writeError(w, 400, "VALIDATION_ERROR", err.Error()); return }
	if h.hub != nil {
		h.hub.BroadcastToOrg(claims.OrganizationID, "agent.task_updated", map[string]any{"task": task})
	}
	writeData(w, 200, task, nil)
}
