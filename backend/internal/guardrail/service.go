package guardrail

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"openagent/internal/repository"
)

type Service struct {
	repo *repository.DB
}

func New(repo *repository.DB) *Service { return &Service{repo: repo} }

// CheckTaskCreation checks execution budgets, delegation depth, etc., before allowing new task
func (s *Service) CheckTaskCreation(orgID, agentID uuid.UUID, teamID *uuid.UUID, delegationDepth int) error {
	// Fetch budget for agent or team
	var budget struct {
		MaxTasksPerDay int
		MaxDelegationDepth int
	}
	// Default budgets
	budget.MaxTasksPerDay = 100
	budget.MaxDelegationDepth = 5
	// Try to get from execution_budgets
	var maxTasks, maxDepth sql.NullInt32
	if agentID != uuid.Nil {
		_ = s.repo.QueryRow(`SELECT max_tasks_per_day, max_delegation_depth FROM execution_budgets WHERE organization_id=$1 AND agent_id=$2`, orgID, agentID).Scan(&maxTasks, &maxDepth)
		if maxTasks.Valid { budget.MaxTasksPerDay = int(maxTasks.Int32) }
		if maxDepth.Valid { budget.MaxDelegationDepth = int(maxDepth.Int32) }
	}
	if teamID != nil {
		_ = s.repo.QueryRow(`SELECT max_tasks_per_day, max_delegation_depth FROM execution_budgets WHERE organization_id=$1 AND team_id=$2`, orgID, *teamID).Scan(&maxTasks, &maxDepth)
		if maxTasks.Valid { budget.MaxTasksPerDay = int(maxTasks.Int32) }
		if maxDepth.Valid { budget.MaxDelegationDepth = int(maxDepth.Int32) }
	}
	if delegationDepth > budget.MaxDelegationDepth {
		return fmt.Errorf("max delegation depth %d exceeded", budget.MaxDelegationDepth)
	}
	if delegationDepth > 5 { // hard global limit to prevent loops
		return fmt.Errorf("global max delegation depth 5 exceeded")
	}
	// Check daily limit: count tasks created by this agent today
	var count int
	_ = s.repo.QueryRow(`SELECT COUNT(*) FROM tasks WHERE organization_id=$1 AND assigned_to_agent=$2 AND created_at > now() - interval '1 day'`, orgID, agentID).Scan(&count)
	if count >= budget.MaxTasksPerDay {
		return fmt.Errorf("daily task limit %d exceeded for agent", budget.MaxTasksPerDay)
	}
	return nil
}

func (s *Service) CheckToolCall(orgID, agentID uuid.UUID, toolName string) error {
	// Rate limit per minute
	var count int
	_ = s.repo.QueryRow(`SELECT COUNT(*) FROM tool_executions WHERE organization_id=$1 AND agent_id=$2 AND created_at > now() - interval '1 minute'`, orgID, agentID).Scan(&count)
	var limit int = 60
	_ = s.repo.QueryRow(`SELECT rate_limit_per_minute FROM execution_budgets WHERE organization_id=$1 AND agent_id=$2`, orgID, agentID).Scan(&limit)
	if limit == 0 { limit = 60 }
	if count >= limit {
		return fmt.Errorf("rate limit %d per minute exceeded for tool %s", limit, toolName)
	}
	// Tool daily limit
	var toolCount int
	_ = s.repo.QueryRow(`SELECT COUNT(*) FROM tool_executions WHERE organization_id=$1 AND agent_id=$2 AND tool_name=$3 AND created_at > now() - interval '1 day'`, orgID, agentID, toolName).Scan(&toolCount)
	var maxTools int = 500
	_ = s.repo.QueryRow(`SELECT max_tool_calls_per_day FROM execution_budgets WHERE organization_id=$1 AND agent_id=$2`, orgID, agentID).Scan(&maxTools)
	if maxTools == 0 { maxTools = 500 }
	if toolCount >= maxTools {
		return fmt.Errorf("daily tool limit %d exceeded", maxTools)
	}
	return nil
}

func (s *Service) CheckTimeout(timeoutSeconds int) error {
	if timeoutSeconds > 600 { return fmt.Errorf("timeout exceeds max 600s") }
	if timeoutSeconds <=0 { return fmt.Errorf("timeout must be positive") }
	return nil
}

// GetAgentStatus computes status based on active runs
func (s *Service) GetAgentStatus(orgID, agentID uuid.UUID) (string, error) {
	// Check for approval_required
	var count int
	_ = s.repo.QueryRow(`SELECT COUNT(*) FROM agent_runs WHERE organization_id=$1 AND agent_id=$2 AND status='awaiting_approval'`, orgID, agentID).Scan(&count)
	if count >0 { return "approval_required", nil }
	_ = s.repo.QueryRow(`SELECT COUNT(*) FROM agent_runs WHERE organization_id=$1 AND agent_id=$2 AND status='running'`, orgID, agentID).Scan(&count)
	if count >0 { return "working", nil }
	_ = s.repo.QueryRow(`SELECT COUNT(*) FROM tasks WHERE organization_id=$1 AND assigned_to_agent=$2 AND status IN ('waiting','blocked')`, orgID, agentID).Scan(&count)
	if count >0 { return "waiting", nil }
	_ = s.repo.QueryRow(`SELECT COUNT(*) FROM tasks WHERE organization_id=$1 AND assigned_to_agent=$2 AND status='blocked'`, orgID, agentID).Scan(&count)
	if count >0 { return "blocked", nil }
	_ = s.repo.QueryRow(`SELECT COUNT(*) FROM agent_runs WHERE organization_id=$1 AND agent_id=$2 AND status='failed' AND created_at > now() - interval '1 hour'`, orgID, agentID).Scan(&count)
	if count >0 { return "failed", nil }
	// Check offline: no recent activity in 5m and no runs
	var lastSeen sql.NullTime
	_ = s.repo.QueryRow(`SELECT last_seen_at FROM users WHERE id=$1`, agentID).Scan(&lastSeen)
	// For agent, we check last_status_at
	_ = s.repo.QueryRow(`SELECT last_status_at FROM agents WHERE id=$1`, agentID).Scan(&lastSeen)
	if !lastSeen.Valid || time.Since(lastSeen.Time) > 5*time.Minute {
		return "offline", nil
	}
	return "idle", nil
}
