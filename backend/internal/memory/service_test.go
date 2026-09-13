package memory

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"openagent/internal/repository"
)

func mustParseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

func TestMemory_Policy_ConversationLowImportanceRejected(t *testing.T) {
	svc := &Service{}
	_, err := svc.Create(context.Background(), uuid.New(), nil, nil, "conversation", "conversation", "interaction", "hello", 0.1, nil, nil)
	if err == nil {
		t.Fatal("expected low importance rejection")
	}
	if err.Error() != "conversation memory importance too low (0.10 < 0.30)" {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestMemory_Types(t *testing.T) {
	cases := []struct {
		memoryType string
		scope      string
		needAgent  bool
		needProject bool
		valid      bool
	}{
		{"working", "task", false, false, true},
		{"project", "project", false, true, true},
		{"agent", "agent", true, false, true},
		{"conversation", "conversation", false, false, true},
		{"invalid", "agent", false, false, false},
	}
	for _, tc := range cases {
		db, mock, _ := sqlmock.New()
		repo := repository.New(db)
		svc := &Service{repo: repo}
		var agentID *uuid.UUID
		var projectID *uuid.UUID
		if tc.needAgent { uid := uuid.New(); agentID = &uid }
		if tc.needProject { uid := uuid.New(); projectID = &uid }
		if tc.valid {
			// Exact hash dedup check - returns no rows
			mock.ExpectQuery(`SELECT id, content, importance, confidence, status FROM memories`).WillReturnRows(
				sqlmock.NewRows([]string{"id","content","importance","confidence","status"}),
			)
			// Candidate search - empty
			mock.ExpectQuery(`SELECT id, organization_id, agent_id, project_id, conversation_id`).WillReturnRows(
				sqlmock.NewRows([]string{"id","organization_id","agent_id","project_id","conversation_id","task_id","memory_type","scope","source","source_event_id","content","importance","confidence","status","metadata","expires_at","last_accessed_at","last_confirmed_at","content_hash","idempotency_key","superseded_by","version","created_at","updated_at"}),
			)
			// Insert new memory - return 24 columns
			now := mustParseTime("2024-01-01T00:00:00Z")
			mock.ExpectQuery(`INSERT INTO memories`).WillReturnRows(
				sqlmock.NewRows([]string{"id","organization_id","agent_id","project_id","conversation_id","task_id","memory_type","scope","source","source_event_id","content","importance","confidence","status","metadata","expires_at","last_accessed_at","last_confirmed_at","content_hash","idempotency_key","superseded_by","version","created_at","updated_at"}).
					AddRow(uuid.New(), uuid.New(), nil, nil, nil, nil, tc.memoryType, tc.scope, "manual", nil, "test", 0.5, 0.5, "active", []byte(`{}`), nil, now, now, "hash", "idem", nil, 1, now, now),
			)
			// Memory event and audit log inserts (best effort)
			mock.ExpectExec(`INSERT INTO memory_events`).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec(`INSERT INTO audit_logs`).WillReturnResult(sqlmock.NewResult(1, 1))

			_, err := svc.Create(context.Background(), uuid.New(), agentID, projectID, tc.memoryType, tc.scope, "manual", "test", 0.5, nil, nil)
			if err != nil {
				t.Fatalf("valid type %s should not fail: %v", tc.memoryType, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet expectations for %s: %v", tc.memoryType, err)
			}
		} else {
			_, err := svc.Create(context.Background(), uuid.New(), nil, nil, tc.memoryType, tc.scope, "manual", "test", 0.5, nil, nil)
			if err == nil {
				t.Fatalf("invalid type %s should fail", tc.memoryType)
			}
		}
		db.Close()
	}
}
