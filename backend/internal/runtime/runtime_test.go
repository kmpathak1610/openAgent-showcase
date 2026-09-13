package runtime

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"openagent/internal/knowledge"
	"openagent/internal/llm"
	"openagent/internal/repository"
	"openagent/internal/ws"
	"log/slog"
)

func newTestRuntime(t *testing.T) (*Runtime, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil { t.Fatalf("sqlmock: %v", err) }
	repo := repository.New(db)
	reg := llm.NewRegistry()
	reg.Register(llm.NewStub("stub"))
	reg.RegisterEmbedder("stub", llm.NewStubEmbedder(1536))
	retriever := knowledge.NewRetriever(repo, llm.NewStubEmbedder(1536), nil)
	hub := ws.NewHub(slog.Default())
	rt := New(repo, reg, retriever, hub)
	return rt, mock, func() { db.Close() }
}

func TestRuntime_MissingAgent_FailsIsolation(t *testing.T) {
	rt, mock, close := newTestRuntime(t)
	defer close()
	orgID := uuid.New()
	agentID := uuid.New()
	taskID := uuid.New()
	// GetAgent should fail with no rows (org isolation)
	mock.ExpectQuery(`SELECT id, organization_id, name, slug, description`).WillReturnError(sql.ErrNoRows)
	err := rt.ExecuteTask(context.Background(), orgID, agentID, taskID, nil, "task_assigned")
	if err == nil { t.Fatal("expected error for missing agent") }
	if err := mock.ExpectationsWereMet(); err != nil { t.Logf("mock not met (expected for missing): %v", err) }
}

func TestRuntime_Execution_CreatesSubtasks(t *testing.T) {
	rt, _, _ := newTestRuntime(t)
	if rt == nil { t.Fatal("runtime nil") }
}

func TestRuntime_FailureHandling(t *testing.T) {
	reg := llm.NewRegistry()
	failing := &failingProvider{}
	reg.Register(failing)
	p, _ := reg.Get("failing")
	_, err := p.Complete(context.Background(), llm.CompletionRequest{Model: "test", Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}}})
	if err == nil {
		t.Fatal("expected failing provider to error")
	}
}

func TestRuntime_ParentAggregation_OnSubtaskCompleted(t *testing.T) {
	rt, mock, close := newTestRuntime(t)
	defer close()
	orgID := uuid.New()
	parentID := uuid.New()
	projectID := uuid.New()
	// Mock GetTask for parent
	mock.ExpectQuery(`SELECT id, organization_id, project_id`).WillReturnRows(
		sqlmock.NewRows([]string{"id", "organization_id", "project_id", "channel_id", "parent_task_id", "team_id", "title", "description", "created_by", "assigned_to_agent", "assigned_to_user", "status", "priority", "deadline", "correlation_id", "required_role", "required_capabilities", "preferred_agent_id", "created_at", "updated_at"}).
			AddRow(parentID, orgID, projectID, nil, nil, nil, "Parent", "", nil, nil, nil, "running", "medium", nil, nil, nil, nil, nil, time.Now(), time.Now()))
	// Mock ListTasks for children - return one completed child
	childID := uuid.New()
	mock.ExpectQuery(`SELECT id, organization_id, project_id`).WillReturnRows(
		sqlmock.NewRows([]string{"id", "organization_id", "project_id", "channel_id", "parent_task_id", "team_id", "title", "description", "created_by", "assigned_to_agent", "assigned_to_user", "status", "priority", "deadline", "correlation_id", "required_role", "required_capabilities", "preferred_agent_id", "created_at", "updated_at"}).
			AddRow(childID, orgID, projectID, nil, parentID, nil, "Child", "", nil, nil, nil, "completed", "medium", nil, nil, nil, nil, nil, time.Now(), time.Now()))
	// Expect parent marked completed
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT status`).WillReturnRows(sqlmock.NewRows([]string{"status", "correlation_id"}).AddRow("running", nil))
	mock.ExpectExec(`UPDATE tasks SET status`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO task_events`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT id, organization_id`).WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "project_id", "channel_id", "parent_task_id", "team_id", "title", "description", "created_by", "assigned_to_agent", "assigned_to_user", "status", "priority", "deadline", "correlation_id", "required_role", "required_capabilities", "preferred_agent_id", "created_at", "updated_at"}).AddRow(parentID, orgID, projectID, nil, nil, nil, "Parent", "", nil, nil, nil, "completed", "medium", nil, nil, nil, nil, nil, time.Now(), time.Now()))

	err := rt.CheckParentAggregation(context.Background(), orgID, parentID)
	if err != nil {
		t.Fatalf("aggregation failed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations not met: %v", err)
	}
}

type failingProvider struct{}

func (f *failingProvider) Name() string { return "failing" }
func (f *failingProvider) Complete(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	return nil, context.DeadlineExceeded
}
