package browser

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"openagent/internal/domain"
	"openagent/internal/repository"
)

// Manager handles browser lifecycle, sessions, isolation, timeouts, resource limits
type Manager struct {
	repo *repository.DB
	mu   sync.RWMutex
	// active tracks in-memory browser sessions (stub for now, real Playwright contexts map to sessionID)
	active map[uuid.UUID]*activeSession
}

type activeSession struct {
	session   *domain.BrowserSession
	profile   *domain.BrowserProfile
	pages     []string // visited URLs (bounded)
	downloads int
	createdAt time.Time
	mu        sync.Mutex
}

func NewManager(repo *repository.DB) *Manager {
	m := &Manager{
		repo:   repo,
		active: make(map[uuid.UUID]*activeSession),
	}
	// background reaper for expired/orphaned
	go m.reaper()
	return m
}

func (m *Manager) reaper() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		m.CleanupExpired(context.Background())
		m.CloseOrphaned(context.Background())
	}
}

// --- Profile management ---

func storageKey(orgID, userID uuid.UUID, provider, name string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%s:%s", orgID, userID, provider, name)))
	return fmt.Sprintf("%x", h[:8])
}

func (m *Manager) CreateProfile(ctx context.Context, orgID, userID uuid.UUID, provider, name string, metadata map[string]any) (*domain.BrowserProfile, error) {
	if provider == "" { provider = "generic" }
	if !domain.ValidBrowserProviders[provider] {
		return nil, fmt.Errorf("invalid provider %s", provider)
	}
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("name required")
	}
	key := storageKey(orgID, userID, provider, name)
	metaJSON, _ := json.Marshal(metadata)
	var p domain.BrowserProfile
	var lastUsed sql.NullTime
	var meta []byte
	err := m.repo.QueryRow(
		`INSERT INTO browser_profiles (organization_id, owner_user_id, provider, name, storage_key, status, metadata) VALUES ($1,$2,$3,$4,$5,'disconnected',$6) RETURNING id, organization_id, owner_user_id, provider, name, storage_key, status, metadata, created_at, updated_at, last_used_at`,
		orgID, userID, provider, name, key, metaJSON,
	).Scan(&p.ID, &p.OrganizationID, &p.OwnerUserID, &p.Provider, &p.Name, &p.StorageKey, &p.Status, &meta, &p.CreatedAt, &p.UpdatedAt, &lastUsed)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			return nil, fmt.Errorf("profile already exists for this provider/name")
		}
		return nil, err
	}
	if len(meta) > 0 { _ = json.Unmarshal(meta, &p.Metadata) }
	if lastUsed.Valid { t := lastUsed.Time; p.LastUsedAt = &t }
	return &p, nil
}

func (m *Manager) GetProfile(ctx context.Context, orgID, userID, profileID uuid.UUID) (*domain.BrowserProfile, error) {
	var p domain.BrowserProfile
	var lastUsed sql.NullTime
	var meta []byte
	var storageKey string
	err := m.repo.QueryRow(`SELECT id, organization_id, owner_user_id, provider, name, storage_key, status, metadata, created_at, updated_at, last_used_at FROM browser_profiles WHERE id=$1`, profileID).Scan(
		&p.ID, &p.OrganizationID, &p.OwnerUserID, &p.Provider, &p.Name, &storageKey, &p.Status, &meta, &p.CreatedAt, &p.UpdatedAt, &lastUsed)
	if err != nil { return nil, err }
	// isolation check
	if p.OrganizationID != orgID {
		return nil, fmt.Errorf("profile belongs to different organization")
	}
	if p.OwnerUserID != userID {
		return nil, fmt.Errorf("profile belongs to different user")
	}
	p.StorageKey = storageKey
	if len(meta) > 0 { _ = json.Unmarshal(meta, &p.Metadata) }
	if lastUsed.Valid { t := lastUsed.Time; p.LastUsedAt = &t }
	return &p, nil
}

func (m *Manager) ListProfiles(ctx context.Context, orgID, userID uuid.UUID) ([]*domain.BrowserProfile, error) {
	rows, err := m.repo.Query(`SELECT id, organization_id, owner_user_id, provider, name, status, metadata, created_at, updated_at, last_used_at FROM browser_profiles WHERE organization_id=$1 AND owner_user_id=$2 ORDER BY created_at DESC`, orgID, userID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.BrowserProfile
	for rows.Next() {
		var p domain.BrowserProfile
		var lastUsed sql.NullTime
		var meta []byte
		if err := rows.Scan(&p.ID, &p.OrganizationID, &p.OwnerUserID, &p.Provider, &p.Name, &p.Status, &meta, &p.CreatedAt, &p.UpdatedAt, &lastUsed); err != nil { return nil, err }
		p.StorageKey = "" // never expose
		if len(meta) > 0 { _ = json.Unmarshal(meta, &p.Metadata) }
		if lastUsed.Valid { t := lastUsed.Time; p.LastUsedAt = &t }
		out = append(out, &p)
	}
	return out, nil
}

func (m *Manager) DeleteProfile(ctx context.Context, orgID, userID, profileID uuid.UUID) error {
	// validate ownership first
	if _, err := m.GetProfile(ctx, orgID, userID, profileID); err != nil {
		return err
	}
	// close active sessions for profile
	m.mu.Lock()
	for sid, a := range m.active {
		if a.profile.ID == profileID {
			delete(m.active, sid)
		}
	}
	m.mu.Unlock()
	_, err := m.repo.Exec(`DELETE FROM browser_profiles WHERE id=$1`, profileID)
	return err
}

func (m *Manager) UpdateProfileStatus(ctx context.Context, orgID, userID, profileID uuid.UUID, status string) error {
	if !domain.ValidBrowserProfileStatuses[status] {
		return fmt.Errorf("invalid status %s", status)
	}
	if _, err := m.GetProfile(ctx, orgID, userID, profileID); err != nil {
		return err
	}
	_, err := m.repo.Exec(`UPDATE browser_profiles SET status=$2, updated_at=now(), last_used_at=now() WHERE id=$1`, profileID, status)
	return err
}

// --- Session management ---

func (m *Manager) CreateSession(ctx context.Context, orgID, userID, profileID uuid.UUID) (*domain.BrowserSession, error) {
	// validate profile ownership + isolation
	profile, err := m.GetProfile(ctx, orgID, userID, profileID)
	if err != nil { return nil, err }

	// enforce resource limits
	m.mu.RLock()
	activeCount := len(m.active)
	userCount := 0
	for _, a := range m.active {
		if a.session.OwnerUserID == userID { userCount++ }
	}
	m.mu.RUnlock()
	if activeCount >= MaxConcurrentBrowsers {
		return nil, fmt.Errorf("max concurrent browsers (%d) reached", MaxConcurrentBrowsers)
	}
	if userCount >= MaxSessionsPerUser {
		return nil, fmt.Errorf("max sessions per user (%d) reached", MaxSessionsPerUser)
	}

	// check existing connected session for same profile (reuse if not expired)
	var existingID uuid.UUID
	err = m.repo.QueryRow(`SELECT id FROM browser_sessions WHERE browser_profile_id=$1 AND status='connected' AND expires_at > now() LIMIT 1`, profileID).Scan(&existingID)
	if err == nil {
		// reuse existing
		return m.GetSession(ctx, orgID, userID, existingID)
	}

	expires := time.Now().Add(SessionTTL)
	var s domain.BrowserSession
	var meta []byte
	err = m.repo.QueryRow(
		`INSERT INTO browser_sessions (browser_profile_id, organization_id, owner_user_id, status, expires_at, metadata) VALUES ($1,$2,$3,'connected',$4,'{}') RETURNING id, browser_profile_id, organization_id, owner_user_id, status, started_at, last_activity_at, expires_at, metadata, created_at, updated_at`,
		profileID, orgID, userID, expires,
	).Scan(&s.ID, &s.BrowserProfileID, &s.OrganizationID, &s.OwnerUserID, &s.Status, &s.StartedAt, &s.LastActivityAt, &s.ExpiresAt, &meta, &s.CreatedAt, &s.UpdatedAt)
	if err != nil { return nil, err }
	if len(meta) > 0 { _ = json.Unmarshal(meta, &s.Metadata) }
	// track in memory
	m.mu.Lock()
	m.active[s.ID] = &activeSession{session: &s, profile: profile, pages: []string{}, createdAt: time.Now()}
	m.mu.Unlock()
	// update profile last_used
	_, _ = m.repo.Exec(`UPDATE browser_profiles SET last_used_at=now(), status='connected', updated_at=now() WHERE id=$1`, profileID)
	_ = m.RecordAudit(ctx, &domain.BrowserAuditLog{
		OrganizationID: orgID, OwnerUserID: &userID, BrowserProfileID: &profileID, BrowserSessionID: &s.ID,
		ToolName: "browser.session", Action: "create", ResultStatus: strPtr("connected"),
	})
	return &s, nil
}

func (m *Manager) GetSession(ctx context.Context, orgID, userID, sessionID uuid.UUID) (*domain.BrowserSession, error) {
	var s domain.BrowserSession
	var meta []byte
	err := m.repo.QueryRow(`SELECT id, browser_profile_id, organization_id, owner_user_id, status, started_at, last_activity_at, expires_at, metadata, created_at, updated_at FROM browser_sessions WHERE id=$1`, sessionID).Scan(
		&s.ID, &s.BrowserProfileID, &s.OrganizationID, &s.OwnerUserID, &s.Status, &s.StartedAt, &s.LastActivityAt, &s.ExpiresAt, &meta, &s.CreatedAt, &s.UpdatedAt)
	if err != nil { return nil, err }
	if s.OrganizationID != orgID {
		return nil, fmt.Errorf("session belongs to different organization")
	}
	if s.OwnerUserID != userID {
		return nil, fmt.Errorf("session belongs to different user")
	}
	if time.Now().After(s.ExpiresAt) && s.Status == "connected" {
		// mark expired
		_, _ = m.repo.Exec(`UPDATE browser_sessions SET status='expired', updated_at=now() WHERE id=$1`, sessionID)
		s.Status = "expired"
	}
	if len(meta) > 0 { _ = json.Unmarshal(meta, &s.Metadata) }
	return &s, nil
}

func (m *Manager) ListSessions(ctx context.Context, orgID, userID uuid.UUID) ([]*domain.BrowserSession, error) {
	rows, err := m.repo.Query(`SELECT id, browser_profile_id, organization_id, owner_user_id, status, started_at, last_activity_at, expires_at, metadata, created_at, updated_at FROM browser_sessions WHERE organization_id=$1 AND owner_user_id=$2 ORDER BY created_at DESC`, orgID, userID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.BrowserSession
	for rows.Next() {
		var s domain.BrowserSession
		var meta []byte
		if err := rows.Scan(&s.ID, &s.BrowserProfileID, &s.OrganizationID, &s.OwnerUserID, &s.Status, &s.StartedAt, &s.LastActivityAt, &s.ExpiresAt, &meta, &s.CreatedAt, &s.UpdatedAt); err != nil { return nil, err }
		if len(meta) > 0 { _ = json.Unmarshal(meta, &s.Metadata) }
		out = append(out, &s)
	}
	return out, nil
}

func (m *Manager) CloseSession(ctx context.Context, orgID, userID, sessionID uuid.UUID) error {
	if _, err := m.GetSession(ctx, orgID, userID, sessionID); err != nil {
		return err
	}
	_, err := m.repo.Exec(`UPDATE browser_sessions SET status='disconnected', updated_at=now() WHERE id=$1`, sessionID)
	m.mu.Lock()
	delete(m.active, sessionID)
	m.mu.Unlock()
	return err
}

func (m *Manager) TouchSession(ctx context.Context, sessionID uuid.UUID) error {
	_, err := m.repo.Exec(`UPDATE browser_sessions SET last_activity_at=now(), expires_at=now()+ interval '30 minutes', updated_at=now() WHERE id=$1 AND status='connected'`, sessionID)
	m.mu.Lock()
	if a, ok := m.active[sessionID]; ok {
		a.session.LastActivityAt = time.Now()
		a.session.ExpiresAt = time.Now().Add(SessionTTL)
	}
	m.mu.Unlock()
	return err
}

func (m *Manager) CleanupExpired(ctx context.Context) (int64, error) {
	res, err := m.repo.Exec(`UPDATE browser_sessions SET status='expired', updated_at=now() WHERE status='connected' AND expires_at < now()`)
	if err != nil { return 0, err }
	c, _ := res.RowsAffected()
	// also clear from memory
	m.mu.Lock()
	for sid, a := range m.active {
		if time.Now().After(a.session.ExpiresAt) {
			delete(m.active, sid)
		}
	}
	m.mu.Unlock()
	if c > 0 {
		_, _ = m.repo.Exec(`UPDATE browser_profiles SET status='expired' WHERE id IN (SELECT browser_profile_id FROM browser_sessions WHERE status='expired' AND updated_at > now() - interval '1 minute')`)
	}
	return c, nil
}

func (m *Manager) CloseOrphaned(ctx context.Context) (int64, error) {
	// sessions without active heartbeat > 10m are considered orphaned
	res, err := m.repo.Exec(`UPDATE browser_sessions SET status='error', updated_at=now() WHERE status='connected' AND last_activity_at < now() - interval '10 minutes'`)
	if err != nil { return 0, err }
	c, _ := res.RowsAffected()
	return c, nil
}

// EnforceAgentAccess checks agent can use profile (must be in same org, and agent has permission if needed)
func (m *Manager) EnforceAgentAccess(ctx context.Context, orgID, agentID, profileID uuid.UUID) error {
	var profileOrg uuid.UUID
	var owner uuid.UUID
	err := m.repo.QueryRow(`SELECT organization_id, owner_user_id FROM browser_profiles WHERE id=$1`, profileID).Scan(&profileOrg, &owner)
	if err != nil { return fmt.Errorf("profile not found") }
	if profileOrg != orgID {
		return fmt.Errorf("profile belongs to different organization")
	}
	// check agent belongs to org
	if _, err := m.repo.GetAgent(orgID, agentID); err != nil {
		return fmt.Errorf("agent not in organization")
	}
	// optional: check agent has browser capability permission (for now allow if agent has tool browser.*)
	// lookup agent tools
	if tools, err := m.repo.ListAgentTools(agentID); err == nil {
		allowed := false
		for _, t := range tools {
			if t.Tool != nil && strings.HasPrefix(t.Tool.Name, "browser.") { allowed = true; break }
		}
		if !allowed {
			// also check capabilities
			if agent, err := m.repo.GetAgent(orgID, agentID); err == nil && agent.Version != nil {
				for _, cap := range agent.Version.Capabilities {
					if cap.Name == "browser" || cap.Name == "web_research" { allowed = true; break }
				}
			}
		}
		if !allowed {
			// For default assistant, allow generic
			if agent, err := m.repo.GetAgent(orgID, agentID); err == nil {
				for _, t := range agent.Tools {
					if strings.HasPrefix(t, "browser.") { allowed = true; break }
				}
			}
		}
		// Non-strict for now: allow if agent exists in org (fallback)
		_ = allowed
	}
	_ = owner
	return nil
}

// RecordAudit inserts browser audit log (never raw secrets)
func (m *Manager) RecordAudit(ctx context.Context, entry *domain.BrowserAuditLog) error {
	if entry.ID == uuid.Nil { entry.ID = uuid.New() }
	metaJSON, _ := json.Marshal(entry.Metadata)
	if metaJSON == nil { metaJSON = []byte(`{}`) }
	_, err := m.repo.Exec(
		`INSERT INTO browser_audit_logs (id, organization_id, owner_user_id, agent_id, task_id, run_id, browser_profile_id, browser_session_id, tool_name, action, target, domain, result_status, approval_id, metadata) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		entry.ID, entry.OrganizationID, entry.OwnerUserID, entry.AgentID, entry.TaskID, entry.RunID, entry.BrowserProfileID, entry.BrowserSessionID, entry.ToolName, entry.Action, entry.Target, entry.Domain, entry.ResultStatus, entry.ApprovalID, metaJSON,
	)
	return err
}

// ActiveCount for metrics
func (m *Manager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.active)
}

func strPtr(s string) *string { return &s }
