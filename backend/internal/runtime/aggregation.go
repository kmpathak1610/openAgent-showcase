package runtime

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"openagent/internal/security"
)

func (r *Runtime) CheckParentAggregation(ctx context.Context, orgID, parentID uuid.UUID) error {
	parent, err := r.repo.GetTask(orgID, parentID)
	if err != nil {
		return err
	}
	if parent.Status == "completed" || parent.Status == "failed" || parent.Status == "cancelled" {
		return nil
	}
	children, err := r.repo.ListTasks(orgID, &parent.ProjectID, nil, "", nil, 100)
	if err != nil {
		return err
	}
	hasPending := false
	allCompleted := 0
	totalChildren := 0
	for _, c := range children {
		if c.ParentTaskID != nil && *c.ParentTaskID == parentID {
			totalChildren++
			if c.Status != "completed" && c.Status != "failed" && c.Status != "cancelled" {
				hasPending = true
			}
			if c.Status == "completed" {
				allCompleted++
			}
		}
	}
	if totalChildren == 0 {
		return nil
	}
	if hasPending {
		return nil
	}
	if allCompleted > 0 {
		assignee := uuid.Nil
		if parent.AssignedToAgent != nil {
			assignee = *parent.AssignedToAgent
		}
		_, err = r.repo.UpdateTaskStatus(orgID, parentID, "completed", "agent", assignee, map[string]any{"aggregated": allCompleted})
		if err != nil {
			return fmt.Errorf("update parent: %w", err)
		}
		if parent.ChannelID != nil {
			msg := fmt.Sprintf("✅ Parent **%s** completed — %d/%d subtasks done", parent.Title, allCompleted, totalChildren)
			threadID := r.resolveThreadID(orgID, parent)
			if m, _ := r.repo.CreateMessage(orgID, *parent.ChannelID, threadID, "agent", parent.AssignedToAgent, security.SanitizeUserContent(msg)); m != nil && r.hub != nil {
				r.hub.BroadcastToOrg(orgID, "agent.completed", map[string]any{"taskId": parentID.String(), "aggregated": true})
				r.hub.BroadcastToOrg(orgID, "agent.message", map[string]any{"message": m})
			}
		}
	}
	return nil
}
