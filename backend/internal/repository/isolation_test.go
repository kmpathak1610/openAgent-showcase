package repository

import (
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func mustParseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

func newMockDB(t *testing.T) (*DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil { t.Fatalf("sqlmock: %v", err) }
	return &DB{db}, mock, func() { db.Close() }
}

func TestTenantIsolation_ProjectOrgFilter(t *testing.T) {
	db, mock, close := newMockDB(t)
	defer close()
	orgID := uuid.New()
	otherOrg := uuid.New()
	pid := uuid.New()
	// GetProject should query with organization_id filter; simulate not found when org mismatch
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id,organization_id,name,slug,description,objective,icon,status,settings,created_by,created_at,updated_at FROM projects WHERE id=$1 AND organization_id=$2`)).
		WithArgs(pid, otherOrg).
		WillReturnError(sql.ErrNoRows)

	_, err := db.GetProject(otherOrg, pid)
	if err != sql.ErrNoRows {
		t.Fatalf("expected ErrNoRows for mismatched org, got %v", err)
	}
	// Correct org should succeed
	now := "2024-01-01T00:00:00Z"
	// Use time parseable string but we need time.Time in mock - sqlmock will scan string into time.Time via parse? Use time.Time directly
	// To avoid scan error, return time.Time values
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id,organization_id,name,slug,description,objective,icon,status,settings,created_by,created_at,updated_at FROM projects WHERE id=$1 AND organization_id=$2`)).
		WithArgs(pid, orgID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "name", "slug", "description", "objective", "icon", "status", "settings", "created_by", "created_at", "updated_at"}).
			AddRow(pid, orgID, "Test", "test", "", "", "📁", "active", []byte(`{}`), nil, mustParseTime(now), mustParseTime(now)))
	p, err := db.GetProject(orgID, pid)
	if err != nil || p.ID != pid {
		t.Fatalf("expected project, got %v err %v", p, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil { t.Fatalf("unmet: %v", err) }
}

func TestTenantIsolation_ChannelOrgFilter(t *testing.T) {
	db, mock, close := newMockDB(t)
	defer close()
	orgA := uuid.New()
	cid := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id,organization_id,project_id,name,display_name,description,topic,channel_type,created_by,created_at,updated_at FROM channels WHERE id=$1 AND organization_id=$2`)).
		WithArgs(cid, orgA).
		WillReturnError(sql.ErrNoRows)
	_, err := db.GetChannel(orgA, cid)
	if err != sql.ErrNoRows { t.Fatalf("expected isolation failure") }
	if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
}

func TestTenantIsolation_MessageOrgFilter(t *testing.T) {
	db, mock, close := newMockDB(t)
	defer close()
	orgA := uuid.New()
	mid := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id,organization_id,channel_id,thread_id,sender_type,sender_user_id,sender_agent_id,body,body_format,metadata,edited_at,deleted_at,created_at,updated_at FROM messages WHERE id=$1 AND organization_id=$2`)).
		WithArgs(mid, orgA).WillReturnError(sql.ErrNoRows)
	_, err := db.GetMessage(orgA, mid)
	if err != sql.ErrNoRows { t.Fatalf("expected isolation") }
	if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
}

func TestTenantIsolation_IsOrgMember(t *testing.T) {
	db, mock, close := newMockDB(t)
	defer close()
	orgID := uuid.New()
	userID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT role FROM organization_members WHERE organization_id=$1 AND user_id=$2`)).
		WithArgs(orgID, userID).WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("member"))
	role, ok := db.IsOrgMember(orgID, userID)
	if !ok || role != "member" { t.Fatalf("expected member") }
	otherUser := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT role FROM organization_members WHERE organization_id=$1 AND user_id=$2`)).
		WithArgs(orgID, otherUser).WillReturnError(sql.ErrNoRows)
	_, ok = db.IsOrgMember(orgID, otherUser)
	if ok { t.Fatalf("should not be member") }
	if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
}

func TestProjectAccess_IsProjectMember(t *testing.T) {
	db, mock, close := newMockDB(t)
	defer close()
	pid := uuid.New()
	uid := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM project_members WHERE project_id=$1 AND user_id=$2`)).
		WithArgs(pid, uid).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	if !db.IsProjectMember(pid, uid) { t.Fatalf("expected member") }
	if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
}

func TestChannelAccess_IsChannelMember(t *testing.T) {
	db, mock, close := newMockDB(t)
	defer close()
	cid := uuid.New()
	uid := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM channel_members WHERE channel_id=$1 AND member_type='user' AND user_id=$2`)).
		WithArgs(cid, uid).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	if !db.IsChannelMember(cid, uid) { t.Fatalf("expected channel member") }
	if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
}
