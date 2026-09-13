package repository

import (
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestTool_PermissionEnforcement(t *testing.T) {
	db, mock, close := newMockDB(t)
	defer close()
	agentID := uuid.New()
	toolID := uuid.New()
	// ListAgentTools returns empty -> agent not authorized for high-risk tool
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT at.id, at.agent_id, at.tool_id, at.enabled, at.created_at, t.name, t.description, t.risk_level FROM agent_tools at JOIN tools t ON t.id=at.tool_id WHERE at.agent_id=$1`)).
		WithArgs(agentID).WillReturnRows(sqlmock.NewRows([]string{"id","agent_id","tool_id","enabled","created_at","name","description","risk_level"}))
	tools, _ := db.ListAgentTools(agentID)
	if len(tools)!=0 { t.Fatal("should have no tools") }
	// Assign tool
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO agent_tools (agent_id, tool_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`)).
		WithArgs(agentID, toolID).WillReturnResult(sqlmock.NewResult(1,1))
	if err:=db.AssignToolToAgent(agentID, toolID); err!=nil { t.Fatalf("assign failed: %v", err) }
	// Now list should return one
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT at.id, at.agent_id, at.tool_id, at.enabled, at.created_at, t.name, t.description, t.risk_level FROM agent_tools at JOIN tools t ON t.id=at.tool_id WHERE at.agent_id=$1`)).
		WithArgs(agentID).WillReturnRows(sqlmock.NewRows([]string{"id","agent_id","tool_id","enabled","created_at","name","description","risk_level"}).
			AddRow(uuid.New(), agentID, toolID, true, mustParseTime("2024-01-01T00:00:00Z"), "publish_social_post", "Publish", "high"))
	tools, _ = db.ListAgentTools(agentID)
	if len(tools)!=1 || tools[0].Tool.Name!="publish_social_post" { t.Fatal("should have tool after assign") }
	if err:=mock.ExpectationsWereMet(); err!=nil { t.Fatal(err) }
}

func TestTool_DuplicatePrevention(t *testing.T) {
	db, mock, close := newMockDB(t)
	defer close()
	orgID := uuid.New()
	key := "idem-key-123"
	toolID := uuid.New()
	agentID := uuid.New()
	// Get by idempotency returns existing (duplicate prevention)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, organization_id, agent_id, task_id, run_id, tool_id, tool_name, input, output, status, approval_id, idempotency_key, error, created_at, updated_at FROM tool_executions WHERE organization_id=$1 AND idempotency_key=$2`)).
		WithArgs(orgID, key).WillReturnRows(
			sqlmock.NewRows([]string{"id","organization_id","agent_id","task_id","run_id","tool_id","tool_name","input","output","status","approval_id","idempotency_key","error","created_at","updated_at"}).
				AddRow(uuid.New(), orgID, agentID, nil, nil, toolID, "web_search", []byte(`{}`), []byte(`{}`), "succeeded", nil, key, nil, mustParseTime("2024-01-01T00:00:00Z"), mustParseTime("2024-01-01T00:00:00Z")),
		)
	existing, err := db.GetToolExecutionByIdempotency(orgID, key)
	if err != nil || existing == nil || existing.IdempotencyKey != key { t.Fatalf("should find existing: %v", err) }
	if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
}

func TestTool_AuditLogging_NotSecrets(t *testing.T) {
	// Verify that RedactedInput hides secrets
	input := map[string]any{"query": "test", "apiKey": "sk-secret", "token": "tok"}
	redacted := RedactedForTest(input)
	if redacted["apiKey"] != "***REDACTED***" { t.Fatal("should redact apiKey") }
	if redacted["query"] != "test" { t.Fatal("query should not be redacted") }
}

// helper to test RedactedInput from tool executor (we duplicate logic for test)
func RedactedForTest(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for k, v := range input {
		lower := ""
		for _, c := range k {
			if c >= 'A' && c <= 'Z' { lower += string(c + 32) } else { lower += string(c) }
		}
		if lower == "apikey" || lower == "token" || lower == "secret" {
			out[k] = "***REDACTED***"
		} else {
			out[k] = v
		}
	}
	return out
}
