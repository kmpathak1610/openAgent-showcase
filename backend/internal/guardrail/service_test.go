package guardrail

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"openagent/internal/repository"
)

func TestGuardrail_CheckTaskCreation_MaxDepth(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := repository.New(db)
	svc := New(repo)
	orgID := uuid.New()
	agentID := uuid.New()
	mock.ExpectQuery(`SELECT max_tasks_per_day, max_delegation_depth FROM execution_budgets WHERE organization_id=.+ AND agent_id=.+`).WillReturnRows(
		sqlmock.NewRows([]string{"max_tasks_per_day","max_delegation_depth"}).AddRow(int32(100), int32(2)),
	)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM tasks WHERE organization_id=.+ AND assigned_to_agent=.+ AND created_at > now`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	if err := svc.CheckTaskCreation(orgID, agentID, nil, 2); err != nil {
		t.Fatalf("should allow depth 2: %v", err)
	}
	mock.ExpectQuery(`SELECT max_tasks_per_day, max_delegation_depth FROM execution_budgets WHERE organization_id=.+ AND agent_id=.+`).WillReturnRows(
		sqlmock.NewRows([]string{"max_tasks_per_day","max_delegation_depth"}).AddRow(int32(100), int32(2)),
	)
	if err := svc.CheckTaskCreation(orgID, agentID, nil, 3); err == nil {
		t.Fatal("should fail depth 3 exceeds max 2")
	}
}

func TestGuardrail_CheckToolCall_RateLimit(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := repository.New(db)
	svc := New(repo)
	orgID := uuid.New()
	agentID := uuid.New()
	// Mock count for tool executions in last minute = 60 (limit)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM tool_executions WHERE organization_id=.+ AND agent_id=.+ AND created_at > now`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(60))
	mock.ExpectQuery(`SELECT rate_limit_per_minute FROM execution_budgets WHERE organization_id=.+ AND agent_id=.+`).WillReturnRows(sqlmock.NewRows([]string{"rate_limit_per_minute"}).AddRow(60))
	if err := svc.CheckToolCall(orgID, agentID, "web_search"); err == nil {
		t.Fatal("should fail rate limit")
	}
	// With count 0, should pass
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM tool_executions WHERE organization_id=.+ AND agent_id=.+ AND created_at > now`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT rate_limit_per_minute FROM execution_budgets WHERE organization_id=.+ AND agent_id=.+`).WillReturnRows(sqlmock.NewRows([]string{"rate_limit_per_minute"}).AddRow(60))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM tool_executions WHERE organization_id=.+ AND agent_id=.+ AND tool_name=.+ AND created_at > now`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT max_tool_calls_per_day FROM execution_budgets WHERE organization_id=.+ AND agent_id=.+`).WillReturnRows(sqlmock.NewRows([]string{"max_tool_calls_per_day"}).AddRow(500))
	if err := svc.CheckToolCall(orgID, agentID, "web_search"); err != nil {
		t.Fatalf("should pass: %v", err)
	}
}

func TestGuardrail_AgentStatus(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := repository.New(db)
	svc := New(repo)
	orgID := uuid.New()
	agentID := uuid.New()
	// Mock awaiting_approval count 1
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_runs WHERE organization_id=.+ AND agent_id=.+ AND status='awaiting_approval'`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	status, _ := svc.GetAgentStatus(orgID, agentID)
	if status != "approval_required" { t.Fatalf("expected approval_required, got %s", status) }
	// Mock no awaiting, but running
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_runs WHERE organization_id=.+ AND agent_id=.+ AND status='awaiting_approval'`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_runs WHERE organization_id=.+ AND agent_id=.+ AND status='running'`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	status, _ = svc.GetAgentStatus(orgID, agentID)
	if status != "working" { t.Fatalf("expected working, got %s", status) }
}
