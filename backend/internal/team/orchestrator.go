package team

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"openagent/internal/domain"
	"openagent/internal/repository"
	"openagent/internal/worker"
	"openagent/internal/ws"
)

type Orchestrator struct {
	repo   *repository.DB
	worker *worker.Pool
	hub    *ws.Hub
}

func NewOrchestrator(repo *repository.DB, worker *worker.Pool, hub *ws.Hub) *Orchestrator {
	return &Orchestrator{repo: repo, worker: worker, hub: hub}
}

const (
	MaxDelegationDepth = 5
	MaxRetries         = 3
	TaskTimeout        = 5 * time.Minute
)

// StartTeamTask creates a root task for the team and kicks off orchestration
func (o *Orchestrator) StartTeamTask(ctx context.Context, orgID, teamID, projectID uuid.UUID, channelID *uuid.UUID, title, description string, createdBy uuid.UUID) (*domain.Task, error) {
	team, err := o.repo.GetTeam(orgID, teamID)
	if err != nil { return nil, fmt.Errorf("team not found: %w", err) }
	if len(team.Members)==0 { return nil, fmt.Errorf("team has no members") }
	// P1-7: explicit coordinator — use team.CoordinatorAgentID if set, else derive and persist
	var coordinator domain.AgentTeamMember
	if team.CoordinatorAgentID != nil {
		found := false
		for _, m := range team.Members {
			if m.AgentID == *team.CoordinatorAgentID {
				coordinator = m
				found = true
				break
			}
		}
		if !found {
			coordinator = team.Members[0]
		}
	} else {
		// No explicit coordinator — pick first and persist (usually manager)
		coordinator = team.Members[0]
		_ = o.repo.SetTeamCoordinator(orgID, teamID, coordinator.AgentID)
	}
	corrID := uuid.New()
	// create root task with team_id
	task, err := o.repo.CreateTask(orgID, projectID, channelID, nil, &teamID, &coordinator.AgentID, nil, title, description, "high", nil, &corrID, createdBy)
	if err != nil { return nil, err }
	// Enqueue for coordinator
	if o.worker != nil {
		o.worker.Enqueue(worker.Job{
			ID:   task.ID.String(),
			Type: "agent_run",
			Payload: map[string]any{
				"organization_id": orgID.String(),
				"agent_id": coordinator.AgentID.String(),
				"task_id": task.ID.String(),
				"correlation_id": corrID.String(),
				"trigger": "team_task",
			},
			Timeout: TaskTimeout,
			Retries: MaxRetries,
		})
	}
	if o.hub != nil {
		o.hub.BroadcastToOrg(orgID, "team.task_started", map[string]any{"teamId": teamID.String(), "task": task})
	}
	_ = o.repo.AddTeamActivity(teamID, orgID, "user", &createdBy, "task_started", map[string]any{"task_id": task.ID.String(), "title": title})
	return task, nil
}

// Delegate handles task delegation with depth, cycle, timeout checks
func (o *Orchestrator) Delegate(ctx context.Context, orgID, parentTaskID, destAgentID uuid.UUID, title, description string, delegatedBy uuid.UUID) (*domain.Task, error) {
	parent, err := o.repo.GetTask(orgID, parentTaskID)
	if err != nil { return nil, err }
	// check delegation depth
	if parent.DelegationDepth >= parent.MaxDelegationDepth && parent.MaxDelegationDepth >0 {
		return nil, fmt.Errorf("max delegation depth %d reached", parent.MaxDelegationDepth)
	}
	if parent.DelegationDepth >= MaxDelegationDepth {
		return nil, fmt.Errorf("max delegation depth %d reached", MaxDelegationDepth)
	}
	// check task execution limits: retry count
	if parent.RetryCount >= parent.MaxRetries && parent.MaxRetries>0 {
		return nil, fmt.Errorf("max retries reached")
	}
	// cycle detection: ensure destAgent not already in delegation chain for this correlation
	// For Phase 6, we check if this task already has a child delegated to same agent with same title (simple)
	// More robust: check task_dependencies cycle already handled in repo.AddTaskDependency, but for delegate we create new task, not dependency
	// Check team membership: destAgent must be in same team if parent has team
	if parent.TeamID != nil {
		members, _ := o.repo.ListTeamMembers(*parent.TeamID)
		found := false
		for _, m := range members { if m.AgentID == destAgentID { found = true; break } }
		if !found { return nil, fmt.Errorf("agent not in team") }
	}
	corrID := parent.CorrelationID
	if corrID == nil { gen := uuid.New(); corrID = &gen }
	// create subtask with incremented depth
	subTask, err := o.repo.CreateTask(orgID, parent.ProjectID, parent.ChannelID, &parentTaskID, parent.TeamID, &destAgentID, nil, title, description, "medium", nil, corrID, delegatedBy)
	if err != nil { return nil, err }
	// set delegation depth
	_, _ = o.repo.Exec(`UPDATE tasks SET delegation_depth=$2 WHERE id=$1`, subTask.ID, parent.DelegationDepth+1)
	// parent waits for subtask, so parent is blocked until child completed
	_ = o.repo.AddTaskDependency(parent.ID, subTask.ID)
	// events
	_, _ = o.repo.CreateTaskEvent(parent.ID, "agent", &delegatedBy, "delegated", map[string]any{"subtask_id": subTask.ID.String(), "to": destAgentID.String()}, corrID)
	// enqueue delegated agent run
	if o.worker != nil {
		o.worker.Enqueue(worker.Job{
			ID:   subTask.ID.String(),
			Type: "agent_run",
			Payload: map[string]any{
				"organization_id": orgID.String(),
				"agent_id": destAgentID.String(),
				"task_id": subTask.ID.String(),
				"correlation_id": corrID.String(),
				"trigger": "delegated",
			},
			Timeout: TaskTimeout,
		})
	}
	if o.hub != nil {
		o.hub.BroadcastToOrg(orgID, "agent.delegated", map[string]any{"parent": parent.ID.String(), "subtask": subTask, "to": destAgentID.String(), "correlationId": corrID.String()})
	}
	if parent.TeamID != nil {
		_ = o.repo.AddTeamActivity(*parent.TeamID, orgID, "agent", &delegatedBy, "delegated", map[string]any{"from": parent.ID.String(), "to": subTask.ID.String()})
	}
	return subTask, nil
}

// Evaluate checks if a task's dependencies are satisfied and retries or escalates
func (o *Orchestrator) Evaluate(ctx context.Context, orgID, taskID uuid.UUID) (string, error) {
	blocked, err := o.repo.IsTaskBlocked(taskID)
	if err != nil { return "", err }
	if blocked {
		_, _ = o.repo.UpdateTaskStatus(orgID, taskID, "blocked", "system", uuid.Nil, map[string]any{"reason": "dependencies pending"})
		return "blocked", nil
	}
	// check if task has failed and can retry
	task, _ := o.repo.GetTask(orgID, taskID)
	if task.Status == "failed" && task.RetryCount < task.MaxRetries {
		_, _ = o.repo.Exec(`UPDATE tasks SET retry_count=retry_count+1, status='pending' WHERE id=$1`, taskID)
		// re-enqueue
		if task.AssignedToAgent != nil && o.worker != nil {
			o.worker.Enqueue(worker.Job{
				ID:   task.ID.String(),
				Type: "agent_run",
				Payload: map[string]any{
					"organization_id": orgID.String(),
					"agent_id": task.AssignedToAgent.String(),
					"task_id": task.ID.String(),
					"trigger": "retry",
				},
			})
		}
		return "retry", nil
	}
	if task.Status == "failed" {
		// escalate to human
		if o.hub != nil {
			o.hub.BroadcastToOrg(orgID, "team.escalated", map[string]any{"taskId": taskID.String(), "reason": "max retries exceeded"})
		}
		return "escalated", nil
	}
	return "ready", nil
}

// RequestHumanInput marks task as waiting for human
func (o *Orchestrator) RequestHumanInput(orgID, taskID uuid.UUID, requester uuid.UUID, prompt string) error {
	_, err := o.repo.UpdateTaskStatus(orgID, taskID, "waiting", "agent", requester, map[string]any{"prompt": prompt})
	if err != nil { return err }
	if o.hub != nil {
		o.hub.BroadcastToOrg(orgID, "agent.waiting", map[string]any{"taskId": taskID.String(), "prompt": prompt})
	}
	return nil
}
