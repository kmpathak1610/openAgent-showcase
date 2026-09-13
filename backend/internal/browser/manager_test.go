package browser

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"openagent/internal/repository"
)

func newMockManager(t *testing.T) (*Manager, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil { t.Fatalf("sqlmock: %v", err) }
	mock.MatchExpectationsInOrder(false)
	repo := repository.New(db)
	m := NewManager(repo)
	return m, mock, func() { db.Close() }
}

func TestBrowser_ProfileCreation(t *testing.T) {
	m, mock, cleanup := newMockManager(t); defer cleanup()
	org := uuid.New()
	user := uuid.New()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO browser_profiles`)).
		WillReturnRows(sqlmock.NewRows([]string{"id","organization_id","owner_user_id","provider","name","storage_key","status","metadata","created_at","updated_at","last_used_at"}).
			AddRow(uuid.New(), org, user, "generic", "Test", "key", "disconnected", []byte(`{}`), now, now, nil))
	p, err := m.CreateProfile(context.Background(), org, user, "generic", "Test", nil)
	if err != nil { t.Fatalf("create profile: %v", err) }
	if p.Name != "Test" { t.Fatalf("name mismatch") }
	if p.StorageKey != "key" { t.Fatalf("storageKey should be returned in manager but not exposed via API") }
}

func TestBrowser_ProfileIsolation_CrossOrg(t *testing.T) {
	m, mock, cleanup := newMockManager(t); defer cleanup()
	orgA := uuid.New()
	orgB := uuid.New()
	user := uuid.New()
	profileID := uuid.New()
	now := time.Now()
	// GetProfile will query and return orgA, then isolation check should fail for orgB
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, organization_id, owner_user_id, provider, name, storage_key, status, metadata, created_at, updated_at, last_used_at FROM browser_profiles WHERE id=`)).
		WillReturnRows(sqlmock.NewRows([]string{"id","organization_id","owner_user_id","provider","name","storage_key","status","metadata","created_at","updated_at","last_used_at"}).
			AddRow(profileID, orgA, user, "generic", "P", "key", "disconnected", []byte(`{}`), now, now, nil))
	_, err := m.GetProfile(context.Background(), orgB, user, profileID)
	if err == nil || err.Error() != "profile belongs to different organization" {
		t.Fatalf("expected cross-org isolation, got %v", err)
	}
}

func TestBrowser_ProfileIsolation_CrossUser(t *testing.T) {
	m, mock, cleanup := newMockManager(t); defer cleanup()
	org := uuid.New()
	userA := uuid.New()
	userB := uuid.New()
	profileID := uuid.New()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, organization_id, owner_user_id, provider, name, storage_key, status, metadata, created_at, updated_at, last_used_at FROM browser_profiles WHERE id=`)).
		WillReturnRows(sqlmock.NewRows([]string{"id","organization_id","owner_user_id","provider","name","storage_key","status","metadata","created_at","updated_at","last_used_at"}).
			AddRow(profileID, org, userA, "generic", "P", "key", "disconnected", []byte(`{}`), now, now, nil))
	_, err := m.GetProfile(context.Background(), org, userB, profileID)
	if err == nil || err.Error() != "profile belongs to different user" {
		t.Fatalf("expected cross-user isolation, got %v", err)
	}
}

func TestBrowser_SSRFProtection(t *testing.T) {
	cases := []struct{ url string; allowed bool }{
		{"https://example.com", true},
		{"http://example.com/path", true},
		{"file:///etc/passwd", false},
		{"http://127.0.0.1/admin", false},
		{"http://10.0.0.1", false},
		{"http://192.168.1.1", false},
		{"http://169.254.169.254/latest/meta-data/", false},
		{"javascript:alert(1)", false},
		{"https://internal.example.internal", false},
	}
	for _, c := range cases {
		ok, _ := IsAllowedURL(c.url)
		if ok != c.allowed {
			t.Fatalf("IsAllowedURL(%q) = %v, want %v", c.url, ok, c.allowed)
		}
	}
}

func TestBrowser_SessionIsolation(t *testing.T) {
	m, mock, cleanup := newMockManager(t); defer cleanup()
	orgA := uuid.New()
	orgB := uuid.New()
	user := uuid.New()
	sessionID := uuid.New()
	profileID := uuid.New()
	now := time.Now()
	expires := now.Add(time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, browser_profile_id, organization_id, owner_user_id, status, started_at, last_activity_at, expires_at, metadata, created_at, updated_at FROM browser_sessions WHERE id=`)).
		WillReturnRows(sqlmock.NewRows([]string{"id","browser_profile_id","organization_id","owner_user_id","status","started_at","last_activity_at","expires_at","metadata","created_at","updated_at"}).
			AddRow(sessionID, profileID, orgA, user, "connected", now, now, expires, []byte(`{}`), now, now))
	_, err := m.GetSession(context.Background(), orgB, user, sessionID)
	if err == nil { t.Fatalf("expected cross-org session isolation") }
}

func TestBrowser_ResourceLimits(t *testing.T) {
	if MaxConcurrentBrowsers != 3 { t.Fatalf("expected 3 concurrent") }
	if MaxSessionsPerUser != 5 { t.Fatalf("expected 5 per user") }
	if SessionTTL != 30*60*1e9 { t.Fatalf("expected 30m TTL") }
}

func TestBrowser_ToolsSecurity(t *testing.T) {
	// Click requires selector
	m, _, cleanup := newMockManager(t); defer cleanup()
	exec := NewToolExecutor(m)
	_, err := exec.Execute(context.Background(), uuid.New(), uuid.New(), nil, nil, nil, "browser.click", map[string]any{})
	if err == nil { t.Fatalf("expected selector required") }
	// Type should reject secret
	_, err = exec.Execute(context.Background(), uuid.New(), uuid.New(), nil, nil, nil, "browser.type", map[string]any{"selector": "#a", "text": "password=sk-1234"})
	if err != nil { t.Fatalf("type should not error for secret, should return success false") }
	// IsAllowedURL already tested
}
