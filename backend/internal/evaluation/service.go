package evaluation

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"openagent/internal/repository"
)

// Evaluator defines deterministic checks for agent tasks
type Evaluator interface {
	Evaluate(ctx context.Context, orgID, taskID uuid.UUID) (*Result, error)
}

type Result struct {
	TaskID           uuid.UUID       `json:"taskId"`
	Success          bool            `json:"success"`
	Score            float64         `json:"score"`
	Checks           []CheckResult   `json:"checks"`
	DurationMs       int64           `json:"durationMs"`
	Iterations       int             `json:"iterations"`
	ToolCalls        int             `json:"toolCalls"`
	Delegations      int             `json:"delegations"`
	HumanInterventions int          `json:"humanInterventions"`
	TokenUsage       int             `json:"tokenUsage"`
	EstimatedCost    float64         `json:"estimatedCost"`
	Error            string          `json:"error,omitempty"`
}

type CheckResult struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Details string `json:"details"`
}

type Check interface {
	Name() string
	Run(ctx context.Context, orgID, taskID uuid.UUID) (bool, string, error)
}

type Service struct {
	repo *repository.DB
	checks []Check
}

func New(repo *repository.DB) *Service {
	s := &Service{repo: repo}
	// register deterministic checks
	s.checks = []Check{
		&RequiredToolCheck{repo: repo, requiredTool: ""},
		&ArtifactCheck{repo: repo},
		&TaskCompletedCheck{repo: repo},
	}
	return s
}

func (s *Service) Register(c Check) {
	s.checks = append(s.checks, c)
}

func (s *Service) Evaluate(ctx context.Context, orgID, taskID uuid.UUID) (*Result, error) {
	task, err := s.repo.GetTask(orgID, taskID)
	if err != nil {
		return nil, err
	}
	start := task.CreatedAt
	if start.IsZero() {
		start = time.Now().Add(-1 * time.Minute)
	}
	duration := time.Since(start).Milliseconds()

	// Gather metrics from DB
	var iterations, toolCalls, delegations int
	var tokenUsage int
	var cost float64

	// Agent runs for this task
	if runs, err := s.repo.ListAgentRuns(orgID, nil, &taskID, 50); err == nil {
		for _, r := range runs {
			if len(r.Actions) > iterations {
				iterations = len(r.Actions)
			}
			for _, a := range r.Actions {
				if a.ActionType == "tool_call" {
					toolCalls++
				}
			}
			if r.TokenMetadata != nil {
				if v, ok := r.TokenMetadata["totalTokens"].(float64); ok {
					tokenUsage += int(v)
				}
				if v, ok := r.TokenMetadata["estimatedCost"].(float64); ok {
					cost += v
				}
			}
		}
	}
	// Delegations = child tasks count
	if children, err := s.repo.ListTasks(orgID, &task.ProjectID, nil, "", nil, 100); err == nil {
		for _, c := range children {
			if c.ParentTaskID != nil && *c.ParentTaskID == taskID {
				delegations++
			}
		}
	}
	// Approvals = human interventions
	var interventions int
	_ = s.repo.QueryRow(`SELECT COUNT(*) FROM approvals WHERE organization_id=$1 AND entity_id IN (SELECT id FROM tool_executions WHERE task_id=$2)`, orgID, taskID).Scan(&interventions)

	// Run checks
	var checks []CheckResult
	allPassed := true
	for _, c := range s.checks {
		// Skip generic required tool check if no tool required
		if rc, ok := c.(*RequiredToolCheck); ok && rc.requiredTool == "" {
			continue
		}
		passed, details, _ := c.Run(ctx, orgID, taskID)
		checks = append(checks, CheckResult{Name: c.Name(), Passed: passed, Details: details})
		if !passed {
			allPassed = false
		}
	}
	success := task.Status == "completed" && allPassed
	score := 0.0
	if success {
		score = 1.0
	} else if task.Status == "completed" {
		score = 0.5
	}
	// Bonus for tool usage etc.
	if toolCalls > 0 && success {
		score = 0.9
	}

	return &Result{
		TaskID: taskID,
		Success: success,
		Score: score,
		Checks: checks,
		DurationMs: duration,
		Iterations: iterations,
		ToolCalls: toolCalls,
		Delegations: delegations,
		HumanInterventions: interventions,
		TokenUsage: tokenUsage,
		EstimatedCost: cost,
	}, nil
}

// Built-in checks

type TaskCompletedCheck struct { repo *repository.DB }
func (c *TaskCompletedCheck) Name() string { return "task_completed" }
func (c *TaskCompletedCheck) Run(ctx context.Context, orgID, taskID uuid.UUID) (bool, string, error) {
	task, err := c.repo.GetTask(orgID, taskID)
	if err != nil {
		return false, err.Error(), err
	}
	if task.Status == "completed" {
		return true, "task is completed", nil
	}
	return false, "task status is " + task.Status, nil
}

type RequiredToolCheck struct {
	repo *repository.DB
	requiredTool string
}
func (c *RequiredToolCheck) Name() string { return "required_tool_" + c.requiredTool }
func (c *RequiredToolCheck) Run(ctx context.Context, orgID, taskID uuid.UUID) (bool, string, error) {
	if c.requiredTool == "" {
		return true, "no required tool", nil
	}
	var count int
	err := c.repo.QueryRow(`SELECT COUNT(*) FROM tool_executions WHERE task_id=$1 AND tool_name=$2 AND status='succeeded'`, taskID, c.requiredTool).Scan(&count)
	if err != nil {
		return false, err.Error(), err
	}
	if count > 0 {
		return true, c.requiredTool + " was used", nil
	}
	return false, c.requiredTool + " not used", nil
}

type ArtifactCheck struct { repo *repository.DB }
func (c *ArtifactCheck) Name() string { return "artifact_created" }
func (c *ArtifactCheck) Run(ctx context.Context, orgID, taskID uuid.UUID) (bool, string, error) {
	// Check if task has child tasks or messages indicating artifact
	var childCount int
	_ = c.repo.QueryRow(`SELECT COUNT(*) FROM tasks WHERE parent_task_id=$1`, taskID).Scan(&childCount)
	var msgCount int
	task, _ := c.repo.GetTask(orgID, taskID)
	if task != nil && task.ChannelID != nil {
		_ = c.repo.QueryRow(`SELECT COUNT(*) FROM messages WHERE channel_id=$1 AND created_at >= $2`, *task.ChannelID, task.CreatedAt).Scan(&msgCount)
	}
	if childCount > 0 || msgCount > 0 {
		return true, "artifact evidence found", nil
	}
	return false, "no artifact evidence", nil
}

// Wrapper for sql.Row scanning
func scanCount(row *sql.Row) int {
	var c int
	_ = row.Scan(&c)
	return c
}
