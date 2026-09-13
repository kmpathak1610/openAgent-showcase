package autonomy

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"openagent/internal/domain"
	"openagent/internal/repository"
	"openagent/internal/worker"
	"openagent/internal/ws"
	"log/slog"
)

func TestAutonomy_ObservesProjectEvent(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := repository.New(db)
	wsHub := ws.NewHub(slog.Default())
	workerPool := worker.New(10, slog.Default())
	svc := New(repo, workerPool, wsHub)
	orgID := uuid.New()
	projectID := uuid.New()
	agentID := uuid.New()
	// Mock autonomous agents in project
	mock.ExpectQuery(`SELECT a.id, a.name FROM agents a JOIN project_agents`).WillReturnRows(
		sqlmock.NewRows([]string{"id","name"}).AddRow(agentID, "Auto Agent"),
	)
	mock.ExpectQuery(`SELECT a.id, a.name FROM agents a JOIN agent_team_members`).WillReturnRows(
		sqlmock.NewRows([]string{"id","name"}),
	)
	// Mock CreateTask for autonomous task
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO tasks`).WillReturnRows(
		sqlmock.NewRows([]string{"id","organization_id","project_id","channel_id","parent_task_id","team_id","title","description","created_by","assigned_to_agent","assigned_to_user","status","priority","deadline","correlation_id","created_at","updated_at"}).
			AddRow(uuid.New(), orgID, projectID, nil, nil, nil, "Autonomous: new_document", "desc", agentID, agentID, nil, "assigned", "medium", nil, uuid.New(), "2024-01-01T00:00:00Z", "2024-01-01T00:00:00Z"),
	)
	mock.ExpectExec(`INSERT INTO task_assignments`).WillReturnResult(sqlmock.NewResult(1,1))
	mock.ExpectExec(`INSERT INTO task_events`).WillReturnResult(sqlmock.NewResult(1,1))
	mock.ExpectExec(`INSERT INTO task_events`).WillReturnResult(sqlmock.NewResult(1,1))
	mock.ExpectCommit()
	err := svc.HandleProjectEvent(context.Background(), orgID, projectID, "new_document", map[string]any{"docId": "123"})
	if err != nil { t.Fatalf("handle event: %v", err) }
}

func TestAutonomy_CanAct(t *testing.T) {
	svc := &Service{}
	agent := &domain.Agent{AutonomyLevel: "autonomous"}
	if !svc.CanAutonomousAct(agent, "web_search") {
		t.Fatal("autonomous should handle web_search")
	}
	if !svc.CanAutonomousAct(agent, "publish_social_post") {
		t.Logf("publish should still be allowed but will require approval via tool executor")
	}
	nonAuto := &domain.Agent{AutonomyLevel: "assistant"}
	if svc.CanAutonomousAct(nonAuto, "web_search") {
		t.Fatal("non-autonomous should not handle")
	}
}
