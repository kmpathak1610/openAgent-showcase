package approval

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"openagent/internal/repository"
)

func TestApproval_CreateAndDecide(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil { t.Fatal(err) }
	defer db.Close()
	repo := repository.New(db)
	svc := New(repo)
	orgID := uuid.New()
	agentID := uuid.New()
	entityID := uuid.New()

	// Create
	mock.ExpectQuery(`INSERT INTO approvals`).WillReturnRows(
		sqlmock.NewRows([]string{"id","organization_id","requester_type","requester_agent_id","requester_user_id","entity_type","entity_id","title","description","payload","risk_level","action","target","context","status","decided_by","decided_at","expires_at","created_at","updated_at"}).
			AddRow(uuid.New(), orgID, "agent", agentID, nil, "tool_execution", entityID, "Approve publish", "desc", []byte(`{}`), "high", "publish_social_post", "linkedin", []byte(`{}`), "pending", nil, nil, time.Now().Add(24*time.Hour), time.Now(), time.Now()),
	)
	approval, err := svc.Create(orgID, "agent", &agentID, nil, "tool_execution", entityID, "Approve publish", "desc", map[string]any{"tool":"publish"}, "high", "publish_social_post", "linkedin", map[string]any{}, 24*time.Hour)
	if err != nil || approval.Status != "pending" { t.Fatalf("create failed: %v", err) }

	// Decide approve
	mock.ExpectQuery(`SELECT status, expires_at FROM approvals WHERE id=.+`).WillReturnRows(
		sqlmock.NewRows([]string{"status","expires_at"}).AddRow("pending", time.Now().Add(24*time.Hour)),
	)
	mock.ExpectQuery(`UPDATE approvals SET status=.+`).WillReturnRows(
		sqlmock.NewRows([]string{"id","organization_id","requester_type","requester_agent_id","requester_user_id","entity_type","entity_id","title","description","payload","risk_level","action","target","context","status","decided_by","decided_at","expires_at","created_at","updated_at"}).
			AddRow(approval.ID, orgID, "agent", agentID, nil, "tool_execution", entityID, "Approve publish", "desc", []byte(`{}`), "high", "publish_social_post", "linkedin", []byte(`{}`), "approved", uuid.New(), time.Now(), time.Now().Add(24*time.Hour), time.Now(), time.Now()),
	)
	decided, err := svc.Decide(orgID, approval.ID, uuid.New(), "approved")
	if err != nil || decided.Status != "approved" { t.Fatalf("approve failed: %v", err) }

	// Try to decide again should fail (already decided)
	mock.ExpectQuery(`SELECT status, expires_at FROM approvals WHERE id=.+`).WillReturnRows(
		sqlmock.NewRows([]string{"status","expires_at"}).AddRow("approved", time.Now().Add(24*time.Hour)),
	)
	_, err = svc.Decide(orgID, approval.ID, uuid.New(), "rejected")
	if err == nil { t.Fatal("should fail already decided") }
}

func TestApproval_Rejection(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := repository.New(db)
	svc := New(repo)
	orgID := uuid.New()
	agentID := uuid.New()
	entityID := uuid.New()
	mock.ExpectQuery(`INSERT INTO approvals`).WillReturnRows(
		sqlmock.NewRows([]string{"id","organization_id","requester_type","requester_agent_id","requester_user_id","entity_type","entity_id","title","description","payload","risk_level","action","target","context","status","decided_by","decided_at","expires_at","created_at","updated_at"}).
			AddRow(uuid.New(), orgID, "agent", agentID, nil, "tool_execution", entityID, "Title", "desc", []byte(`{}`), "high", "act", "tgt", []byte(`{}`), "pending", nil, nil, time.Now().Add(24*time.Hour), time.Now(), time.Now()),
	)
	approval, _ := svc.Create(orgID, "agent", &agentID, nil, "tool_execution", entityID, "Title", "desc", nil, "high", "act", "tgt", nil, 24*time.Hour)
	mock.ExpectQuery(`SELECT status, expires_at FROM approvals WHERE id=.+`).WillReturnRows(
		sqlmock.NewRows([]string{"status","expires_at"}).AddRow("pending", time.Now().Add(24*time.Hour)),
	)
	mock.ExpectQuery(`UPDATE approvals SET status=.+`).WillReturnRows(
		sqlmock.NewRows([]string{"id","organization_id","requester_type","requester_agent_id","requester_user_id","entity_type","entity_id","title","description","payload","risk_level","action","target","context","status","decided_by","decided_at","expires_at","created_at","updated_at"}).
			AddRow(approval.ID, orgID, "agent", agentID, nil, "tool_execution", entityID, "Title", "desc", []byte(`{}`), "high", "act", "tgt", []byte(`{}`), "rejected", uuid.New(), time.Now(), time.Now().Add(24*time.Hour), time.Now(), time.Now()),
	)
	rejected, err := svc.Decide(orgID, approval.ID, uuid.New(), "rejected")
	if err != nil || rejected.Status != "rejected" { t.Fatalf("reject failed: %v", err) }
}
