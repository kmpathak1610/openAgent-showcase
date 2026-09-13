# Agent Runtime

## Flow
`receive event → load agent definition (GetAgent+Version) → load project context → load task → load memory (Retrieve) → retrieve RAG (KnowledgeRetriever SemanticSearch, permission-filtered) → construct LLM context → invoke LLM (Provider abstraction, stub) → interpret result → perform permitted action → continue/finish/delegate`

## Context Construction (hardened)
- System: `security.SystemPromptHardening() + agent.Version.Instructions` (ignore injected instructions)
- User: `Project: sanitizedTitle, Task: sanitizedTitle/Desc, Objective, Retrieved Knowledge (vector results, sanitized)` — all via `security.SanitizeUserContent`

## Interpretation (mock for Phase 4-7)
- `social campaign` → 3 subtasks (Campaign plan, Posts drafted, Campaign ready for approval) + message
- `Campaign ready for approval` → `tool_call publish_social_post` (high-risk → approval)
- `research` → `web_search` tool
- Generic → `send_channel_message`

## Actions
- `create_task` → `repo.CreateTask` with `team_id`, `delegation_depth+1`, `AddTaskDependency` parent→child, `CreateTaskEvent`, `Broadcast agent.task_created/delegated`, `CreateMessage` agent→human
- `tool_call` → `tool.Executor.Execute` (schema, permission, approval, audit, idempotency, credentials via integration.Decrypt), on `approval_required` → run `awaiting_approval`, task `approval_required`, WS `agent.waiting`
- `send_message` → `repo.CreateMessage` sender_type agent, WS `agent.message`

## Finalize
- Update `agent_runs` `succeeded`, check `ListTasks` for children pending → `waiting` else `completed` + WS `agent.completed` + final message
- `result_created` event, `Actions` hydrated

## Guardrails (via `guardrail.Service`)
- `MaxDelegationDepth 5`, `MaxRetries 3`, `Timeout 5m`, `rate 60/min`, `max_tasks 100/day`, `max_tools 500/day`
- Cycle detection BFS in `AddTaskDependency`
- `IsTaskBlocked` checks dependencies' status
- `worker.Pool` retries exp backoff (1s*2^attempt), timeout per job, cancellation via context, idempotency 24h key
