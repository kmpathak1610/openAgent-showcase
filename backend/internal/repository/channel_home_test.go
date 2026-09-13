package repository

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestEnsureGeneralChannel_CreatesOnce(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	r := New(db)
	orgID := uuid.New()
	ownerID := uuid.New()
	chanID := uuid.New()
	now := time.Now()
	mock.ExpectQuery(`SELECT id FROM channels WHERE organization_id=\$1 AND name=\$2`).
		WithArgs(orgID, "general").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`INSERT INTO channels`).
		WithArgs(orgID, nil, "general", "General", "Home for humans and agents", "", "standard", ownerID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "project_id", "name", "display_name", "description", "topic", "channel_type", "created_by", "created_at", "updated_at"}).
			AddRow(chanID, orgID, nil, "general", "General", "Home for humans and agents", "", "standard", ownerID, now, now))
	mock.ExpectExec(`INSERT INTO channel_members`).
		WithArgs(chanID, ownerID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO messages`).
		WithArgs(orgID, chanID, "Welcome to #general — humans and agents collaborate here. Mention an agent or just ask.").
		WillReturnResult(sqlmock.NewResult(1, 1))
	ch, err := r.EnsureGeneralChannel(orgID, ownerID)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if ch.Name != "general" {
		t.Fatalf("expected general got %q", ch.Name)
	}
	if ch.DisplayName != "General" {
		t.Fatalf("expected DisplayName General got %q", ch.DisplayName)
	}
	if ch.ChannelType != "standard" {
		t.Fatalf("expected standard got %q", ch.ChannelType)
	}
	if ch.ProjectID != nil {
		t.Fatalf("expected nil ProjectID got %v", *ch.ProjectID)
	}
	if ch.CreatedBy == nil || *ch.CreatedBy != ownerID {
		t.Fatalf("expected created_by %v got %v", ownerID, ch.CreatedBy)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestEnsureGeneralChannel_ReturnsExisting(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	r := New(db)
	orgID := uuid.New()
	ownerID := uuid.New()
	chanID := uuid.New()
	now := time.Now()
	// Hit path: SELECT returns id, GetChannel returns general, no INSERT.
	mock.ExpectQuery(`SELECT id FROM channels WHERE organization_id=\$1 AND name=\$2`).
		WithArgs(orgID, "general").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(chanID))
	mock.ExpectQuery(`SELECT id,organization_id,project_id,name`).
		WithArgs(chanID, orgID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "project_id", "name", "display_name", "description", "topic", "channel_type", "created_by", "created_at", "updated_at"}).
			AddRow(chanID, orgID, nil, "general", "General", "Home for humans and agents", "", "standard", ownerID, now, now))
	ch, err := r.EnsureGeneralChannel(orgID, ownerID)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if ch.ID != chanID {
		t.Fatalf("expected %v got %v", chanID, ch.ID)
	}
	if ch.Name != "general" {
		t.Fatalf("expected general got %q", ch.Name)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestEnsureGeneralChannel_RaceReturnsExisting(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	r := New(db)
	orgID := uuid.New()
	ownerID := uuid.New()
	chanID := uuid.New()
	now := time.Now()
	// Miss, then Create fails unique (race), fallback Get returns existing.
	mock.ExpectQuery(`SELECT id FROM channels WHERE organization_id=\$1 AND name=\$2`).
		WithArgs(orgID, "general").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`INSERT INTO channels`).
		WithArgs(orgID, nil, "general", "General", "Home for humans and agents", "", "standard", ownerID).
		WillReturnError(errors.New(`pq: duplicate key value violates unique constraint "channels_org_name_uniq"`))
	mock.ExpectQuery(`SELECT id FROM channels WHERE organization_id=\$1 AND name=\$2`).
		WithArgs(orgID, "general").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(chanID))
	mock.ExpectQuery(`SELECT id,organization_id,project_id,name`).
		WithArgs(chanID, orgID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "project_id", "name", "display_name", "description", "topic", "channel_type", "created_by", "created_at", "updated_at"}).
			AddRow(chanID, orgID, nil, "general", "General", "Home for humans and agents", "", "standard", ownerID, now, now))
	ch, err := r.EnsureGeneralChannel(orgID, ownerID)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if ch.ID != chanID {
		t.Fatalf("expected %v got %v", chanID, ch.ID)
	}
	if ch.Name != "general" {
		t.Fatalf("expected general got %q", ch.Name)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}
