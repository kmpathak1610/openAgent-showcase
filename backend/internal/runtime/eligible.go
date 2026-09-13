package runtime

import (
	"fmt"
	"github.com/google/uuid"
	"openagent/internal/domain"
)

func (r *Runtime) tryEligibleAssignment(orgID, projectID uuid.UUID, task *domain.Task, run *domain.AgentRun, seq int, correlationID *uuid.UUID) (*uuid.UUID, bool) {
	eligible, _ := r.repo.GetEligibleAgents(orgID, projectID, task.RequiredRole, task.RequiredCapabilities)
	if len(eligible) == 0 {
		if task.RequiredRole != nil || len(task.RequiredCapabilities) > 0 {
			return nil, false
		}
		return nil, false
	}
	// Prefer preferred agent
	if task.PreferredAgentID != nil {
		for _, e := range eligible {
			if e.ID == *task.PreferredAgentID {
				return &e.ID, true
			}
		}
	}
	// Pick best by capability overlap
	best := eligible[0]
	bestScore := -1
	for _, cand := range eligible {
		score := 0
		if task.RequiredRole != nil && cand.Version != nil && cand.Version.Role == *task.RequiredRole {
			score += 10
		}
		for _, rc := range task.RequiredCapabilities {
			for _, cc := range cand.Capabilities {
				if cc == rc {
					score += 5
				}
			}
			if cand.Version != nil {
				for _, cc := range cand.Version.Capabilities {
					if cc.Name == rc {
						score += 5
					}
				}
			}
		}
		if score > bestScore {
			bestScore = score
			best = cand
		}
	}
	return &best.ID, true
}

func (r *Runtime) validateEligibleOrNeedsHuman(orgID uuid.UUID, task *domain.Task, destID uuid.UUID) error {
	eligible, _ := r.repo.GetEligibleAgents(orgID, task.ProjectID, task.RequiredRole, task.RequiredCapabilities)
	for _, e := range eligible {
		if e.ID == destID {
			return nil
		}
	}
	// Also check if dest is project-assigned even if not in eligible due to capability mismatch
	// For strict P1-6, if required capabilities not met, fail
	if task.RequiredRole != nil || len(task.RequiredCapabilities) > 0 {
		return fmt.Errorf("agent %s not eligible for task requirements", destID.String()[:8])
	}
	return nil
}
