package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"openagent/internal/domain"
)

func (db *DB) CreateTeam(orgID uuid.UUID, name, objective, description string, createdBy uuid.UUID) (*domain.AgentTeam, error) {
	slug := slugify(name)
	base := slug
	var team domain.AgentTeam
	for i:=0; i<5; i++ {
		err := db.QueryRow(`INSERT INTO agent_teams (organization_id, name, slug, objective, description, created_by) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id, organization_id, name, slug, objective, description, status, created_by, created_at, updated_at`,
			orgID, name, slug, objective, description, createdBy).Scan(&team.ID, &team.OrganizationID, &team.Name, &team.Slug, &team.Objective, &team.Description, &team.Status, &team.CreatedBy, &team.CreatedAt, &team.UpdatedAt)
		if err == nil { return &team, nil }
		if !strings.Contains(err.Error(), "duplicate") { return nil, err }
		slug = fmt.Sprintf("%s-%s", base, uuid.NewString()[:4])
	}
	return nil, fmt.Errorf("failed to create team")
}

func (db *DB) GetTeam(orgID, teamID uuid.UUID) (*domain.AgentTeam, error) {
	var t domain.AgentTeam
	var coord sql.NullString
	err := db.QueryRow(`SELECT id, organization_id, name, slug, objective, description, status, coordinator_agent_id, created_by, created_at, updated_at FROM agent_teams WHERE id=$1 AND organization_id=$2`, teamID, orgID).Scan(
		&t.ID, &t.OrganizationID, &t.Name, &t.Slug, &t.Objective, &t.Description, &t.Status, &coord, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if coord.Valid { uid, _ := uuid.Parse(coord.String); t.CoordinatorAgentID = &uid }
	if err != nil { return nil, err }
	// hydrate members
	members, _ := db.ListTeamMembers(teamID)
	t.Members = members
	// hydrate version
	if ver, err := db.GetTeamVersion(teamID, 1); err == nil { t.CurrentVersion = ver }
	return &t, nil
}

func (db *DB) ListTeams(orgID uuid.UUID) ([]*domain.AgentTeam, error) {
	rows, err := db.Query(`SELECT id, organization_id, name, slug, objective, description, status, coordinator_agent_id, created_by, created_at, updated_at FROM agent_teams WHERE organization_id=$1 ORDER BY created_at DESC`, orgID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.AgentTeam
	for rows.Next() {
		var t domain.AgentTeam
		var coord sql.NullString
		if err := rows.Scan(&t.ID, &t.OrganizationID, &t.Name, &t.Slug, &t.Objective, &t.Description, &t.Status, &coord, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil { return nil, err }
		if coord.Valid { uid, _ := uuid.Parse(coord.String); t.CoordinatorAgentID = &uid }
		out = append(out, &t)
	}
	return out, nil
}

func (db *DB) CreateTeamVersion(teamID uuid.UUID, version int, objective string, workflow []domain.WorkflowStep, commRules, delegRules []string, perms, approval map[string]any, config map[string]any, createdBy uuid.UUID) (*domain.AgentTeamVersion, error) {
	wfJSON, _ := json.Marshal(workflow)
	commJSON, _ := json.Marshal(commRules)
	delegJSON, _ := json.Marshal(delegRules)
	permsJSON, _ := json.Marshal(perms)
	approvalJSON, _ := json.Marshal(approval)
	configJSON, _ := json.Marshal(config)
	var v domain.AgentTeamVersion
	err := db.QueryRow(
		`INSERT INTO agent_team_versions (team_id, version, objective, workflow, communication_rules, delegation_rules, permissions, approval_policy, config, created_by) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id, team_id, version, objective, workflow, communication_rules, delegation_rules, permissions, approval_policy, config, created_by, created_at`,
		teamID, version, objective, wfJSON, commJSON, delegJSON, permsJSON, approvalJSON, configJSON, createdBy,
	).Scan(&v.ID, &v.TeamID, &v.Version, &v.Objective, &wfJSON, &commJSON, &delegJSON, &permsJSON, &approvalJSON, &configJSON, &v.CreatedBy, &v.CreatedAt)
	if err != nil { return nil, err }
	_ = json.Unmarshal(wfJSON, &v.Workflow)
	_ = json.Unmarshal(commJSON, &v.CommunicationRules)
	_ = json.Unmarshal(delegJSON, &v.DelegationRules)
	_ = json.Unmarshal(permsJSON, &v.Permissions)
	_ = json.Unmarshal(approvalJSON, &v.ApprovalPolicy)
	_ = json.Unmarshal(configJSON, &v.Config)
	return &v, nil
}

func (db *DB) GetTeamVersion(teamID uuid.UUID, version int) (*domain.AgentTeamVersion, error) {
	var v domain.AgentTeamVersion
	var wfJSON, commJSON, delegJSON, permsJSON, approvalJSON, configJSON []byte
	err := db.QueryRow(`SELECT id, team_id, version, objective, workflow, communication_rules, delegation_rules, permissions, approval_policy, config, created_by, created_at FROM agent_team_versions WHERE team_id=$1 AND version=$2`, teamID, version).Scan(
		&v.ID, &v.TeamID, &v.Version, &v.Objective, &wfJSON, &commJSON, &delegJSON, &permsJSON, &approvalJSON, &configJSON, &v.CreatedBy, &v.CreatedAt)
	if err != nil { return nil, err }
	_ = json.Unmarshal(wfJSON, &v.Workflow)
	_ = json.Unmarshal(commJSON, &v.CommunicationRules)
	_ = json.Unmarshal(delegJSON, &v.DelegationRules)
	_ = json.Unmarshal(permsJSON, &v.Permissions)
	_ = json.Unmarshal(approvalJSON, &v.ApprovalPolicy)
	_ = json.Unmarshal(configJSON, &v.Config)
	return &v, nil
}

func (db *DB) ListTeamVersions(teamID uuid.UUID) ([]*domain.AgentTeamVersion, error) {
	rows, err := db.Query(`SELECT id, team_id, version, objective, workflow, communication_rules, delegation_rules, permissions, approval_policy, config, created_by, created_at FROM agent_team_versions WHERE team_id=$1 ORDER BY version`, teamID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.AgentTeamVersion
	for rows.Next() {
		var v domain.AgentTeamVersion
		var wfJSON, commJSON, delegJSON, permsJSON, approvalJSON, configJSON []byte
		if err := rows.Scan(&v.ID, &v.TeamID, &v.Version, &v.Objective, &wfJSON, &commJSON, &delegJSON, &permsJSON, &approvalJSON, &configJSON, &v.CreatedBy, &v.CreatedAt); err != nil { return nil, err }
		_ = json.Unmarshal(wfJSON, &v.Workflow)
		_ = json.Unmarshal(commJSON, &v.CommunicationRules)
		_ = json.Unmarshal(delegJSON, &v.DelegationRules)
		_ = json.Unmarshal(permsJSON, &v.Permissions)
		_ = json.Unmarshal(approvalJSON, &v.ApprovalPolicy)
		_ = json.Unmarshal(configJSON, &v.Config)
		out = append(out, &v)
	}
	return out, nil
}

func (db *DB) AddTeamMember(teamID, agentID uuid.UUID, role, responsibilities string, dependencies []string, tools, knowledge []string) (*domain.AgentTeamMember, error) {
	depJSON, _ := json.Marshal(dependencies)
	toolsJSON, _ := json.Marshal(tools)
	knowJSON, _ := json.Marshal(knowledge)
	var m domain.AgentTeamMember
	var depStr sql.NullString
	var toolsStr, knowStr []byte
	err := db.QueryRow(
		`INSERT INTO agent_team_members (team_id, agent_id, role, responsibilities, dependencies, tools, knowledge_requirements) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id, team_id, agent_id, role, responsibilities, dependencies, tools, knowledge_requirements, added_at`,
		teamID, agentID, role, responsibilities, depJSON, toolsJSON, knowJSON,
	).Scan(&m.ID, &m.TeamID, &m.AgentID, &m.Role, &m.Responsibilities, &depStr, &toolsStr, &knowStr, &m.AddedAt)
	if err != nil { return nil, err }
	if depStr.Valid { _ = json.Unmarshal([]byte(depStr.String), &m.Dependencies) }
	if len(toolsStr)>0 { _ = json.Unmarshal(toolsStr, &m.Tools) }
	if len(knowStr)>0 { _ = json.Unmarshal(knowStr, &m.KnowledgeRequirements) }
	return &m, nil
}

func (db *DB) ListTeamMembers(teamID uuid.UUID) ([]domain.AgentTeamMember, error) {
	rows, err := db.Query(`SELECT id, team_id, agent_id, role, responsibilities, dependencies, tools, knowledge_requirements, added_at FROM agent_team_members WHERE team_id=$1`, teamID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []domain.AgentTeamMember
	for rows.Next() {
		var m domain.AgentTeamMember
		var depStr sql.NullString
		var toolsStr, knowStr []byte
		if err := rows.Scan(&m.ID, &m.TeamID, &m.AgentID, &m.Role, &m.Responsibilities, &depStr, &toolsStr, &knowStr, &m.AddedAt); err != nil { return nil, err }
		if depStr.Valid { _ = json.Unmarshal([]byte(depStr.String), &m.Dependencies) }
		if len(toolsStr)>0 { _ = json.Unmarshal(toolsStr, &m.Tools) }
		if len(knowStr)>0 { _ = json.Unmarshal(knowStr, &m.KnowledgeRequirements) }
		// hydrate agent
		if agent, err := db.GetAgentByID(m.AgentID); err == nil { m.Agent = agent }
		out = append(out, m)
	}
	return out, nil
}

func (db *DB) RemoveTeamMember(teamID, agentID uuid.UUID) error {
	_, err := db.Exec(`DELETE FROM agent_team_members WHERE team_id=$1 AND agent_id=$2`, teamID, agentID)
	return err
}

// Task dependencies

func (db *DB) AddTaskDependency(taskID, dependsOn uuid.UUID) error {
	// cycle detection: check if dependsOn already depends on taskID (would create cycle)
	visited := map[uuid.UUID]bool{taskID: true}
	queue := []uuid.UUID{dependsOn}
	for len(queue)>0 {
		cur := queue[0]; queue=queue[1:]
		if cur == taskID { return fmt.Errorf("cycle detected") }
		if visited[cur] { continue }
		visited[cur]=true
		rows, _ := db.Query(`SELECT depends_on_task_id FROM task_dependencies WHERE task_id=$1`, cur)
		if rows != nil {
			for rows.Next() {
				var dep uuid.UUID
				rows.Scan(&dep)
				queue = append(queue, dep)
			}
			rows.Close()
		}
	}
	_, err := db.Exec(`INSERT INTO task_dependencies (task_id, depends_on_task_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, taskID, dependsOn)
	return err
}

func (db *DB) ListTaskDependencies(taskID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := db.Query(`SELECT depends_on_task_id FROM task_dependencies WHERE task_id=$1`, taskID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil { return nil, err }
		out = append(out, id)
	}
	return out, nil
}

func (db *DB) IsTaskBlocked(taskID uuid.UUID) (bool, error) {
	rows, err := db.Query(`SELECT depends_on_task_id FROM task_dependencies WHERE task_id=$1`, taskID)
	if err != nil { return false, err }
	defer rows.Close()
	for rows.Next() {
		var depID uuid.UUID
		rows.Scan(&depID)
		var status string
		_ = db.QueryRow(`SELECT status FROM tasks WHERE id=$1`, depID).Scan(&status)
		if status != "completed" { return true, nil }
	}
	return false, nil
}

// Project-Teams

func (db *DB) AssignTeamToProject(projectID, teamID uuid.UUID, assignedBy uuid.UUID) error {
	_, err := db.Exec(`INSERT INTO project_teams (project_id, team_id, assigned_by) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, projectID, teamID, assignedBy)
	return err
}

func (db *DB) ListProjectTeams(projectID uuid.UUID) ([]*domain.AgentTeam, error) {
	rows, err := db.Query(`SELECT t.id, t.organization_id, t.name, t.slug, t.objective, t.description, t.status, t.created_by, t.created_at, t.updated_at FROM agent_teams t JOIN project_teams pt ON pt.team_id=t.id WHERE pt.project_id=$1`, projectID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.AgentTeam
	for rows.Next() {
		var t domain.AgentTeam
		var coord sql.NullString
		if err := rows.Scan(&t.ID, &t.OrganizationID, &t.Name, &t.Slug, &t.Objective, &t.Description, &t.Status, &coord, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil { return nil, err }
		if coord.Valid { uid, _ := uuid.Parse(coord.String); t.CoordinatorAgentID = &uid }
		out = append(out, &t)
	}
	return out, nil
}

// Team activity
func (db *DB) AddTeamActivity(teamID, orgID uuid.UUID, actorType string, actorID *uuid.UUID, eventType string, payload map[string]any) error {
	payloadJSON, _ := json.Marshal(payload)
	_, err := db.Exec(`INSERT INTO team_activity (team_id, organization_id, actor_type, actor_id, event_type, payload) VALUES ($1,$2,$3,$4,$5,$6)`, teamID, orgID, actorType, actorID, eventType, payloadJSON)
	return err
}

func (db *DB) ListTeamActivity(teamID uuid.UUID, limit int) ([]*domain.TeamActivity, error) {
	if limit<=0 || limit>50 { limit=20 }
	rows, err := db.Query(`SELECT id, team_id, organization_id, actor_type, actor_id, event_type, payload, created_at FROM team_activity WHERE team_id=$1 ORDER BY created_at DESC LIMIT $2`, teamID, limit)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.TeamActivity
	for rows.Next() {
		var a domain.TeamActivity
		var payload []byte
		var actorID sql.NullString
		if err := rows.Scan(&a.ID, &a.TeamID, &a.OrganizationID, &a.ActorType, &actorID, &a.EventType, &payload, &a.CreatedAt); err != nil { return nil, err }
		if actorID.Valid { uid,_:=uuid.Parse(actorID.String); a.ActorID=&uid }
		if len(payload)>0 { _ = json.Unmarshal(payload, &a.Payload) }
		out = append(out, &a)
	}
	return out, nil
}

func (db *DB) SetTeamCoordinator(orgID, teamID, agentID uuid.UUID) error {
	// Verify agent belongs to team
	var exists bool
	_ = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM agent_team_members WHERE team_id=$1 AND agent_id=$2)`, teamID, agentID).Scan(&exists)
	if !exists {
		return fmt.Errorf("coordinator must belong to team")
	}
	_, err := db.Exec(`UPDATE agent_teams SET coordinator_agent_id=$3, updated_at=now() WHERE id=$1 AND organization_id=$2`, teamID, orgID, agentID)
	return err
}

func (db *DB) GetTeamCoordinator(orgID, teamID uuid.UUID) (*domain.Agent, error) {
	var coord sql.NullString
	err := db.QueryRow(`SELECT coordinator_agent_id FROM agent_teams WHERE id=$1 AND organization_id=$2`, teamID, orgID).Scan(&coord)
	if err != nil { return nil, err }
	if !coord.Valid { return nil, fmt.Errorf("team has no coordinator") }
	aid, _ := uuid.Parse(coord.String)
	return db.GetAgent(orgID, aid)
}
