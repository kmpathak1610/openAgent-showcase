package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"openagent/internal/domain"
)

// DB wraps sql.DB
type DB struct {
	*sql.DB
}

func New(db *sql.DB) *DB { return &DB{db} }

// Helpers

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = uuid.NewString()[:8]
	}
	return out
}

func scanOrg(row interface{ Scan(dest ...any) error }) (*domain.Organization, error) {
	var o domain.Organization
	var avatar sql.NullString
	var settings []byte
	var createdAt, updatedAt time.Time
	if err := row.Scan(&o.ID, &o.Name, &o.Slug, &avatar, &settings, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	if avatar.Valid { o.AvatarURL = &avatar.String }
	if len(settings) > 0 { _ = json.Unmarshal(settings, &o.Settings) } else { o.Settings = map[string]any{} }
	o.CreatedAt = createdAt; o.UpdatedAt = updatedAt
	return &o, nil
}

// ——— Users

func (db *DB) CreateUser(email, displayName, passwordHash string) (*domain.User, error) {
	var u domain.User
	var avatar sql.NullString
	var lastSeen sql.NullTime
	err := db.QueryRow(
		`INSERT INTO users (email, display_name, password_hash) VALUES ($1,$2,$3) RETURNING id,email,display_name,avatar_url,password_hash,status,last_seen_at,created_at,updated_at`,
		strings.ToLower(strings.TrimSpace(email)), displayName, passwordHash,
	).Scan(&u.ID, &u.Email, &u.DisplayName, &avatar, &u.PasswordHash, &u.Status, &lastSeen, &u.CreatedAt, &u.UpdatedAt)
	if err != nil { return nil, err }
	if avatar.Valid { u.AvatarURL = &avatar.String }
	if lastSeen.Valid { u.LastSeenAt = &lastSeen.Time }
	return &u, nil
}

func (db *DB) FindUserByEmail(email string) (*domain.User, error) {
	var u domain.User
	var avatar sql.NullString
	var lastSeen sql.NullTime
	err := db.QueryRow(`SELECT id,email,display_name,avatar_url,password_hash,status,last_seen_at,created_at,updated_at FROM users WHERE email=$1`, strings.ToLower(strings.TrimSpace(email))).Scan(
		&u.ID, &u.Email, &u.DisplayName, &avatar, &u.PasswordHash, &u.Status, &lastSeen, &u.CreatedAt, &u.UpdatedAt)
	if err != nil { return nil, err }
	if avatar.Valid { u.AvatarURL = &avatar.String }
	if lastSeen.Valid { u.LastSeenAt = &lastSeen.Time }
	return &u, nil
}

func (db *DB) FindUserByID(id uuid.UUID) (*domain.User, error) {
	var u domain.User
	var avatar sql.NullString
	var lastSeen sql.NullTime
	err := db.QueryRow(`SELECT id,email,display_name,avatar_url,password_hash,status,last_seen_at,created_at,updated_at FROM users WHERE id=$1`, id).Scan(
		&u.ID, &u.Email, &u.DisplayName, &avatar, &u.PasswordHash, &u.Status, &lastSeen, &u.CreatedAt, &u.UpdatedAt)
	if err != nil { return nil, err }
	if avatar.Valid { u.AvatarURL = &avatar.String }
	if lastSeen.Valid { u.LastSeenAt = &lastSeen.Time }
	return &u, nil
}

// ——— Organizations

func (db *DB) CreateOrganization(name, slug string, ownerID uuid.UUID) (*domain.Organization, error) {
	tx, err := db.Begin()
	if err != nil { return nil, err }
	defer tx.Rollback()
	if slug == "" { slug = slugify(name) }
	// ensure unique slug: append suffix if conflict
	baseSlug := slug
	for i := 0; i < 5; i++ {
		var o domain.Organization
		var avatar sql.NullString
		var settings []byte
		err = tx.QueryRow(`INSERT INTO organizations (name, slug) VALUES ($1,$2) RETURNING id,name,slug,avatar_url,settings,created_at,updated_at`, name, slug).Scan(&o.ID, &o.Name, &o.Slug, &avatar, &settings, &o.CreatedAt, &o.UpdatedAt)
		if err == nil {
			if avatar.Valid { o.AvatarURL = &avatar.String }
			if len(settings) > 0 { _ = json.Unmarshal(settings, &o.Settings) }
			_, err = tx.Exec(`INSERT INTO organization_members (organization_id, user_id, role) VALUES ($1,$2,'owner')`, o.ID, ownerID)
			if err != nil { return nil, err }
			if err := tx.Commit(); err != nil { return nil, err }
			return &o, nil
		}
		if !strings.Contains(err.Error(), "duplicate") && !strings.Contains(err.Error(), "unique") {
			return nil, err
		}
		slug = fmt.Sprintf("%s-%s", baseSlug, uuid.NewString()[:4])
	}
	return nil, err
}

func (db *DB) GetOrganization(orgID uuid.UUID) (*domain.Organization, error) {
	row := db.QueryRow(`SELECT id,name,slug,avatar_url,settings,created_at,updated_at FROM organizations WHERE id=$1`, orgID)
	return scanOrg(row)
}

func (db *DB) ListOrganizationsForUser(userID uuid.UUID) ([]*domain.Organization, error) {
	rows, err := db.Query(`SELECT o.id,o.name,o.slug,o.avatar_url,o.settings,o.created_at,o.updated_at FROM organizations o JOIN organization_members om ON om.organization_id=o.id WHERE om.user_id=$1 ORDER BY o.created_at`, userID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.Organization
	for rows.Next() {
		var o domain.Organization
		var avatar sql.NullString
		var settings []byte
		var ca, ua time.Time
		if err := rows.Scan(&o.ID, &o.Name, &o.Slug, &avatar, &settings, &ca, &ua); err != nil { return nil, err }
		if avatar.Valid { o.AvatarURL = &avatar.String }
		if len(settings) > 0 { _ = json.Unmarshal(settings, &o.Settings) }
		o.CreatedAt = ca; o.UpdatedAt = ua
		out = append(out, &o)
	}
	return out, nil
}

func (db *DB) UpdateOrganization(orgID uuid.UUID, name string) (*domain.Organization, error) {
	row := db.QueryRow(`UPDATE organizations SET name=$2, updated_at=now() WHERE id=$1 RETURNING id,name,slug,avatar_url,settings,created_at,updated_at`, orgID, name)
	return scanOrg(row)
}

func (db *DB) IsOrgMember(orgID, userID uuid.UUID) (string, bool) {
	var role string
	err := db.QueryRow(`SELECT role FROM organization_members WHERE organization_id=$1 AND user_id=$2`, orgID, userID).Scan(&role)
	if err != nil { return "", false }
	return role, true
}

func (db *DB) ListOrgMembers(orgID uuid.UUID) ([]*domain.OrganizationMember, error) {
	rows, err := db.Query(`
		SELECT om.id, om.organization_id, om.user_id, om.role, om.joined_at, u.id,u.email,u.display_name,u.avatar_url,u.status,u.created_at,u.updated_at
		FROM organization_members om JOIN users u ON u.id=om.user_id WHERE om.organization_id=$1 ORDER BY om.joined_at`, orgID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.OrganizationMember
	for rows.Next() {
		var m domain.OrganizationMember
		var u domain.User
		var uAvatar sql.NullString
		var uPass string
		var uLastSeen sql.NullTime
		var joinedAt time.Time
		if err := rows.Scan(&m.ID, &m.OrganizationID, &m.UserID, &m.Role, &joinedAt, &u.ID, &u.Email, &u.DisplayName, &uAvatar, &u.Status, &u.CreatedAt, &u.UpdatedAt); err != nil { return nil, err }
		_ = uPass; _ = uLastSeen
		if uAvatar.Valid { u.AvatarURL = &uAvatar.String }
		m.JoinedAt = joinedAt
		uc := u; m.User = &uc
		out = append(out, &m)
	}
	return out, nil
}

func (db *DB) AddOrgMember(orgID, userID uuid.UUID, role string) error {
	if role == "" { role = "member" }
	_, err := db.Exec(`INSERT INTO organization_members (organization_id,user_id,role) VALUES ($1,$2,$3) ON CONFLICT (organization_id,user_id) DO UPDATE SET role=EXCLUDED.role`, orgID, userID, role)
	return err
}

func (db *DB) RemoveOrgMember(orgID, userID uuid.UUID) error {
	_, err := db.Exec(`DELETE FROM organization_members WHERE organization_id=$1 AND user_id=$2`, orgID, userID)
	return err
}

// ——— Projects

func (db *DB) CreateProject(orgID uuid.UUID, name, description, objective, icon, slug string, createdBy uuid.UUID) (*domain.Project, error) {
	if slug == "" { slug = slugify(name) }
	base := slug
	var p domain.Project
	var settings []byte
	for i := 0; i < 5; i++ {
		err := db.QueryRow(`INSERT INTO projects (organization_id,name,slug,description,objective,icon,created_by) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id,organization_id,name,slug,description,objective,icon,status,settings,created_by,created_at,updated_at`,
			orgID, name, slug, description, objective, icon, createdBy).Scan(&p.ID, &p.OrganizationID, &p.Name, &p.Slug, &p.Description, &p.Objective, &p.Icon, &p.Status, &settings, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
		if err == nil {
			if len(settings) > 0 { _ = json.Unmarshal(settings, &p.Settings) } else { p.Settings = map[string]any{} }
			// auto-add creator as project member lead
			_, _ = db.Exec(`INSERT INTO project_members (project_id,user_id,role) VALUES ($1,$2,'lead') ON CONFLICT DO NOTHING`, p.ID, createdBy)
			return &p, nil
		}
		if !strings.Contains(err.Error(), "duplicate") && !strings.Contains(err.Error(), "unique") {
			return nil, err
		}
		slug = fmt.Sprintf("%s-%s", base, uuid.NewString()[:4])
	}
	return nil, fmt.Errorf("failed to create project: slug conflict")
}

func (db *DB) ListProjects(orgID uuid.UUID) ([]*domain.Project, error) {
	rows, err := db.Query(`SELECT id,organization_id,name,slug,description,objective,icon,status,settings,created_by,created_at,updated_at FROM projects WHERE organization_id=$1 AND status='active' ORDER BY created_at DESC`, orgID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.Project
	for rows.Next() {
		var p domain.Project
		var settings []byte
		if err := rows.Scan(&p.ID, &p.OrganizationID, &p.Name, &p.Slug, &p.Description, &p.Objective, &p.Icon, &p.Status, &settings, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil { return nil, err }
		if len(settings) > 0 { _ = json.Unmarshal(settings, &p.Settings) }
		out = append(out, &p)
	}
	return out, nil
}

func (db *DB) GetProject(orgID, projectID uuid.UUID) (*domain.Project, error) {
	var p domain.Project
	var settings []byte
	err := db.QueryRow(`SELECT id,organization_id,name,slug,description,objective,icon,status,settings,created_by,created_at,updated_at FROM projects WHERE id=$1 AND organization_id=$2`, projectID, orgID).Scan(
		&p.ID, &p.OrganizationID, &p.Name, &p.Slug, &p.Description, &p.Objective, &p.Icon, &p.Status, &settings, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if err != nil { return nil, err }
	if len(settings) > 0 { _ = json.Unmarshal(settings, &p.Settings) }
	return &p, nil
}

func (db *DB) GetProjectBySlug(orgID uuid.UUID, slug string) (*domain.Project, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	var p domain.Project
	var settings []byte
	err := db.QueryRow(`SELECT id,organization_id,name,slug,description,objective,icon,status,settings,created_by,created_at,updated_at FROM projects WHERE organization_id=$1 AND slug=$2 LIMIT 1`, orgID, slug).Scan(
		&p.ID, &p.OrganizationID, &p.Name, &p.Slug, &p.Description, &p.Objective, &p.Icon, &p.Status, &settings, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if err != nil { return nil, err }
	if len(settings) > 0 { _ = json.Unmarshal(settings, &p.Settings) } else { p.Settings = map[string]any{} }
	return &p, nil
}

// EnsureHomeProject returns the idempotent "home" project for general-channel tasks.
// It avoids CreateProject's auto-rename (home-xxxx dup) by using
// INSERT ... ON CONFLICT (organization_id, slug) DO NOTHING then re-Get.
func (db *DB) EnsureHomeProject(orgID, creator uuid.UUID) (uuid.UUID, error) {
	slug := strings.ToLower(strings.TrimSpace("home"))
	if p, err := db.GetProjectBySlug(orgID, slug); err == nil {
		return p.ID, nil
	} else if err != sql.ErrNoRows {
		return uuid.Nil, err
	}
	_, _ = db.Exec(`INSERT INTO projects (organization_id,name,slug,description,objective,icon,created_by) VALUES ($1,'Home',$2,'Home for general channel tasks','','🏠',$3) ON CONFLICT (organization_id, slug) DO NOTHING`, orgID, slug, creator)
	p, err := db.GetProjectBySlug(orgID, slug)
	if err != nil {
		return uuid.Nil, err
	}
	_, _ = db.Exec(`INSERT INTO project_members (project_id,user_id,role) VALUES ($1,$2,'lead') ON CONFLICT DO NOTHING`, p.ID, creator)
	return p.ID, nil
}

func (db *DB) UpdateProject(orgID, projectID uuid.UUID, name, description, objective *string) (*domain.Project, error) {
	p, err := db.GetProject(orgID, projectID)
	if err != nil { return nil, err }
	if name != nil { p.Name = *name }
	if description != nil { p.Description = *description }
	if objective != nil { p.Objective = *objective }
	var settings []byte
	err = db.QueryRow(`UPDATE projects SET name=$3,description=$4,objective=$5,updated_at=now() WHERE id=$1 AND organization_id=$2 RETURNING id,organization_id,name,slug,description,objective,icon,status,settings,created_by,created_at,updated_at`,
		projectID, orgID, p.Name, p.Description, p.Objective).Scan(&p.ID, &p.OrganizationID, &p.Name, &p.Slug, &p.Description, &p.Objective, &p.Icon, &p.Status, &settings, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if err != nil { return nil, err }
	if len(settings) > 0 { _ = json.Unmarshal(settings, &p.Settings) }
	return p, nil
}

func (db *DB) ArchiveProject(orgID, projectID uuid.UUID) error {
	_, err := db.Exec(`UPDATE projects SET status='archived', updated_at=now() WHERE id=$1 AND organization_id=$2`, projectID, orgID)
	return err
}

func (db *DB) IsProjectMember(projectID, userID uuid.UUID) bool {
	var id uuid.UUID
	err := db.QueryRow(`SELECT id FROM project_members WHERE project_id=$1 AND user_id=$2`, projectID, userID).Scan(&id)
	return err == nil
}

func (db *DB) ListProjectMembers(projectID uuid.UUID) ([]*domain.ProjectMember, error) {
	rows, err := db.Query(`SELECT pm.id,pm.project_id,pm.user_id,pm.role,pm.added_at, u.id,u.email,u.display_name,u.avatar_url,u.status,u.created_at,u.updated_at FROM project_members pm JOIN users u ON u.id=pm.user_id WHERE pm.project_id=$1 ORDER BY pm.added_at`, projectID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.ProjectMember
	for rows.Next() {
		var m domain.ProjectMember
		var u domain.User
		var avatar sql.NullString
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.UserID, &m.Role, &m.JoinedAt, &u.ID, &u.Email, &u.DisplayName, &avatar, &u.Status, &u.CreatedAt, &u.UpdatedAt); err != nil { return nil, err }
		if avatar.Valid { u.AvatarURL = &avatar.String }
		uc := u; m.User = &uc
		out = append(out, &m)
	}
	return out, nil
}

func (db *DB) AddProjectMember(projectID, userID uuid.UUID, role string) error {
	if role == "" { role = "member" }
	_, err := db.Exec(`INSERT INTO project_members (project_id,user_id,role) VALUES ($1,$2,$3) ON CONFLICT (project_id,user_id) DO UPDATE SET role=EXCLUDED.role`, projectID, userID, role)
	return err
}

func (db *DB) RemoveProjectMember(projectID, userID uuid.UUID) error {
	_, err := db.Exec(`DELETE FROM project_members WHERE project_id=$1 AND user_id=$2`, projectID, userID)
	return err
}

func (db *DB) ListProjectAgents(projectID uuid.UUID) ([]*domain.Agent, error) {
	rows, err := db.Query(`SELECT a.id, a.organization_id, a.name, a.slug, a.description, a.avatar_url, a.avatar, a.purpose, a.intent, a.system_prompt, a.model, a.provider, a.capabilities, a.tools, a.status, a.autonomy_level, a.current_version, a.owner_id, a.created_by, a.created_at, a.updated_at FROM agents a JOIN project_agents pa ON pa.agent_id=a.id WHERE pa.project_id=$1 ORDER BY a.name`, projectID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.Agent
	for rows.Next() {
		var a domain.Agent
		var avatarUrl, avatar sql.NullString
		var purpose, intent, systemPrompt, model, provider, status, autonomy sql.NullString
		var capJSON, toolsJSON []byte
		var ownerID, createdBy sql.NullString
		var currentVersion sql.NullInt32
		if err := rows.Scan(&a.ID, &a.OrganizationID, &a.Name, &a.Slug, &a.Description, &avatarUrl, &avatar, &purpose, &intent, &systemPrompt, &model, &provider, &capJSON, &toolsJSON, &status, &autonomy, &currentVersion, &ownerID, &createdBy, &a.CreatedAt, &a.UpdatedAt); err != nil { return nil, err }
		if avatarUrl.Valid { a.AvatarURL = &avatarUrl.String }
		if avatar.Valid { a.Avatar = &avatar.String }
		if purpose.Valid { a.Purpose = purpose.String }
		if intent.Valid { a.Intent = intent.String }
		if systemPrompt.Valid { a.SystemPrompt = systemPrompt.String }
		if model.Valid { a.Model = model.String }
		if provider.Valid { a.Provider = provider.String }
		if status.Valid { a.Status = status.String }
		if autonomy.Valid { a.AutonomyLevel = autonomy.String }
		if currentVersion.Valid { a.CurrentVersion = int(currentVersion.Int32) }
		if ownerID.Valid { uid, _ := uuid.Parse(ownerID.String); a.OwnerID = &uid }
		if createdBy.Valid { uid, _ := uuid.Parse(createdBy.String); a.CreatedBy = &uid }
		if len(capJSON) > 0 { _ = json.Unmarshal(capJSON, &a.Capabilities) }
		if len(toolsJSON) > 0 { _ = json.Unmarshal(toolsJSON, &a.Tools) }
		out = append(out, &a)
	}
	return out, nil
}

func (db *DB) AddProjectAgent(projectID, agentID uuid.UUID, role string, addedBy uuid.UUID) error {
	if role == "" { role = "collaborator" }
	_, err := db.Exec(`INSERT INTO project_agents (project_id, agent_id, role, added_by) VALUES ($1,$2,$3,$4) ON CONFLICT (project_id, agent_id) DO UPDATE SET role=EXCLUDED.role`, projectID, agentID, role, addedBy)
	return err
}

func (db *DB) RemoveProjectAgent(projectID, agentID uuid.UUID) error {
	_, err := db.Exec(`DELETE FROM project_agents WHERE project_id=$1 AND agent_id=$2`, projectID, agentID)
	return err
}

// ——— Channels

func (db *DB) CreateChannel(orgID uuid.UUID, projectID *uuid.UUID, name, displayName, description, topic, channelType string, createdBy uuid.UUID) (*domain.Channel, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if displayName == "" { displayName = name }
	if channelType == "" { channelType = "standard" }
	var ch domain.Channel
	err := db.QueryRow(`INSERT INTO channels (organization_id,project_id,name,display_name,description,topic,channel_type,created_by) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id,organization_id,project_id,name,display_name,description,topic,channel_type,created_by,created_at,updated_at`,
		orgID, projectID, name, displayName, description, topic, channelType, createdBy).Scan(&ch.ID, &ch.OrganizationID, &ch.ProjectID, &ch.Name, &ch.DisplayName, &ch.Description, &ch.Topic, &ch.ChannelType, &ch.CreatedBy, &ch.CreatedAt, &ch.UpdatedAt)
	if err != nil { return nil, err }
	_, _ = db.Exec(`INSERT INTO channel_members (channel_id,member_type,user_id,role) VALUES ($1,'user',$2,'admin') ON CONFLICT DO NOTHING`, ch.ID, createdBy)
	return &ch, nil
}

func (db *DB) ListChannels(orgID uuid.UUID, projectID *uuid.UUID) ([]*domain.Channel, error) {
	var rows *sql.Rows
	var err error
	if projectID != nil {
		rows, err = db.Query(`SELECT id,organization_id,project_id,name,display_name,description,topic,channel_type,created_by,created_at,updated_at FROM channels WHERE organization_id=$1 AND project_id=$2 ORDER BY name`, orgID, *projectID)
	} else {
		rows, err = db.Query(`SELECT id,organization_id,project_id,name,display_name,description,topic,channel_type,created_by,created_at,updated_at FROM channels WHERE organization_id=$1 ORDER BY name`, orgID)
	}
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.Channel
	for rows.Next() {
		var ch domain.Channel
		if err := rows.Scan(&ch.ID, &ch.OrganizationID, &ch.ProjectID, &ch.Name, &ch.DisplayName, &ch.Description, &ch.Topic, &ch.ChannelType, &ch.CreatedBy, &ch.CreatedAt, &ch.UpdatedAt); err != nil { return nil, err }
		out = append(out, &ch)
	}
	return out, nil
}

func (db *DB) GetChannel(orgID, channelID uuid.UUID) (*domain.Channel, error) {
	var ch domain.Channel
	err := db.QueryRow(`SELECT id,organization_id,project_id,name,display_name,description,topic,channel_type,created_by,created_at,updated_at FROM channels WHERE id=$1 AND organization_id=$2`, channelID, orgID).Scan(
		&ch.ID, &ch.OrganizationID, &ch.ProjectID, &ch.Name, &ch.DisplayName, &ch.Description, &ch.Topic, &ch.ChannelType, &ch.CreatedBy, &ch.CreatedAt, &ch.UpdatedAt)
	if err != nil { return nil, err }
	return &ch, nil
}

// GeneralChannelName is the canonical org-level home channel.
const GeneralChannelName = "general"

func (db *DB) GetChannelByName(orgID uuid.UUID, name string) (*domain.Channel, error) {
	// Normalize to match CreateChannel (lowercase, trimmed).
	name = strings.ToLower(strings.TrimSpace(name))
	var id uuid.UUID
	err := db.QueryRow(`SELECT id FROM channels WHERE organization_id=$1 AND name=$2 LIMIT 1`, orgID, name).Scan(&id)
	if err != nil {
		return nil, err
	}
	return db.GetChannel(orgID, id)
}

func (db *DB) EnsureGeneralChannel(orgID, ownerID uuid.UUID) (*domain.Channel, error) {
	if ch, err := db.GetChannelByName(orgID, GeneralChannelName); err == nil {
		return ch, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	ch, err := db.CreateChannel(orgID, nil, GeneralChannelName, "General", "Home for humans and agents", "", "standard", ownerID)
	if err != nil {
		if existing, gerr := db.GetChannelByName(orgID, GeneralChannelName); gerr == nil {
			return existing, nil
		}
		return nil, err
	}
	// Best-effort welcome message: never fail bootstrap if this insert fails.
	_, _ = db.Exec(`INSERT INTO messages (organization_id,channel_id,sender_type,body) VALUES ($1,$2,'system',$3)`, orgID, ch.ID, "Welcome to #general — humans and agents collaborate here. Mention an agent or just ask.")
	return ch, nil
}

func (db *DB) UpdateChannel(orgID, channelID uuid.UUID, displayName, description, topic *string) (*domain.Channel, error) {
	ch, err := db.GetChannel(orgID, channelID)
	if err != nil { return nil, err }
	if displayName != nil { ch.DisplayName = *displayName }
	if description != nil { ch.Description = *description }
	if topic != nil { ch.Topic = *topic }
	err = db.QueryRow(`UPDATE channels SET display_name=$3,description=$4,topic=$5,updated_at=now() WHERE id=$1 AND organization_id=$2 RETURNING id,organization_id,project_id,name,display_name,description,topic,channel_type,created_by,created_at,updated_at`,
		channelID, orgID, ch.DisplayName, ch.Description, ch.Topic).Scan(&ch.ID, &ch.OrganizationID, &ch.ProjectID, &ch.Name, &ch.DisplayName, &ch.Description, &ch.Topic, &ch.ChannelType, &ch.CreatedBy, &ch.CreatedAt, &ch.UpdatedAt)
	if err != nil { return nil, err }
	return ch, nil
}

func (db *DB) RenameChannel(orgID, channelID uuid.UUID, newName string) (*domain.Channel, error) {
	newName = strings.ToLower(strings.TrimSpace(newName))
	var ch domain.Channel
	err := db.QueryRow(`UPDATE channels SET name=$3,updated_at=now() WHERE id=$1 AND organization_id=$2 RETURNING id,organization_id,project_id,name,display_name,description,topic,channel_type,created_by,created_at,updated_at`, channelID, orgID, newName).Scan(
		&ch.ID, &ch.OrganizationID, &ch.ProjectID, &ch.Name, &ch.DisplayName, &ch.Description, &ch.Topic, &ch.ChannelType, &ch.CreatedBy, &ch.CreatedAt, &ch.UpdatedAt)
	if err != nil { return nil, err }
	return &ch, nil
}

func (db *DB) ArchiveChannel(orgID, channelID uuid.UUID) error {
	// For Phase 1, delete channel (cascades messages)
	_, err := db.Exec(`DELETE FROM channels WHERE id=$1 AND organization_id=$2`, channelID, orgID)
	return err
}

func (db *DB) IsChannelMember(channelID, userID uuid.UUID) bool {
	var id uuid.UUID
	err := db.QueryRow(`SELECT id FROM channel_members WHERE channel_id=$1 AND member_type='user' AND user_id=$2`, channelID, userID).Scan(&id)
	return err == nil
}

func (db *DB) ListChannelMembers(channelID uuid.UUID) ([]*domain.ChannelMember, error) {
	rows, err := db.Query(`SELECT id,channel_id,member_type,user_id,agent_id,role,joined_at FROM channel_members WHERE channel_id=$1 ORDER BY joined_at`, channelID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.ChannelMember
	for rows.Next() {
		var m domain.ChannelMember
		if err := rows.Scan(&m.ID, &m.ChannelID, &m.MemberType, &m.UserID, &m.AgentID, &m.Role, &m.JoinedAt); err != nil { return nil, err }
		out = append(out, &m)
	}
	return out, nil
}

func (db *DB) AddChannelMember(channelID, userID uuid.UUID, role string) error {
	if role == "" { role = "member" }
	_, err := db.Exec(`INSERT INTO channel_members (channel_id,member_type,user_id,role) VALUES ($1,'user',$2,$3) ON CONFLICT (channel_id,member_type,user_id,agent_id) DO NOTHING`, channelID, userID, role)
	return err
}

func (db *DB) RemoveChannelMember(channelID, userID uuid.UUID) error {
	_, err := db.Exec(`DELETE FROM channel_members WHERE channel_id=$1 AND member_type='user' AND user_id=$2`, channelID, userID)
	return err
}

// ——— Messages

func (db *DB) CreateMessage(orgID, channelID uuid.UUID, threadID *uuid.UUID, senderType string, senderUserID *uuid.UUID, body string) (*domain.Message, error) {
	if body == "" { return nil, fmt.Errorf("body required") }
	var m domain.Message
	var metadata []byte
	var edited, deleted sql.NullTime
	var senderAgent sql.NullString
	var senderUserVal, senderAgentVal sql.NullString
	if senderType == "agent" && senderUserID != nil {
		senderAgentVal = sql.NullString{String: senderUserID.String(), Valid: true}
	} else if senderUserID != nil {
		senderUserVal = sql.NullString{String: senderUserID.String(), Valid: true}
	}
	err := db.QueryRow(`INSERT INTO messages (organization_id,channel_id,thread_id,sender_type,sender_user_id,sender_agent_id,body) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id,organization_id,channel_id,thread_id,sender_type,sender_user_id,sender_agent_id,body,body_format,metadata,edited_at,deleted_at,created_at,updated_at`,
		orgID, channelID, threadID, senderType, senderUserVal, senderAgentVal, body).Scan(&m.ID, &m.OrganizationID, &m.ChannelID, &m.ThreadID, &m.SenderType, &m.SenderUserID, &senderAgent, &m.Body, &m.BodyFormat, &metadata, &edited, &deleted, &m.CreatedAt, &m.UpdatedAt)
	if err != nil { return nil, err }
	if senderAgent.Valid {
		uid, _ := uuid.Parse(senderAgent.String)
		m.SenderAgentID = &uid
	}
	if len(metadata) > 0 { _ = json.Unmarshal(metadata, &m.Metadata) }
	if edited.Valid { m.EditedAt = &edited.Time }
	if deleted.Valid { m.DeletedAt = &deleted.Time }
	// Update thread metadata if reply
	if threadID != nil {
		_, _ = db.Exec(`INSERT INTO message_threads (root_message_id,channel_id,reply_count,last_reply_at) VALUES ($1,$2,1,now()) ON CONFLICT (root_message_id) DO UPDATE SET reply_count=message_threads.reply_count+1, last_reply_at=now(), updated_at=now()`, *threadID, channelID)
	}
	return &m, nil
}

func (db *DB) GetMessage(orgID, msgID uuid.UUID) (*domain.Message, error) {
	var m domain.Message
	var metadata []byte
	var edited, deleted sql.NullTime
	var senderAgentID sql.NullString
	err := db.QueryRow(`SELECT id,organization_id,channel_id,thread_id,sender_type,sender_user_id,sender_agent_id,body,body_format,metadata,edited_at,deleted_at,created_at,updated_at FROM messages WHERE id=$1 AND organization_id=$2`, msgID, orgID).Scan(
		&m.ID, &m.OrganizationID, &m.ChannelID, &m.ThreadID, &m.SenderType, &m.SenderUserID, &senderAgentID, &m.Body, &m.BodyFormat, &metadata, &edited, &deleted, &m.CreatedAt, &m.UpdatedAt)
	if err != nil { return nil, err }
	if len(metadata) > 0 { _ = json.Unmarshal(metadata, &m.Metadata) }
	if edited.Valid { m.EditedAt = &edited.Time }
	if deleted.Valid { m.DeletedAt = &deleted.Time }
	if senderAgentID.Valid { uid, _ := uuid.Parse(senderAgentID.String); m.SenderAgentID = &uid }
	return &m, nil
}

func (db *DB) ListMessages(orgID, channelID uuid.UUID, threadID *uuid.UUID, before time.Time, limit int, includeDeleted bool) ([]*domain.Message, error) {
	if limit <= 0 || limit > 100 { limit = 20 }
	query := `SELECT id,organization_id,channel_id,thread_id,sender_type,sender_user_id,sender_agent_id,body,body_format,metadata,edited_at,deleted_at,created_at,updated_at FROM messages WHERE organization_id=$1 AND channel_id=$2`
	args := []any{orgID, channelID}
	idx := 3
	if threadID != nil {
		query += fmt.Sprintf(` AND thread_id=$%d`, idx)
		args = append(args, *threadID)
		idx++
	} else {
		query += ` AND thread_id IS NULL`
	}
	if !includeDeleted {
		query += ` AND deleted_at IS NULL`
	}
	if !before.IsZero() {
		query += fmt.Sprintf(` AND created_at < $%d`, idx)
		args = append(args, before)
		idx++
	}
	query += ` ORDER BY created_at DESC`
	query += fmt.Sprintf(` LIMIT $%d`, idx)
	args = append(args, limit)
	rows, err := db.Query(query, args...)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.Message
	for rows.Next() {
		var m domain.Message
		var metadata []byte
		var edited, deleted sql.NullTime
		var senderAgent sql.NullString
		if err := rows.Scan(&m.ID, &m.OrganizationID, &m.ChannelID, &m.ThreadID, &m.SenderType, &m.SenderUserID, &senderAgent, &m.Body, &m.BodyFormat, &metadata, &edited, &deleted, &m.CreatedAt, &m.UpdatedAt); err != nil { return nil, err }
		if len(metadata) > 0 { _ = json.Unmarshal(metadata, &m.Metadata) }
		if edited.Valid { m.EditedAt = &edited.Time }
		if deleted.Valid { m.DeletedAt = &deleted.Time }
		if senderAgent.Valid { uid, _ := uuid.Parse(senderAgent.String); m.SenderAgentID = &uid }
		out = append(out, &m)
	}
	return out, nil
}

func (db *DB) UpdateMessage(orgID, msgID uuid.UUID, body string) (*domain.Message, error) {
	var m domain.Message
	var metadata []byte
	var edited, deleted sql.NullTime
	var senderAgent sql.NullString
	err := db.QueryRow(`UPDATE messages SET body=$3, edited_at=now(), updated_at=now() WHERE id=$1 AND organization_id=$2 AND deleted_at IS NULL RETURNING id,organization_id,channel_id,thread_id,sender_type,sender_user_id,sender_agent_id,body,body_format,metadata,edited_at,deleted_at,created_at,updated_at`, msgID, orgID, body).Scan(
		&m.ID, &m.OrganizationID, &m.ChannelID, &m.ThreadID, &m.SenderType, &m.SenderUserID, &senderAgent, &m.Body, &m.BodyFormat, &metadata, &edited, &deleted, &m.CreatedAt, &m.UpdatedAt)
	if err != nil { return nil, err }
	if len(metadata) > 0 { _ = json.Unmarshal(metadata, &m.Metadata) }
	if edited.Valid { m.EditedAt = &edited.Time }
	if deleted.Valid { m.DeletedAt = &deleted.Time }
	return &m, nil
}

func (db *DB) DeleteMessage(orgID, msgID uuid.UUID) error {
	_, err := db.Exec(`UPDATE messages SET deleted_at=now(), updated_at=now() WHERE id=$1 AND organization_id=$2`, msgID, orgID)
	return err
}

func (db *DB) CountThreadReplies(threadID uuid.UUID) int {
	var n int
	_ = db.QueryRow(`SELECT count(*) FROM messages WHERE thread_id=$1 AND deleted_at IS NULL`, threadID).Scan(&n)
	return n
}

// Reactions

func (db *DB) AddReaction(orgID, messageID, userID uuid.UUID, emoji string) (*domain.Reaction, error) {
	var r domain.Reaction
	err := db.QueryRow(`INSERT INTO reactions (organization_id,message_id,user_id,emoji) VALUES ($1,$2,$3,$4) RETURNING id,organization_id,message_id,user_id,emoji,created_at`, orgID, messageID, userID, emoji).Scan(&r.ID, &r.OrganizationID, &r.MessageID, &r.UserID, &r.Emoji, &r.CreatedAt)
	if err != nil { return nil, err }
	return &r, nil
}

func (db *DB) RemoveReaction(orgID, messageID, userID uuid.UUID, emoji string) error {
	_, err := db.Exec(`DELETE FROM reactions WHERE organization_id=$1 AND message_id=$2 AND user_id=$3 AND emoji=$4`, orgID, messageID, userID, emoji)
	return err
}

func (db *DB) ListReactions(messageID uuid.UUID) ([]*domain.Reaction, error) {
	rows, err := db.Query(`SELECT id,organization_id,message_id,user_id,emoji,created_at FROM reactions WHERE message_id=$1 ORDER BY created_at`, messageID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.Reaction
	for rows.Next() {
		var r domain.Reaction
		if err := rows.Scan(&r.ID, &r.OrganizationID, &r.MessageID, &r.UserID, &r.Emoji, &r.CreatedAt); err != nil { return nil, err }
		out = append(out, &r)
	}
	return out, nil
}

// Hydrate reactions for batch
func (db *DB) HydrateReactions(messages []*domain.Message) error {
	for _, m := range messages {
		reactions, _ := db.ListReactions(m.ID)
		if len(reactions) > 0 {
			m.Reactions = make([]domain.Reaction, len(reactions))
			for i, r := range reactions { m.Reactions[i] = *r }
		}
		m.ThreadCount = db.CountThreadReplies(m.ID)
	}
	return nil
}
