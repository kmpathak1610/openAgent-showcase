package repository

import (
	"database/sql"
	"encoding/json"

	"github.com/google/uuid"
	"openagent/internal/domain"
)

func (db *DB) ListTools(orgID uuid.UUID) ([]*domain.Tool, error) {
	rows, err := db.Query(`SELECT id, organization_id, name, description, input_schema, output_schema, risk_level, required_permissions, approval_policy, enabled, created_at, updated_at FROM tools WHERE organization_id IS NULL OR organization_id=$1 ORDER BY name`, orgID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.Tool
	for rows.Next() {
		var t domain.Tool
		var orgID sql.NullString
		var inputSchema, outputSchema, requiredPerms, approvalPolicy []byte
		if err := rows.Scan(&t.ID, &orgID, &t.Name, &t.Description, &inputSchema, &outputSchema, &t.RiskLevel, &requiredPerms, &approvalPolicy, &t.Enabled, &t.CreatedAt, &t.UpdatedAt); err != nil { return nil, err }
		if orgID.Valid { uid, _ := uuid.Parse(orgID.String); t.OrganizationID = &uid }
		if len(inputSchema)>0 { _ = json.Unmarshal(inputSchema, &t.InputSchema) }
		if len(outputSchema)>0 { _ = json.Unmarshal(outputSchema, &t.OutputSchema) }
		if len(requiredPerms)>0 { _ = json.Unmarshal(requiredPerms, &t.RequiredPermissions) }
		if len(approvalPolicy)>0 { _ = json.Unmarshal(approvalPolicy, &t.ApprovalPolicy) }
		out = append(out, &t)
	}
	return out, nil
}

func (db *DB) GetToolByName(orgID *uuid.UUID, name string) (*domain.Tool, error) {
	var t domain.Tool
	var orgIDStr sql.NullString
	var inputSchema, outputSchema, requiredPerms, approvalPolicy []byte
	var query string
	var args []any
	if orgID != nil {
		query = `SELECT id, organization_id, name, description, input_schema, output_schema, risk_level, required_permissions, approval_policy, enabled, created_at, updated_at FROM tools WHERE organization_id=$1 AND name=$2`
		args = []any{*orgID, name}
	} else {
		query = `SELECT id, organization_id, name, description, input_schema, output_schema, risk_level, required_permissions, approval_policy, enabled, created_at, updated_at FROM tools WHERE organization_id IS NULL AND name=$1`
		args = []any{name}
	}
	err := db.QueryRow(query, args...).Scan(&t.ID, &orgIDStr, &t.Name, &t.Description, &inputSchema, &outputSchema, &t.RiskLevel, &requiredPerms, &approvalPolicy, &t.Enabled, &t.CreatedAt, &t.UpdatedAt)
	if err != nil { return nil, err }
	if orgIDStr.Valid { uid, _ := uuid.Parse(orgIDStr.String); t.OrganizationID = &uid }
	if len(inputSchema)>0 { _ = json.Unmarshal(inputSchema, &t.InputSchema) }
	if len(outputSchema)>0 { _ = json.Unmarshal(outputSchema, &t.OutputSchema) }
	if len(requiredPerms)>0 { _ = json.Unmarshal(requiredPerms, &t.RequiredPermissions) }
	if len(approvalPolicy)>0 { _ = json.Unmarshal(approvalPolicy, &t.ApprovalPolicy) }
	return &t, nil
}

func (db *DB) GetToolByID(id uuid.UUID) (*domain.Tool, error) {
	var t domain.Tool
	var orgIDStr sql.NullString
	var inputSchema, outputSchema, requiredPerms, approvalPolicy []byte
	err := db.QueryRow(`SELECT id, organization_id, name, description, input_schema, output_schema, risk_level, required_permissions, approval_policy, enabled, created_at, updated_at FROM tools WHERE id=$1`, id).Scan(
		&t.ID, &orgIDStr, &t.Name, &t.Description, &inputSchema, &outputSchema, &t.RiskLevel, &requiredPerms, &approvalPolicy, &t.Enabled, &t.CreatedAt, &t.UpdatedAt)
	if err != nil { return nil, err }
	if orgIDStr.Valid { uid, _ := uuid.Parse(orgIDStr.String); t.OrganizationID = &uid }
	if len(inputSchema)>0 { _ = json.Unmarshal(inputSchema, &t.InputSchema) }
	if len(outputSchema)>0 { _ = json.Unmarshal(outputSchema, &t.OutputSchema) }
	if len(requiredPerms)>0 { _ = json.Unmarshal(requiredPerms, &t.RequiredPermissions) }
	if len(approvalPolicy)>0 { _ = json.Unmarshal(approvalPolicy, &t.ApprovalPolicy) }
	return &t, nil
}

func (db *DB) ListAgentTools(agentID uuid.UUID) ([]*domain.AgentTool, error) {
	rows, err := db.Query(`SELECT at.id, at.agent_id, at.tool_id, at.enabled, at.created_at, t.name, t.description, t.risk_level FROM agent_tools at JOIN tools t ON t.id=at.tool_id WHERE at.agent_id=$1`, agentID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.AgentTool
	for rows.Next() {
		var at domain.AgentTool
		var risk string
		var name, desc string
		if err := rows.Scan(&at.ID, &at.AgentID, &at.ToolID, &at.Enabled, &at.CreatedAt, &name, &desc, &risk); err != nil { return nil, err }
		at.Tool = &domain.Tool{Name: name, Description: desc, RiskLevel: risk}
		out = append(out, &at)
	}
	return out, nil
}

func (db *DB) AssignToolToAgent(agentID, toolID uuid.UUID) error {
	_, err := db.Exec(`INSERT INTO agent_tools (agent_id, tool_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, agentID, toolID)
	return err
}

func (db *DB) CreateToolExecution(exec *domain.ToolExecution) (*domain.ToolExecution, error) {
	var out domain.ToolExecution
	var tid, rid, aid sql.NullString
	var inputJSON, outputJSON []byte
	err := db.QueryRow(
		`INSERT INTO tool_executions (organization_id, agent_id, task_id, run_id, tool_id, tool_name, input, output, status, approval_id, idempotency_key, error) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id, organization_id, agent_id, task_id, run_id, tool_id, tool_name, input, output, status, approval_id, idempotency_key, error, created_at, updated_at`,
		exec.OrganizationID, exec.AgentID, exec.TaskID, exec.RunID, exec.ToolID, exec.ToolName, mustMarshal(exec.Input), mustMarshal(exec.Output), exec.Status, exec.ApprovalID, exec.IdempotencyKey, exec.Error,
	).Scan(&out.ID, &out.OrganizationID, &out.AgentID, &tid, &rid, &out.ToolID, &out.ToolName, &inputJSON, &outputJSON, &out.Status, &aid, &out.IdempotencyKey, &out.Error, &out.CreatedAt, &out.UpdatedAt)
	if err != nil { return nil, err }
	if len(inputJSON)>0 { _ = json.Unmarshal(inputJSON, &out.Input) }
	if len(outputJSON)>0 { _ = json.Unmarshal(outputJSON, &out.Output) }
	if tid.Valid { uid, _ := uuid.Parse(tid.String); out.TaskID = &uid }
	if rid.Valid { uid, _ := uuid.Parse(rid.String); out.RunID = &uid }
	if aid.Valid { uid, _ := uuid.Parse(aid.String); out.ApprovalID = &uid }
	return &out, nil
}

func (db *DB) GetToolExecutionByIdempotency(orgID uuid.UUID, key string) (*domain.ToolExecution, error) {
	var out domain.ToolExecution
	var tid, rid, aid sql.NullString
	var inputJSON, outputJSON []byte
	err := db.QueryRow(`SELECT id, organization_id, agent_id, task_id, run_id, tool_id, tool_name, input, output, status, approval_id, idempotency_key, error, created_at, updated_at FROM tool_executions WHERE organization_id=$1 AND idempotency_key=$2`, orgID, key).Scan(
		&out.ID, &out.OrganizationID, &out.AgentID, &tid, &rid, &out.ToolID, &out.ToolName, &inputJSON, &outputJSON, &out.Status, &aid, &out.IdempotencyKey, &out.Error, &out.CreatedAt, &out.UpdatedAt)
	if err != nil { return nil, err }
	if len(inputJSON)>0 { _ = json.Unmarshal(inputJSON, &out.Input) }
	if len(outputJSON)>0 { _ = json.Unmarshal(outputJSON, &out.Output) }
	if tid.Valid { uid, _ := uuid.Parse(tid.String); out.TaskID = &uid }
	if rid.Valid { uid, _ := uuid.Parse(rid.String); out.RunID = &uid }
	if aid.Valid { uid, _ := uuid.Parse(aid.String); out.ApprovalID = &uid }
	return &out, nil
}

func (db *DB) ListToolExecutions(orgID uuid.UUID, agentID *uuid.UUID, taskID *uuid.UUID, limit int) ([]*domain.ToolExecution, error) {
	if limit<=0 || limit>50 { limit=20 }
	query := `SELECT id, organization_id, agent_id, task_id, run_id, tool_id, tool_name, input, output, status, approval_id, idempotency_key, error, created_at, updated_at FROM tool_executions WHERE organization_id=$1`
	args := []any{orgID}
	idx:=2
	if agentID!=nil {
		query += ` AND agent_id=$`+itoa(idx)
		args=append(args, *agentID); idx++
	}
	if taskID!=nil {
		query += ` AND task_id=$`+itoa(idx)
		args=append(args, *taskID); idx++
	}
	query += ` ORDER BY created_at DESC LIMIT $`+itoa(idx)
	args=append(args, limit)
	rows, err := db.Query(query, args...)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.ToolExecution
	for rows.Next() {
		var e domain.ToolExecution
		var tid, rid, aid sql.NullString
		var inputJSON, outputJSON []byte
		if err:=rows.Scan(&e.ID,&e.OrganizationID,&e.AgentID,&tid,&rid,&e.ToolID,&e.ToolName,&inputJSON,&outputJSON,&e.Status,&aid,&e.IdempotencyKey,&e.Error,&e.CreatedAt,&e.UpdatedAt); err!=nil { return nil, err }
		if len(inputJSON)>0 { _ = json.Unmarshal(inputJSON, &e.Input) }
		if len(outputJSON)>0 { _ = json.Unmarshal(outputJSON, &e.Output) }
		if tid.Valid { uid,_:=uuid.Parse(tid.String); e.TaskID=&uid }
		if rid.Valid { uid,_:=uuid.Parse(rid.String); e.RunID=&uid }
		if aid.Valid { uid,_:=uuid.Parse(aid.String); e.ApprovalID=&uid }
		out=append(out, &e)
	}
	return out, nil
}

func (db *DB) GetToolExecution(id uuid.UUID) (*domain.ToolExecution, error) {
	var out domain.ToolExecution
	var tid, rid, aid sql.NullString
	var inputJSON, outputJSON []byte
	err := db.QueryRow(`SELECT id, organization_id, agent_id, task_id, run_id, tool_id, tool_name, input, output, status, approval_id, idempotency_key, error, created_at, updated_at FROM tool_executions WHERE id=$1`, id).Scan(
		&out.ID, &out.OrganizationID, &out.AgentID, &tid, &rid, &out.ToolID, &out.ToolName, &inputJSON, &outputJSON, &out.Status, &aid, &out.IdempotencyKey, &out.Error, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if len(inputJSON) > 0 { _ = json.Unmarshal(inputJSON, &out.Input) }
	if len(outputJSON) > 0 { _ = json.Unmarshal(outputJSON, &out.Output) }
	if tid.Valid { uid, _ := uuid.Parse(tid.String); out.TaskID = &uid }
	if rid.Valid { uid, _ := uuid.Parse(rid.String); out.RunID = &uid }
	if aid.Valid { uid, _ := uuid.Parse(aid.String); out.ApprovalID = &uid }
	return &out, nil
}

func (db *DB) UpdateToolExecutionStatus(id uuid.UUID, status string, output map[string]any, errMsg *string) error {
	outJSON,_:=json.Marshal(output)
	_, err := db.Exec(`UPDATE tool_executions SET status=$2, output=$3, error=$4, updated_at=now() WHERE id=$1`, id, status, outJSON, errMsg)
	return err
}

func mustMarshal(v any) []byte {
	if v==nil { return []byte(`{}`) }
	b,_:=json.Marshal(v)
	return b
}
