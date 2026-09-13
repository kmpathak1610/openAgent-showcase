package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"openagent/internal/domain"
	"openagent/internal/llm"
)

func (db *DB) CreateAgentRun(orgID, agentID uuid.UUID, taskID, channelID *uuid.UUID, triggerType string, input, contextData map[string]any, correlationID *uuid.UUID, parentRunID *uuid.UUID) (*domain.AgentRun, error) {
	inputJSON, _ := json.Marshal(input)
	contextJSON, _ := json.Marshal(contextData)
	if contextJSON == nil { contextJSON = []byte(`{}`) }
	var corr sql.NullString
	if correlationID != nil { corr = sql.NullString{String: correlationID.String(), Valid: true} }
	var parent sql.NullString
	if parentRunID != nil { parent = sql.NullString{String: parentRunID.String(), Valid: true} }
	var agentVersion sql.NullInt32
	// get current version
	var cv int
	_ = db.QueryRow(`SELECT current_version FROM agents WHERE id=$1`, agentID).Scan(&cv)
	if cv >0 { agentVersion = sql.NullInt32{Int32: int32(cv), Valid: true} }
	var run domain.AgentRun
	var tokenMeta, retrieved, ctxJSON, inputJ, outputJ []byte
	var errStr sql.NullString
	var started, finished sql.NullTime
	err := db.QueryRow(
		`INSERT INTO agent_runs (organization_id, agent_id, task_id, channel_id, agent_version, trigger_type, status, current_iteration, input, context, retrieved_knowledge, token_metadata, correlation_id, parent_run_id)
		 VALUES ($1,$2,$3,$4,$5,$6,'queued',0,$7,$8,'[]','{}',$9,$10) RETURNING id, organization_id, agent_id, task_id, channel_id, agent_version, trigger_type, status, current_iteration, waiting_reason, pending_action_id, input, context, retrieved_knowledge, output, token_metadata, correlation_id, parent_run_id, error, started_at, finished_at, created_at, updated_at`,
		orgID, agentID, taskID, channelID, agentVersion, triggerType, inputJSON, contextJSON, corr, parent,
	).Scan(&run.ID, &run.OrganizationID, &run.AgentID, &run.TaskID, &run.ChannelID, &run.AgentVersion, &run.TriggerType, &run.Status, &run.CurrentIteration, &run.WaitingReason, &run.PendingActionID, &inputJ, &ctxJSON, &retrieved, &outputJ, &tokenMeta, &corr, &parent, &errStr, &started, &finished, &run.CreatedAt, &run.UpdatedAt)
	if err != nil { return nil, err }
	if len(inputJ)>0 { _ = json.Unmarshal(inputJ, &run.Input) }
	if len(ctxJSON)>0 { _ = json.Unmarshal(ctxJSON, &run.Context) }
	if len(retrieved)>0 { _ = json.Unmarshal(retrieved, &run.RetrievedKnowledge) }
	if len(outputJ)>0 { _ = json.Unmarshal(outputJ, &run.Output) }
	if len(tokenMeta)>0 { _ = json.Unmarshal(tokenMeta, &run.TokenMetadata) }
	if corr.Valid { uid, _ := uuid.Parse(corr.String); run.CorrelationID = &uid }
	if parent.Valid { uid, _ := uuid.Parse(parent.String); run.ParentRunID = &uid }
	if errStr.Valid { run.Error = &errStr.String }
	if started.Valid { run.StartedAt = &started.Time }
	if finished.Valid { run.FinishedAt = &finished.Time }
	return &run, nil
}

func (db *DB) GetAgentRun(orgID, runID uuid.UUID) (*domain.AgentRun, error) {
	var run domain.AgentRun
	var inputJ, ctxJ, retrieved, outputJ, tokenMeta []byte
	var corr, parent sql.NullString
	var errStr sql.NullString
	var started, finished sql.NullTime
	var agentVersion sql.NullInt32
	// Support both old and new columns via COALESCE
	err := db.QueryRow(
		`SELECT id, organization_id, agent_id, task_id, channel_id, agent_version, trigger_type, status, COALESCE(current_iteration,0), waiting_reason, pending_action_id, input, context, retrieved_knowledge, output, token_metadata, correlation_id, parent_run_id, error, started_at, finished_at, created_at, updated_at FROM agent_runs WHERE id=$1 AND organization_id=$2`,
		runID, orgID,
	).Scan(&run.ID, &run.OrganizationID, &run.AgentID, &run.TaskID, &run.ChannelID, &agentVersion, &run.TriggerType, &run.Status, &run.CurrentIteration, &run.WaitingReason, &run.PendingActionID, &inputJ, &ctxJ, &retrieved, &outputJ, &tokenMeta, &corr, &parent, &errStr, &started, &finished, &run.CreatedAt, &run.UpdatedAt)
	if err != nil { return nil, err }
	if len(inputJ)>0 { _ = json.Unmarshal(inputJ, &run.Input) }
	if len(ctxJ)>0 { _ = json.Unmarshal(ctxJ, &run.Context) }
	if len(retrieved)>0 { _ = json.Unmarshal(retrieved, &run.RetrievedKnowledge) }
	if len(outputJ)>0 { _ = json.Unmarshal(outputJ, &run.Output) }
	if len(tokenMeta)>0 { _ = json.Unmarshal(tokenMeta, &run.TokenMetadata) }
	if agentVersion.Valid { v := int(agentVersion.Int32); run.AgentVersion = &v }
	if corr.Valid { uid, _ := uuid.Parse(corr.String); run.CorrelationID = &uid }
	if parent.Valid { uid, _ := uuid.Parse(parent.String); run.ParentRunID = &uid }
	if errStr.Valid { run.Error = &errStr.String }
	if started.Valid { run.StartedAt = &started.Time }
	if finished.Valid { run.FinishedAt = &finished.Time }
	// hydrate actions
	actions, _ := db.ListAgentActions(run.ID)
	run.Actions = actions
	return &run, nil
}

func (db *DB) ListAgentRuns(orgID uuid.UUID, agentID *uuid.UUID, taskID *uuid.UUID, limit int) ([]*domain.AgentRun, error) {
	if limit <=0 || limit>50 { limit = 20 }
	query := `SELECT id, organization_id, agent_id, task_id, channel_id, agent_version, trigger_type, status, COALESCE(current_iteration,0), waiting_reason, pending_action_id, input, context, retrieved_knowledge, output, token_metadata, correlation_id, parent_run_id, error, started_at, finished_at, created_at, updated_at FROM agent_runs WHERE organization_id=$1`
	args := []any{orgID}
	idx := 2
	if agentID != nil {
		query += ` AND agent_id=$` + itoa(idx)
		args = append(args, *agentID)
		idx++
	}
	if taskID != nil {
		query += ` AND task_id=$` + itoa(idx)
		args = append(args, *taskID)
		idx++
	}
	query += ` ORDER BY created_at DESC LIMIT $` + itoa(idx)
	args = append(args, limit)
	rows, err := db.Query(query, args...)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.AgentRun
	for rows.Next() {
		var run domain.AgentRun
		var inputJ, ctxJ, retrieved, outputJ, tokenMeta []byte
		var corr, parent sql.NullString
		var errStr sql.NullString
		var started, finished sql.NullTime
		var agentVersion sql.NullInt32
		if err := rows.Scan(&run.ID, &run.OrganizationID, &run.AgentID, &run.TaskID, &run.ChannelID, &agentVersion, &run.TriggerType, &run.Status, &run.CurrentIteration, &run.WaitingReason, &run.PendingActionID, &inputJ, &ctxJ, &retrieved, &outputJ, &tokenMeta, &corr, &parent, &errStr, &started, &finished, &run.CreatedAt, &run.UpdatedAt); err != nil { return nil, err }
		if len(inputJ)>0 { _ = json.Unmarshal(inputJ, &run.Input) }
		if len(ctxJ)>0 { _ = json.Unmarshal(ctxJ, &run.Context) }
		if len(retrieved)>0 { _ = json.Unmarshal(retrieved, &run.RetrievedKnowledge) }
		if len(outputJ)>0 { _ = json.Unmarshal(outputJ, &run.Output) }
		if len(tokenMeta)>0 { _ = json.Unmarshal(tokenMeta, &run.TokenMetadata) }
		if agentVersion.Valid { v := int(agentVersion.Int32); run.AgentVersion = &v }
		if corr.Valid { uid, _ := uuid.Parse(corr.String); run.CorrelationID = &uid }
		if parent.Valid { uid, _ := uuid.Parse(parent.String); run.ParentRunID = &uid }
		if errStr.Valid { run.Error = &errStr.String }
		if started.Valid { run.StartedAt = &started.Time }
		if finished.Valid { run.FinishedAt = &finished.Time }
		out = append(out, &run)
	}
	return out, nil
}


func (db *DB) UpdateAgentRunWaiting(orgID, runID uuid.UUID, status string, iteration int, waitingReason *string, pendingActionID *uuid.UUID, output map[string]any) error {
	var current string
	_ = db.QueryRow(`SELECT status FROM agent_runs WHERE id=$1 AND organization_id=$2`, runID, orgID).Scan(&current)
	outputJSON, _ := json.Marshal(output)
	_, err := db.Exec(`UPDATE agent_runs SET status=$3, current_iteration=$4, waiting_reason=$5, pending_action_id=$6, output=$7, updated_at=now() WHERE id=$1 AND organization_id=$2`, runID, orgID, status, iteration, waitingReason, pendingActionID, outputJSON)
	return err
}

func (db *DB) GetWaitingRunByCorrelation(orgID uuid.UUID, correlationID uuid.UUID) (*domain.AgentRun, error) {
	var run domain.AgentRun
	var inputJ, ctxJ, retrieved, outputJ, tokenMeta []byte
	var corr, parent sql.NullString
	var errStr sql.NullString
	var started, finished sql.NullTime
	var agentVersion sql.NullInt32
	err := db.QueryRow(`SELECT id, organization_id, agent_id, task_id, channel_id, agent_version, trigger_type, status, COALESCE(current_iteration,0), waiting_reason, pending_action_id, input, context, retrieved_knowledge, output, token_metadata, correlation_id, parent_run_id, error, started_at, finished_at, created_at, updated_at FROM agent_runs WHERE organization_id=$1 AND correlation_id=$2 AND status IN ('awaiting_approval','WAITING_FOR_APPROVAL','waiting') ORDER BY created_at DESC LIMIT 1`, orgID, correlationID).Scan(&run.ID, &run.OrganizationID, &run.AgentID, &run.TaskID, &run.ChannelID, &agentVersion, &run.TriggerType, &run.Status, &run.CurrentIteration, &run.WaitingReason, &run.PendingActionID, &inputJ, &ctxJ, &retrieved, &outputJ, &tokenMeta, &corr, &parent, &errStr, &started, &finished, &run.CreatedAt, &run.UpdatedAt)
	if err != nil { return nil, err }
	if len(inputJ)>0 { _ = json.Unmarshal(inputJ, &run.Input) }
	if len(ctxJ)>0 { _ = json.Unmarshal(ctxJ, &run.Context) }
	if len(retrieved)>0 { _ = json.Unmarshal(retrieved, &run.RetrievedKnowledge) }
	if len(outputJ)>0 { _ = json.Unmarshal(outputJ, &run.Output) }
	if len(tokenMeta)>0 { _ = json.Unmarshal(tokenMeta, &run.TokenMetadata) }
	if agentVersion.Valid { v := int(agentVersion.Int32); run.AgentVersion = &v }
	if corr.Valid { uid, _ := uuid.Parse(corr.String); run.CorrelationID = &uid }
	if parent.Valid { uid, _ := uuid.Parse(parent.String); run.ParentRunID = &uid }
	if errStr.Valid { run.Error = &errStr.String }
	if started.Valid { run.StartedAt = &started.Time }
	if finished.Valid { run.FinishedAt = &finished.Time }
	return &run, nil
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}

var validRunTransitions = map[string][]string{
	"queued": {"running", "cancelled"},
	"running": {"awaiting_approval", "succeeded", "failed", "cancelled", "waiting", "blocked"},
	"awaiting_approval": {"running", "failed", "cancelled"},
	"waiting": {"running", "succeeded", "failed", "cancelled"},
	"blocked": {"running", "cancelled"},
	"succeeded": {},
	"failed": {"queued", "running"},
	"cancelled": {},
	"pending": {"running", "cancelled"},
	"assigned": {"running", "cancelled"},
	"completed": {},
}

func isValidRunTransition(from, to string) bool {
	allowed, ok := validRunTransitions[from]
	if !ok { return false }
	for _, a := range allowed {
		if a == to { return true }
	}
	return from == to // allow same status (idempotent)
}

func (db *DB) UpdateAgentRunStatus(orgID, runID uuid.UUID, status string, output map[string]any, errMsg *string) error {
	// Validate transition unless status is same or we allow retry from failed
	var current string
	_ = db.QueryRow(`SELECT status FROM agent_runs WHERE id=$1 AND organization_id=$2`, runID, orgID).Scan(&current)
	if current != "" && !isValidRunTransition(current, status) {
		// For strict consistency, reject invalid transitions like completed->running without explicit retry
		// Allow if current is failed and new is running (retry) is already in map
		if !(current == "succeeded" && status == "running") {
			// Log but still allow for backward compat? We enforce for terminal states
			if current == "succeeded" || current == "completed" || current == "cancelled" {
				return fmt.Errorf("invalid run transition %s -> %s", current, status)
			}
		}
	}
	outputJSON, _ := json.Marshal(output)
	_, err := db.Exec(`UPDATE agent_runs SET status=$3, output=$4, error=$5, updated_at=now(), started_at=COALESCE(started_at, CASE WHEN $3='running' THEN now() ELSE started_at END), finished_at=CASE WHEN $3 IN ('succeeded','failed','cancelled','completed') THEN now() ELSE finished_at END WHERE id=$1 AND organization_id=$2`, runID, orgID, status, outputJSON, errMsg)
	return err
}

func (db *DB) UpdateAgentRunTokens(orgID, runID uuid.UUID, usage llm.Usage) error {
	// Accumulate token usage into token_metadata jsonb
	_, err := db.Exec(`UPDATE agent_runs SET token_metadata = jsonb_set(jsonb_set(COALESCE(token_metadata,'{}'::jsonb), '{promptTokens}', to_jsonb(COALESCE((token_metadata->>'promptTokens')::int,0) + $3)), '{completionTokens}', to_jsonb(COALESCE((token_metadata->>'completionTokens')::int,0) + $4)) || jsonb_build_object('totalTokens', COALESCE((token_metadata->>'promptTokens')::int,0) + $3 + COALESCE((token_metadata->>'completionTokens')::int,0) + $4, 'estimatedCost', (COALESCE((token_metadata->>'promptTokens')::int,0) + $3)*0.00001 + (COALESCE((token_metadata->>'completionTokens')::int,0) + $4)*0.00003) WHERE id=$1 AND organization_id=$2`, runID, orgID, usage.PromptTokens, usage.CompletionTokens)
	return err
}

func (db *DB) SetAgentRunTokens(orgID, runID uuid.UUID, usage llm.Usage) error {
	meta := map[string]any{"promptTokens": usage.PromptTokens, "completionTokens": usage.CompletionTokens, "totalTokens": usage.TotalTokens, "estimatedCost": float64(usage.PromptTokens)*0.00001 + float64(usage.CompletionTokens)*0.00003}
	metaJSON, _ := json.Marshal(meta)
	_, err := db.Exec(`UPDATE agent_runs SET token_metadata=$3 WHERE id=$1 AND organization_id=$2`, runID, orgID, metaJSON)
	return err
}

func (db *DB) CreateAgentAction(runID uuid.UUID, seq int, actionType string, toolName *string, input, output map[string]any, status string, correlationID *uuid.UUID) (*domain.AgentAction, error) {
	inputJSON, _ := json.Marshal(input)
	outputJSON, _ := json.Marshal(output)
	var corr sql.NullString
	if correlationID != nil { corr = sql.NullString{String: correlationID.String(), Valid: true} }
	var act domain.AgentAction
	var outJSON []byte
	err := db.QueryRow(
		`INSERT INTO agent_actions (run_id, seq, action_type, tool_name, input, output, status, correlation_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id, run_id, seq, action_type, tool_name, input, output, status, correlation_id, created_at`,
		runID, seq, actionType, toolName, inputJSON, outputJSON, status, corr,
	).Scan(&act.ID, &act.RunID, &act.Seq, &act.ActionType, &act.ToolName, &inputJSON, &outJSON, &act.Status, &corr, &act.CreatedAt)
	if err != nil { return nil, err }
	if len(inputJSON)>0 { _ = json.Unmarshal(inputJSON, &act.Input) }
	if len(outJSON)>0 { _ = json.Unmarshal(outJSON, &act.Output) }
	if corr.Valid { uid, _ := uuid.Parse(corr.String); act.CorrelationID = &uid }
	return &act, nil
}

func (db *DB) ListAgentActions(runID uuid.UUID) ([]domain.AgentAction, error) {
	rows, err := db.Query(`SELECT id, run_id, seq, action_type, tool_name, input, output, status, correlation_id, created_at FROM agent_actions WHERE run_id=$1 ORDER BY seq`, runID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []domain.AgentAction
	for rows.Next() {
		var act domain.AgentAction
		var inputJ, outputJ []byte
		var corr sql.NullString
		if err := rows.Scan(&act.ID, &act.RunID, &act.Seq, &act.ActionType, &act.ToolName, &inputJ, &outputJ, &act.Status, &corr, &act.CreatedAt); err != nil { return nil, err }
		if len(inputJ)>0 { _ = json.Unmarshal(inputJ, &act.Input) }
		if len(outputJ)>0 { _ = json.Unmarshal(outputJ, &act.Output) }
		if corr.Valid { uid, _ := uuid.Parse(corr.String); act.CorrelationID = &uid }
		out = append(out, act)
	}
	return out, nil
}
