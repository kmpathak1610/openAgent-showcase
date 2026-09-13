# API Reference — OpenAgent

Base: `http://localhost:8080/api/v1` — All protected routes require `Authorization: Bearer <JWT>`.

## Auth
- `POST /auth/register {email, displayName, password, orgName?}` → `{user, organization, token}`
- `POST /auth/login {email, password}` → `{token, user, organization, organizations}`
- `GET  /auth/me` → `{user, organization, organizations, claims}`
- `POST /auth/switch {organizationId}` → `{token, organizationId, role}`
- `POST /auth/logout` → `{message}`

## Organizations
- `GET    /organizations` → `[]Organization`
- `POST   /organizations {name, slug?}` → `Organization`
- `GET    /organizations/:id`
- `PATCH  /organizations/:id {name}` (owner/admin)
- `GET    /organizations/:id/members` → `[]OrgMember`
- `POST   /organizations/:id/members {email, role}` (owner/admin)
- `DELETE /organizations/:id/members/:userId` (owner/admin)

## Projects
- `GET    /projects`
- `POST   /projects {name, description, objective, icon, slug?}`
- `GET    /projects/:id`
- `PATCH  /projects/:id {name, description, objective}`
- `POST   /projects/:id/archive`
- `GET    /projects/:id/members`, `POST`, `DELETE`

## Channels
- `GET    /channels?projectId`
- `POST   /channels {name, displayName, description, topic, channelType, projectId?}`
- `GET    /channels/:id`
- `PATCH  /channels/:id {displayName, description, topic}`
- `POST   /channels/:id/rename {name}`
- `POST   /channels/:id/archive`
- `GET    /channels/:id/members`, `POST`, `DELETE`

## Messages
- `GET    /messages?channelId&threadId&before&limit`
- `POST   /messages {channelId, body, threadId?}`
- `GET    /messages/:id`
- `PATCH  /messages/:id {body}` (author only)
- `DELETE /messages/:id` (author or admin)
- `GET    /messages/:id/replies`
- `POST   /messages/:id/reactions {emoji}`, `DELETE /messages/:id/reactions/:emoji`

## Agents
- `POST   /agents/builder/preview {description, responsibilities, informationSources, actions, autonomyPreference}`
- `POST   /agents {preview, intent}`
- `GET    /agents`, `GET /agents/:id`, `PUT/PATCH /agents/:id` (owner/admin for security fields)
- `GET    /agents/:id/versions`, `GET /agents/:id/versions/:version`
- `GET    /agents/:id/permissions`, `POST /agents/:id/permissions {resourceType, resourceId, permission}`
- `GET    /agents/:id/runs`, `GET /agents/:id/status`, `GET /agents/status`

## Teams
- `POST   /teams/builder/preview {outcome}`
- `POST   /teams {preview, projectId?}` (creates agents if missing)
- `GET    /teams`, `GET /teams/:id`, `GET /teams/:id/members`, `POST /teams/:id/members`, `DELETE /teams/:id/members/:agentId`
- `POST   /teams/:id/start {projectId, channelId?, title, description}` → task
- `GET    /teams/:id/dashboard` → `{team, members, activeTasks, waitingTasks, blockedTasks, completedTasks, agentsWorking, agentsWaiting, pendingApprovals, recentActivity}`
- `GET    /teams/:id/activity`

## Tasks
- `POST   /tasks {projectId, channelId?, parentTaskId?, teamId?, title, description, priority, assignedToAgent?, assignedToUser?, deadline?, correlationId?}`
- `GET    /tasks?projectId&channelId&status&assignedToAgent`
- `GET    /tasks/:id`, `PATCH/PUT /tasks/:id {status, payload}`, `POST /tasks/:id/assign {agentId,userId}`, `POST /tasks/:id/delegate {title, description, assignedToAgent, sourceAgent}`, `POST /tasks/:id/cancel`, `GET /tasks/:id/events`, `GET /tasks/:id/runs`
- `POST   /tasks/:id/dependencies {dependsOn}`, `GET /tasks/:id/dependencies`, `GET /tasks/:id/blocked`

## Runs
- `GET /runs?agentId&taskId`, `GET /runs/:id`, `GET /agents/:id/runs`

## Knowledge
- `POST /knowledge/sources {name, sourceType, config}`, `GET /knowledge/sources`
- `POST /knowledge/collections {name, description}`, `GET /knowledge/collections`
- `POST /knowledge/documents` (JSON or multipart: title, sourceType, content/sourceUrl/file, scope, projectId, collectionId)
- `GET /knowledge/documents?projectId&scope&status&limit`, `GET /knowledge/documents/:id`, `DELETE /knowledge/documents/:id`, `GET /knowledge/documents/:id/chunks`
- `POST /knowledge/search {query, limit, projectId, agentId, scope, metadata, hybrid}`
- `POST /agents/:id/knowledge {documentId, collectionId}`, `GET /agents/:id/knowledge`, `DELETE /agents/:id/knowledge/:knowledgeId`
- `POST /projects/:id/knowledge`, `GET /projects/:id/knowledge`, `DELETE /projects/:id/knowledge/:knowledgeId`

## Memories
- `POST /memories {agentId?, projectId?, memoryType, scope, source, content, importance, metadata}`, `GET /memories?agentId&projectId&scope&memoryType`, `POST /memories/search {query, agentId, projectId, limit}`, `DELETE /memories/:id`

## Triggers / Schedules
- `POST /triggers {name, triggerType, config{scheduledAt/cron/timezone/title}, agentId, teamId, projectId}`
- `GET /triggers`, `GET /triggers/:id`, `DELETE /triggers/:id`, `GET /schedules`

## Tools / Integrations / Approvals
- `GET /tools`, `GET /tools/:name`, `POST /tools/:name/execute {input, agentId, taskId, runId, idempotencyKey}`, `GET /tool-executions?agentId&taskId`
- `POST /agents/:id/tools {toolName}`, `GET /integrations`, `POST /integrations {name, provider, config, credentials}`, `GET /integrations/:id`, `DELETE /integrations/:id`
- `GET /approvals?status`, `GET /approvals/:id`, `POST /approvals/:id/approve`, `POST /approvals/:id/reject {reason}`, `POST /approvals/:id/cancel`

## WebSocket
- `GET /ws?token=<JWT>` — auth via query or `Authorization: Bearer`
- Server joins `org:<id>` room; client can send `{"type":"join_channel","payload":{"channelId":"..."}}` (verified), `ping` → `pong`
- Server broadcasts: `message.created/updated/deleted`, `reaction.added/removed`, `agent.started/thinking/tool_started/tool_completed/message/task_created/delegated/task_assigned/waiting/completed/failed`, `agent.task_updated`, `team.task_started/escalated`, `tool.approved`, `approval.decided`, `autonomous.task_created`

## Health / Metrics
- `GET /health` → `{"status":"ok"}`
- `GET /ready` → checks DB
- `GET /metrics` → Prometheus text (requests, runs, tasks, tools, WS, latency)
- Rate limited: 10 req/s burst 20 per IP/org/path → 429 `RATE_LIMITED`

## Errors
```json
{"error":{"code":"VALIDATION_ERROR","message":"...","details":{}}}
```
Codes: `VALIDATION_ERROR, UNAUTHORIZED, FORBIDDEN, NOT_FOUND, RATE_LIMITED, INTERNAL`

## Articles (team pipeline, v1 reuses Tasks)

Brief → parent Task `title=Article: <topic>`, `description=topic=..; length=short|long|flexible; tone=..; format=blog|seo|docs`.

1. `POST /tasks` with `{projectId, title: "Article: Vector DBs", description: "topic=Vector DBs; length=short; tone=friendly; format=blog"}`
2. Upload inputs: `POST /knowledge/documents` with `{title, sourceType: "upload|website|text", projectId, content|sourceUrl}`
3. `POST /teams/:id/start` with `{projectId, title, description}` (optional `channelId`)
4. Poll `GET /tasks/:id/events` + `GET /tasks/:id/runs` + WS `agent.task_updated`
5. Draft stored as `documents` (v1 uses `sourceType` `upload`/`text`; intended `generated`), final requires `approvals` before `publish_social_post`.

## Settings LLM (read-only v1)

`GET /settings/llm` returns `{defaultModel, availableModels, providers: [{name, real}]}`. Keys never exposed. Configure via `.env` (`OPENROUTER_API_KEY`, `OPENROUTER_MODEL`) or compose `environment`. Frontend Settings page shows badges; Builder/Detail dropdowns consume `availableModels` with minimax-first fallback.

## Home channel (auto-reply)

Signup creates org `general` home channel. Post user messages there to trigger smart-routed agent replies inline; agent-to-agent collaboration appears in threads. Mention `@agent-slug` to force routing, else router picks (assistant fallback). Agent messages never retrigger.
