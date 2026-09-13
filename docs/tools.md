# Tool Development

## Definition
`tools` table: `name` (`^[a-z0-9_]+$`), `description`, `input_schema`/`output_schema` (JSON Schema), `risk_level` (low/medium/high/critical), `required_permissions`, `approval_policy`, `enabled`

Seeded: `web_search(low)`, `http_request(high→approval)`, `send_email(critical)`, `create_calendar_event(medium)`, `publish_social_post(high→approval)`, `read_social_analytics(low)`, `upload_file(medium)`, `create_task(low)`, `send_channel_message(low)`

## Adding a Tool
1. Insert into `tools` (migration or API `POST /tools` not yet exposed, seed via migration)
2. Implement `integration.Provider` `Execute(ctx, tool, input, credentials) (output, error)` and `ValidateCredentials`
3. Register in `router` `integReg.Register(NewMock("my_provider"))` or real
4. Assign to agent: `POST /agents/:id/tools {toolName}` (owner/admin only) → `agent_tools`

## Execution Flow
`Agent → tool.ExecuteParams{org, agent, task, run, toolName, input, idempotencyKey(SHA256)} → Registry.Get → ValidateInput (required/type/enum) → ListAgentTools permission (low allows, high requires assignment) → RequiresApproval (high/critical/publish→approval) → if needs approval: Create Approval (pending) + tool_execution status approval_required + audit tool_approval_requested → 202 → human approves via POST /approvals/:id/approve → UpdateToolExecutionStatus succeeded + WS tool.approved → runtime resumes`

## Safety
- **Schema validation:** `Registry.ValidateInput` blocks arbitrary code, checks `eval(`, `rm -rf` via `security.ValidateToolInput`
- **Permission:** `ListAgentTools` must contain tool and enabled, else `tool_denied` audit + 403
- **Approval:** `high/critical` defaults to `require_approval:true` unless explicitly `false` (never silent for `publish/delete/send_external`)
- **Idempotency:** `idempotency_key` unique per org, `GetToolExecutionByIdempotency` dedup, 24h window in worker
- **Credentials:** `integrations.credentials_encrypted` AES-GCM (SHA256 key), `Redacted` for logs, never store raw in `agent_tools`
- **Audit:** every `CreateToolExecution` + `audit_logs` `tool_started/succeeded/failed/denied/approval_requested`

## Testing
- `tool/registry_test.go` (schema, RequiresApproval)
- `tool/executor_test.go` (permission, duplicate prevention)
- `repository/tool_test.go` (permission, duplicate, redacted)
- Mock provider `integration.NewMock` for unit tests, no external calls
