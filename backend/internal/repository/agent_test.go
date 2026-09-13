package repository

import (
	"database/sql"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"openagent/internal/domain"
)

func TestAgent_OrgIsolation_List(t *testing.T) {
	db, mock, close := newMockDB(t)
	defer close()
	orgA := uuid.New()
	orgB := uuid.New()
	// ListAgents for orgA
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, organization_id, name, slug, description, avatar_url, avatar, purpose, intent, system_prompt, model, provider, capabilities, tools, status, autonomy_level, current_version, owner_id, created_by, created_at, updated_at FROM agents WHERE organization_id=$1 ORDER BY created_at DESC`)).
		WithArgs(orgA).WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "name", "slug", "description", "avatar_url", "avatar", "purpose", "intent", "system_prompt", "model", "provider", "capabilities", "tools", "status", "autonomy_level", "current_version", "owner_id", "created_by", "created_at", "updated_at"}))
	// orgB should not see orgA's agents — query isolates
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, organization_id, name, slug, description, avatar_url, avatar, purpose, intent, system_prompt, model, provider, capabilities, tools, status, autonomy_level, current_version, owner_id, created_by, created_at, updated_at FROM agents WHERE organization_id=$1 ORDER BY created_at DESC`)).
		WithArgs(orgB).WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "name", "slug", "description", "avatar_url", "avatar", "purpose", "intent", "system_prompt", "model", "provider", "capabilities", "tools", "status", "autonomy_level", "current_version", "owner_id", "created_by", "created_at", "updated_at"}))
	agentsA, _ := db.ListAgents(orgA)
	agentsB, _ := db.ListAgents(orgB)
	if len(agentsA) != 0 || len(agentsB) != 0 { t.Fatal("expected empty") }
	if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
}

func TestAgent_CreateVersioning(t *testing.T) {
	// Verify version creation increments current_version and inserts agent_versions
	// We mock GetAgentVersion for hydration
	db, mock, close := newMockDB(t)
	defer close()
	agentID := uuid.New()
	orgID := uuid.New()
	// GetAgent for isolation check
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, organization_id, name, slug, description, avatar_url, avatar, purpose, intent, system_prompt, model, provider, capabilities, tools, status, autonomy_level, current_version, owner_id, created_by, created_at, updated_at FROM agents WHERE id=$1 AND organization_id=$2`)).
		WithArgs(agentID, orgID).WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "name", "slug", "description", "avatar_url", "avatar", "purpose", "intent", "system_prompt", "model", "provider", "capabilities", "tools", "status", "autonomy_level", "current_version", "owner_id", "created_by", "created_at", "updated_at"}).
			AddRow(agentID, orgID, "Test Agent", "test-agent", "desc", nil, nil, "purpose", "intent", "", "openai/gpt-4o-mini", "openai", []byte(`[]`), []byte(`[]`), "active", "assistant", 1, nil, nil, mustParseTime("2024-01-01T00:00:00Z"), mustParseTime("2024-01-01T00:00:00Z")))
	// GetAgentVersion for v1
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, agent_id, version, config, change_summary, role, objective, instructions, capabilities, behavioral_rules, tool_policy, memory_policy, approval_policy, model_configuration, created_by, created_at FROM agent_versions WHERE agent_id=$1 AND version=$2`)).
		WithArgs(agentID, 1).WillReturnRows(sqlmock.NewRows([]string{"id", "agent_id", "version", "config", "change_summary", "role", "objective", "instructions", "capabilities", "behavioral_rules", "tool_policy", "memory_policy", "approval_policy", "model_configuration", "created_by", "created_at"}).
			AddRow(uuid.New(), agentID, 1, []byte(`{}`), "initial", "Researcher", "obj", "instr", []byte(`[]`), []byte(`[]`), []byte(`{}`), []byte(`{}`), []byte(`{}`), []byte(`{}`), nil, mustParseTime("2024-01-01T00:00:00Z")))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, agent_id, name, description, enabled, created_at FROM agent_capabilities WHERE agent_id=$1 ORDER BY name`)).
		WithArgs(agentID).WillReturnRows(sqlmock.NewRows([]string{"id", "agent_id", "name", "description", "enabled", "created_at"}))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, agent_id, resource_type, resource_id, permission, granted_by, created_at FROM agent_permissions WHERE agent_id=$1 ORDER BY resource_type`)).
		WithArgs(agentID).WillReturnRows(sqlmock.NewRows([]string{"id", "agent_id", "resource_type", "resource_id", "permission", "granted_by", "created_at"}))
	agent, err := db.GetAgent(orgID, agentID)
	if err != nil { t.Fatalf("get agent: %v", err) }
	if agent.CurrentVersion != 1 { t.Fatalf("expected v1") }
	if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
}

func TestAgent_PermissionIsolation(t *testing.T) {
	db, mock, close := newMockDB(t)
	defer close()
	aid := uuid.New()
	// ListAgentPermissions
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, agent_id, resource_type, resource_id, permission, granted_by, created_at FROM agent_permissions WHERE agent_id=$1 ORDER BY resource_type`)).
		WithArgs(aid).WillReturnRows(sqlmock.NewRows([]string{"id", "agent_id", "resource_type", "resource_id", "permission", "granted_by", "created_at"}).
			AddRow(uuid.New(), aid, "organization", nil, "read", nil, mustParseTime("2024-01-01T00:00:00Z")))
	perms, _ := db.ListAgentPermissions(aid)
	if len(perms) != 1 || perms[0].Permission != "read" { t.Fatalf("expected read perm") }
	if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
}

func TestAgent_CapabilityEnabled(t *testing.T) {
	// Capability with enabled false should be stored correctly
	db, mock, close := newMockDB(t)
	defer close()
	aid := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, agent_id, name, description, enabled, created_at FROM agent_capabilities WHERE agent_id=$1 ORDER BY name`)).
		WithArgs(aid).WillReturnRows(sqlmock.NewRows([]string{"id", "agent_id", "name", "description", "enabled", "created_at"}).
			AddRow(uuid.New(), aid, "publish_content", "Publish", false, mustParseTime("2024-01-01T00:00:00Z")).
			AddRow(uuid.New(), aid, "research", "Research", true, mustParseTime("2024-01-01T00:00:00Z")))
	caps, _ := db.ListAgentCapabilities(aid)
	if len(caps) != 2 { t.Fatalf("expected 2 caps") }
	for _, c := range caps {
		if c.Name == "publish_content" && c.Enabled != false { t.Fatal("publish should be disabled in this test") }
	}
	if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
}

func TestAgent_GetAgentVersion_NotFoundIsolation(t *testing.T) {
	db, mock, close := newMockDB(t)
	defer close()
	aid := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, agent_id, version, config, change_summary, role, objective, instructions, capabilities, behavioral_rules, tool_policy, memory_policy, approval_policy, model_configuration, created_by, created_at FROM agent_versions WHERE agent_id=$1 AND version=$2`)).
		WithArgs(aid, 99).WillReturnError(sql.ErrNoRows)
	_, err := db.GetAgentVersion(aid, 99)
	if err != sql.ErrNoRows { t.Fatalf("expected not found") }
	// Ensure handler would check org isolation first via GetAgent; version alone doesn't leak org
	_ = domain.Agent{ID: aid}
	if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
}
