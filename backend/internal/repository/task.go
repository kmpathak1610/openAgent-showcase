package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"openagent/internal/domain"
	"openagent/internal/metrics"
)

func scanTask(row interface{ Scan(...any) error }) (*domain.Task, error) {
	var t domain.Task
	var channelID, parentID, assignedAgent, assignedUser, createdBy, correlationID sql.NullString
	var deadline, dueAt sql.NullTime
	var createdAt, updatedAt sql.NullTime
	// we need to know columns order; we'll scan explicitly in each query instead of helper
	_ = channelID; _ = parentID; _ = assignedAgent; _ = assignedUser; _ = createdBy; _ = correlationID
	_ = deadline; _ = dueAt; _ = createdAt; _ = updatedAt
	return &t, fmt.Errorf("use direct scan")
}

// CreateTask creates a task and its initial event
func (db *DB) CreateTask(orgID, projectID uuid.UUID, channelID, parentTaskID, teamID, assignedAgent, assignedUser *uuid.UUID, title, description, priority string, deadline *time.Time, correlationID *uuid.UUID, createdBy uuid.UUID) (*domain.Task, error) {
	if priority == "" { priority = "medium" }
	status := "pending"
	if assignedAgent != nil || assignedUser != nil { status = "assigned" }
	// Use transaction
	tx, err := db.Begin()
	if err != nil { return nil, err }
	defer tx.Rollback()
	var t domain.Task
	var chID sql.NullString
	var pID sql.NullString
	var aAgent sql.NullString
	var aUser sql.NullString
	var corr sql.NullString
	if channelID != nil { chID = sql.NullString{String: channelID.String(), Valid: true} }
	if parentTaskID != nil { pID = sql.NullString{String: parentTaskID.String(), Valid: true} }
	if assignedAgent != nil { aAgent = sql.NullString{String: assignedAgent.String(), Valid: true} }
	if assignedUser != nil { aUser = sql.NullString{String: assignedUser.String(), Valid: true} }
	if correlationID != nil { corr = sql.NullString{String: correlationID.String(), Valid: true} }
	var deadlineSQL sql.NullTime
	if deadline != nil {
		deadlineSQL = sql.NullTime{Time: *deadline, Valid: true}
	}

	var teamIDStr sql.NullString
	if teamID != nil { teamIDStr = sql.NullString{String: teamID.String(), Valid: true} }
	err = tx.QueryRow(
		`INSERT INTO tasks (organization_id, project_id, channel_id, parent_task_id, team_id, title, description, created_by, assigned_to_agent, assigned_to_user, status, priority, deadline, correlation_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING id, organization_id, project_id, channel_id, parent_task_id, team_id, title, description, created_by, assigned_to_agent, assigned_to_user, status, priority, deadline, correlation_id, created_at, updated_at`,
		orgID, projectID, chID, pID, teamIDStr, title, description, createdBy, aAgent, aUser, status, priority, deadlineSQL, corr,
	).Scan(&t.ID, &t.OrganizationID, &t.ProjectID, &chID, &pID, &teamIDStr, &t.Title, &t.Description, &createdBy, &aAgent, &aUser, &t.Status, &t.Priority, &deadlineSQL, &corr, &t.CreatedAt, &t.UpdatedAt)
	if teamIDStr.Valid { uid, _ := uuid.Parse(teamIDStr.String); t.TeamID = &uid }
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}
	if chID.Valid { uid, _ := uuid.Parse(chID.String); t.ChannelID = &uid }
	if pID.Valid { uid, _ := uuid.Parse(pID.String); t.ParentTaskID = &uid }
	if aAgent.Valid { uid, _ := uuid.Parse(aAgent.String); t.AssignedToAgent = &uid }
	if aUser.Valid { uid, _ := uuid.Parse(aUser.String); t.AssignedToUser = &uid }
	if corr.Valid { uid, _ := uuid.Parse(corr.String); t.CorrelationID = &uid }
	if deadlineSQL.Valid { t.Deadline = &deadlineSQL.Time; t.DueAt = &deadlineSQL.Time }
	t.CreatedBy = &createdBy
	t.OrganizationID = orgID
	t.ProjectID = projectID
	// create task_assignments for compatibility
	if assignedAgent != nil {
		_, _ = tx.Exec(`INSERT INTO task_assignments (task_id, assignee_type, assignee_agent_id, assigned_by) VALUES ($1,'agent',$2,$3) ON CONFLICT DO NOTHING`, t.ID, *assignedAgent, createdBy)
	} else if assignedUser != nil {
		_, _ = tx.Exec(`INSERT INTO task_assignments (task_id, assignee_type, assignee_user_id, assigned_by) VALUES ($1,'user',$2,$3) ON CONFLICT DO NOTHING`, t.ID, *assignedUser, createdBy)
	}
	// event created
	payload, _ := json.Marshal(map[string]any{"title": title, "status": status})
	_, err = tx.Exec(`INSERT INTO task_events (task_id, actor_type, actor_user_id, event_type, payload, correlation_id) VALUES ($1,'user',$2,'created',$3,$4)`, t.ID, createdBy, payload, corr)
	if err != nil { return nil, err }
	if assignedAgent != nil || assignedUser != nil {
		payload2, _ := json.Marshal(map[string]any{"assigned_to_agent": assignedAgent, "assigned_to_user": assignedUser})
		_, _ = tx.Exec(`INSERT INTO task_events (task_id, actor_type, actor_user_id, event_type, payload, correlation_id) VALUES ($1,'user',$2,'assigned',$3,$4)`, t.ID, createdBy, payload2, corr)
	}
	if err = tx.Commit(); err != nil { return nil, err }
	metrics.IncTasks()
	return &t, nil
}

// Simpler CreateTask wrapper for handler
func (db *DB) CreateTaskSimple(t *domain.Task) (*domain.Task, error) {
	return db.CreateTask(t.OrganizationID, t.ProjectID, t.ChannelID, t.ParentTaskID, t.TeamID, t.AssignedToAgent, t.AssignedToUser, t.Title, t.Description, t.Priority, t.Deadline, t.CorrelationID, *t.CreatedBy)
}

func (db *DB) GetTask(orgID, taskID uuid.UUID) (*domain.Task, error) {
	var t domain.Task
	var chID, parentID, teamID, assignedAgent, assignedUser, createdBy, correlationID sql.NullString
	var deadline sql.NullTime
	var reqRole sql.NullString
	var reqCaps []byte
	var prefAgent sql.NullString
	err := db.QueryRow(
		`SELECT id, organization_id, project_id, channel_id, parent_task_id, team_id, title, description, created_by, assigned_to_agent, assigned_to_user, status, priority, deadline, correlation_id, required_role, required_capabilities, preferred_agent_id, created_at, updated_at FROM tasks WHERE id=$1 AND organization_id=$2`,
		taskID, orgID,
	).Scan(&t.ID, &t.OrganizationID, &t.ProjectID, &chID, &parentID, &teamID, &t.Title, &t.Description, &createdBy, &assignedAgent, &assignedUser, &t.Status, &t.Priority, &deadline, &correlationID, &reqRole, &reqCaps, &prefAgent, &t.CreatedAt, &t.UpdatedAt)
	if reqRole.Valid { t.RequiredRole = &reqRole.String }
	if len(reqCaps) > 0 { _ = json.Unmarshal(reqCaps, &t.RequiredCapabilities) }
	if prefAgent.Valid { uid, _ := uuid.Parse(prefAgent.String); t.PreferredAgentID = &uid }
	if err != nil { return nil, err }
	if chID.Valid { uid, _ := uuid.Parse(chID.String); t.ChannelID = &uid }
	if parentID.Valid { uid, _ := uuid.Parse(parentID.String); t.ParentTaskID = &uid }
	if teamID.Valid { uid, _ := uuid.Parse(teamID.String); t.TeamID = &uid }
	if assignedAgent.Valid { uid, _ := uuid.Parse(assignedAgent.String); t.AssignedToAgent = &uid }
	if assignedUser.Valid { uid, _ := uuid.Parse(assignedUser.String); t.AssignedToUser = &uid }
	if createdBy.Valid { uid, _ := uuid.Parse(createdBy.String); t.CreatedBy = &uid }
	if correlationID.Valid { uid, _ := uuid.Parse(correlationID.String); t.CorrelationID = &uid }
	if deadline.Valid { t.Deadline = &deadline.Time; t.DueAt = &deadline.Time }
	return &t, nil
}

func (db *DB) ListTasks(orgID uuid.UUID, projectID *uuid.UUID, channelID *uuid.UUID, status string, assignedAgent *uuid.UUID, limit int) ([]*domain.Task, error) {
	if limit <=0 || limit>100 { limit = 20 }
	query := `SELECT id, organization_id, project_id, channel_id, parent_task_id, team_id, title, description, created_by, assigned_to_agent, assigned_to_user, status, priority, deadline, correlation_id, required_role, required_capabilities, preferred_agent_id, created_at, updated_at FROM tasks WHERE organization_id=$1`
	args := []any{orgID}
	idx := 2
	if projectID != nil {
		query += fmt.Sprintf(` AND project_id=$%d`, idx)
		args = append(args, *projectID)
		idx++
	}
	if channelID != nil {
		query += fmt.Sprintf(` AND channel_id=$%d`, idx)
		args = append(args, *channelID)
		idx++
	}
	if status != "" {
		query += fmt.Sprintf(` AND status=$%d`, idx)
		args = append(args, status)
		idx++
	}
	if assignedAgent != nil {
		query += fmt.Sprintf(` AND assigned_to_agent=$%d`, idx)
		args = append(args, *assignedAgent)
		idx++
	}
	query += ` ORDER BY created_at DESC`
	query += fmt.Sprintf(` LIMIT $%d`, idx)
	args = append(args, limit)
	rows, err := db.Query(query, args...)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.Task
	for rows.Next() {
		var t domain.Task
		var chID, parentID, teamID, assignedAgentStr, assignedUser, createdBy, correlationID sql.NullString
		var deadline sql.NullTime
		var reqRole sql.NullString
		var reqCaps []byte
		var prefAgent sql.NullString
		if err := rows.Scan(&t.ID, &t.OrganizationID, &t.ProjectID, &chID, &parentID, &teamID, &t.Title, &t.Description, &createdBy, &assignedAgentStr, &assignedUser, &t.Status, &t.Priority, &deadline, &correlationID, &reqRole, &reqCaps, &prefAgent, &t.CreatedAt, &t.UpdatedAt); err != nil { return nil, err }
		if reqRole.Valid { t.RequiredRole = &reqRole.String }
		if len(reqCaps) > 0 { _ = json.Unmarshal(reqCaps, &t.RequiredCapabilities) }
		if prefAgent.Valid { uid, _ := uuid.Parse(prefAgent.String); t.PreferredAgentID = &uid }
		if chID.Valid { uid, _ := uuid.Parse(chID.String); t.ChannelID = &uid }
		if parentID.Valid { uid, _ := uuid.Parse(parentID.String); t.ParentTaskID = &uid }
		if teamID.Valid { uid, _ := uuid.Parse(teamID.String); t.TeamID = &uid }
		if assignedAgentStr.Valid { uid, _ := uuid.Parse(assignedAgentStr.String); t.AssignedToAgent = &uid }
		if assignedUser.Valid { uid, _ := uuid.Parse(assignedUser.String); t.AssignedToUser = &uid }
		if createdBy.Valid { uid, _ := uuid.Parse(createdBy.String); t.CreatedBy = &uid }
		if correlationID.Valid { uid, _ := uuid.Parse(correlationID.String); t.CorrelationID = &uid }
		if deadline.Valid { t.Deadline = &deadline.Time; t.DueAt = &deadline.Time }
		out = append(out, &t)
	}
	return out, nil
}

var validTaskTransitions = map[string][]string{
	"pending": {"assigned", "cancelled"},
	"assigned": {"running", "cancelled"},
	"running": {"waiting", "blocked", "approval_required", "completed", "failed", "cancelled"},
	"waiting": {"running", "completed", "failed", "cancelled"},
	"blocked": {"running", "cancelled"},
	"approval_required": {"running", "completed", "failed", "cancelled"},
	"completed": {},
	"failed": {"pending", "cancelled"},
	"cancelled": {},
	"todo": {"assigned", "in_progress", "cancelled"},
	"in_progress": {"completed", "failed", "cancelled"},
	"in_review": {"completed", "failed"},
	"done": {},
}

func isValidTaskTransition(from, to string) bool {
	if from == to { return true }
	allowed, ok := validTaskTransitions[from]
	if !ok { return false }
	for _, a := range allowed {
		if a == to { return true }
	}
	return false
}

func (db *DB) UpdateTaskStatus(orgID, taskID uuid.UUID, newStatus string, actorType string, actorID uuid.UUID, payload map[string]any) (*domain.Task, error) {
	if !domain.ValidTaskStatuses[newStatus] {
		return nil, fmt.Errorf("invalid status %s", newStatus)
	}
	tx, err := db.Begin()
	if err != nil { return nil, err }
	defer tx.Rollback()
	var oldStatus string
	var corr sql.NullString
	err = tx.QueryRow(`SELECT status, correlation_id FROM tasks WHERE id=$1 AND organization_id=$2`, taskID, orgID).Scan(&oldStatus, &corr)
	if err != nil { return nil, err }
	if !isValidTaskTransition(oldStatus, newStatus) {
		return nil, fmt.Errorf("invalid task transition %s -> %s", oldStatus, newStatus)
	}
	_, err = tx.Exec(`UPDATE tasks SET status=$3, updated_at=now() WHERE id=$1 AND organization_id=$2`, taskID, orgID, newStatus)
	if err != nil { return nil, err }
	// event
	payloadJSON, _ := json.Marshal(payload)
	var actorUser, actorAgent sql.NullString
	if actorType == "user" { actorUser = sql.NullString{String: actorID.String(), Valid: true} }
	if actorType == "agent" { actorAgent = sql.NullString{String: actorID.String(), Valid: true} }
	eventType := newStatus
	// map status to event type where possible
	if newStatus == "completed" { eventType = "completed" } else if newStatus == "failed" { eventType = "failed" } else if newStatus == "assigned" { eventType = "assigned" } else if newStatus == "running" { eventType = "started" } else { eventType = "status_changed" }
	// try to insert with correlation
	var corrID *uuid.UUID
	if corr.Valid { uid, _ := uuid.Parse(corr.String); corrID = &uid }
	_, err = tx.Exec(`INSERT INTO task_events (task_id, actor_type, actor_user_id, actor_agent_id, event_type, payload, correlation_id) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		taskID, actorType, actorUser, actorAgent, eventType, payloadJSON, corrID)
	if err != nil { return nil, err }
	if err = tx.Commit(); err != nil { return nil, err }
	if newStatus == "completed" {
		metrics.IncTasksCompleted()
	}
	return db.GetTask(orgID, taskID)
}

func (db *DB) AssignTask(orgID, taskID uuid.UUID, agentID, userID *uuid.UUID, assignedBy uuid.UUID) (*domain.Task, error) {
	tx, err := db.Begin()
	if err != nil { return nil, err }
	defer tx.Rollback()
	var corr sql.NullString
	_ = tx.QueryRow(`SELECT correlation_id FROM tasks WHERE id=$1`, taskID).Scan(&corr)
	var corrID *uuid.UUID
	if corr.Valid { uid, _ := uuid.Parse(corr.String); corrID = &uid }
	_, err = tx.Exec(`UPDATE tasks SET assigned_to_agent=$2, assigned_to_user=$3, status='assigned', updated_at=now() WHERE id=$1 AND organization_id=$4`, taskID, agentID, userID, orgID)
	if err != nil { return nil, err }
	// task_assignments compatibility
	if agentID != nil {
		_, _ = tx.Exec(`INSERT INTO task_assignments (task_id, assignee_type, assignee_agent_id, assigned_by) VALUES ($1,'agent',$2,$3) ON CONFLICT DO NOTHING`, taskID, *agentID, assignedBy)
	} else if userID != nil {
		_, _ = tx.Exec(`INSERT INTO task_assignments (task_id, assignee_type, assignee_user_id, assigned_by) VALUES ($1,'user',$2,$3) ON CONFLICT DO NOTHING`, taskID, *userID, assignedBy)
	}
	payload, _ := json.Marshal(map[string]any{"assigned_to_agent": agentID, "assigned_to_user": userID})
	_, err = tx.Exec(`INSERT INTO task_events (task_id, actor_type, actor_user_id, event_type, payload, correlation_id) VALUES ($1,'user',$2,'assigned',$3,$4)`, taskID, assignedBy.String(), payload, corrID)
	if err != nil { return nil, err }
	if err = tx.Commit(); err != nil { return nil, err }
	return db.GetTask(orgID, taskID)
}

func (db *DB) CreateTaskEvent(taskID uuid.UUID, actorType string, actorID *uuid.UUID, eventType string, payload map[string]any, correlationID *uuid.UUID) (*domain.TaskEvent, error) {
	payloadJSON, _ := json.Marshal(payload)
	var actorUser, actorAgent sql.NullString
	if actorType == "user" && actorID != nil { actorUser = sql.NullString{String: actorID.String(), Valid: true} }
	if actorType == "agent" && actorID != nil { actorAgent = sql.NullString{String: actorID.String(), Valid: true} }
	var ev domain.TaskEvent
	var corr sql.NullString
	if correlationID != nil { corr = sql.NullString{String: correlationID.String(), Valid: true} }
	err := db.QueryRow(
		`INSERT INTO task_events (task_id, actor_type, actor_user_id, actor_agent_id, event_type, payload, correlation_id) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id, task_id, actor_type, actor_user_id, actor_agent_id, event_type, correlation_id, payload, created_at`,
		taskID, actorType, actorUser, actorAgent, eventType, payloadJSON, corr,
	).Scan(&ev.ID, &ev.TaskID, &ev.ActorType, &actorUser, &actorAgent, &ev.EventType, &corr, &payloadJSON, &ev.CreatedAt)
	if err != nil { return nil, err }
	if actorUser.Valid { uid, _ := uuid.Parse(actorUser.String); ev.ActorUserID = &uid }
	if actorAgent.Valid { uid, _ := uuid.Parse(actorAgent.String); ev.ActorAgentID = &uid }
	if corr.Valid { uid, _ := uuid.Parse(corr.String); ev.CorrelationID = &uid }
	_ = json.Unmarshal(payloadJSON, &ev.Payload)
	return &ev, nil
}

func (db *DB) ListTaskEvents(taskID uuid.UUID) ([]*domain.TaskEvent, error) {
	rows, err := db.Query(`SELECT id, task_id, actor_type, actor_user_id, actor_agent_id, event_type, correlation_id, payload, created_at FROM task_events WHERE task_id=$1 ORDER BY created_at ASC`, taskID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.TaskEvent
	for rows.Next() {
		var ev domain.TaskEvent
		var actorUser, actorAgent, corr sql.NullString
		var payload []byte
		if err := rows.Scan(&ev.ID, &ev.TaskID, &ev.ActorType, &actorUser, &actorAgent, &ev.EventType, &corr, &payload, &ev.CreatedAt); err != nil { return nil, err }
		if actorUser.Valid { uid, _ := uuid.Parse(actorUser.String); ev.ActorUserID = &uid }
		if actorAgent.Valid { uid, _ := uuid.Parse(actorAgent.String); ev.ActorAgentID = &uid }
		if corr.Valid { uid, _ := uuid.Parse(corr.String); ev.CorrelationID = &uid }
		if len(payload) > 0 { _ = json.Unmarshal(payload, &ev.Payload) }
		out = append(out, &ev)
	}
	return out, nil
}

func (db *DB) SetTaskRequirements(orgID, taskID uuid.UUID, requiredRole *string, requiredCapabilities []string, preferredAgentID *uuid.UUID) error {
	capsJSON, _ := json.Marshal(requiredCapabilities)
	_, err := db.Exec(`UPDATE tasks SET required_role=$3, required_capabilities=$4, preferred_agent_id=$5, updated_at=now() WHERE id=$1 AND organization_id=$2`, taskID, orgID, requiredRole, capsJSON, preferredAgentID)
	return err
}

func (db *DB) GetEligibleAgents(orgID, projectID uuid.UUID, requiredRole *string, requiredCapabilities []string) ([]*domain.Agent, error) {
	// Get project agents + team agents, filter by status active, and capability/role
	projectAgents, _ := db.ListProjectAgents(projectID)
	// Also include team agents if project has teams
	teamAgentsMap := make(map[uuid.UUID]*domain.Agent)
	for _, ag := range projectAgents {
		teamAgentsMap[ag.ID] = ag
	}
	// Add team members
	rows, _ := db.Query(`SELECT atm.agent_id FROM agent_team_members atm JOIN project_teams pt ON pt.team_id=atm.team_id WHERE pt.project_id=$1`, projectID)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var aid uuid.UUID
			if err := rows.Scan(&aid); err == nil {
				if _, ok := teamAgentsMap[aid]; !ok {
					if ag, err := db.GetAgent(orgID, aid); err == nil {
						teamAgentsMap[aid] = ag
					}
				}
			}
		}
	}
	var eligible []*domain.Agent
	for _, ag := range teamAgentsMap {
		if ag.Status != "active" {
			continue
		}
		// Filter by required_role if specified
		if requiredRole != nil && *requiredRole != "" {
			role := ""
			if ag.Version != nil { role = ag.Version.Role }
			if role == "" { role = ag.Purpose }
			if role != *requiredRole && ag.Name != *requiredRole {
				continue
			}
		}
		// Filter by required_capabilities
		if len(requiredCapabilities) > 0 {
			hasAll := true
			caps := []string{}
			if ag.Version != nil {
				for _, c := range ag.Version.Capabilities { caps = append(caps, c.Name) }
			} else {
				caps = ag.Capabilities
			}
			capSet := make(map[string]bool)
			for _, c := range caps { capSet[c] = true }
			for _, req := range requiredCapabilities {
				if !capSet[req] {
					hasAll = false
					break
				}
			}
			if !hasAll {
				continue
			}
		}
		eligible = append(eligible, ag)
	}
	return eligible, nil
}
