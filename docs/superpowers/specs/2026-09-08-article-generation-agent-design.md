# Article Generation Agent — Design Spec

Date: 2026-09-08
Status: Approved (architecture, components, flow)
Project: D:\ai_projects\openagent

## 1. Context

OpenAgent is a human + AI collaborative workspace (Angular 21 frontend, Go Chi backend, PostgreSQL 16 + pgvector, Redis, WebSocket).

Existing primitives reused:
- `agents` + `agent_versions` (role, objective, instructions, capabilities, tool/memory/approval policies, modelConfiguration) — `backend/internal/domain/models.go`, `backend/internal/builder/builder.go`, `backend/internal/repository/agent.go`
- `runtime.ExecuteTask` loop (Task → AgentRun → LLM → Decision → tools → finalize) — `backend/internal/runtime/`
- `worker.Pool` jobs `agent_run`, `ingest_document` — `backend/internal/worker/`, wired in `backend/internal/http/router.go`
- `knowledge` RAG (ingest → chunk 500tok/50 overlap → embed 1536 → HNSW, Semantic/Hybrid retrieval with permission filter) — `backend/internal/knowledge/`
- `tools` (`web_search`, `browser.*`, `send_channel_message`, `publish_social_post`, etc.) — `migrations/000007_*`, `000015_*`, `backend/internal/tool/`
- `teams` orchestrator (coordinator-first, delegation depth/cycle checks) — `backend/internal/team/`
- `tasks` (parent/subtasks, correlationID, events, runs), `documents` (`sourceType=generated` for drafts), `approvals`, `memories` (brand voice), `ws.Hub` live updates
- Frontend patterns: `core/api.service.ts`, `features/tasks/tasks.component.ts` (inline create), `features/knowledge/knowledge.component.ts` (tabs), `features/agents/builder/builder.component.ts` (wizard)

No article-specific tables, tools, or endpoints exist today.

## 2. Requirements (validated)

- Input: research-grounded. Brief + per-article inputs (URLs, uploads, pasted text) + internal knowledge base + live web + brand memory.
- Sources (all four): internal knowledge base (collections/documents via `RetrievalFilter`), web search (`web_search` + `browser.search/navigate/extract`), per-article inputs (ingested as `documents` scoped to article task/project), brand + memory (`memories` project/agent scope, style-guide collections).
- Workflow: multi-agent team (researcher → writer → editor) via existing Team orchestrator with coordinator.
- Output: flexible per article. Brief params choose length/format each run (short blog 300-800 words, long-form SEO with headings/citations/meta, docs). Draft stored as `documents` version, human reviews in UI before publish.
- Success: `POST brief → team run → grounded draft with citations → editor pass → approval → 201 publish`, WS progress visible, token/RAG auditable via `agent_runs`.

Out of scope (YAGNI): new core execution engine, new vector store, scheduled series automation v1 (triggers reusable later), CMS publish integrations beyond existing `publish_social_post`, custom markdown editor (reuse message/document preview).

## 3. Architecture

Article Team is a standard `agent_teams` entry with 3 members + coordinator, driven by a parent `tasks` row holding the brief JSON in `description` + `metadata`.

- Parent Task (project-scoped, `type=article` in metadata): `{topic, length, tone, format, sourceUrls, documentIds, knowledgeCollectionIds, brandMemoryScope}`
- Subtasks: `research`, `outline` (part of research output), `draft`, `edit`, linked via `parent_id` + `correlation_id`
- Storage: research bundle + draft versions as `documents` (`sourceType=generated`, `project_id` set, `version` bumped per revision), chunks embedded for citations
- Grounding: `KnowledgeRetriever.HybridSearch` (internal + brand) merged with `web_search`/`browser.extract` results, top-3 RAG + top-3 memories injected via `buildAgentContext` (8k cap, sanitized)
- Human gate: `approvals` (`entity_type=tool_execution` or `task`, `require_approval_for=[publish_content]`) before publish; frontend Articles detail shows preview, versions, runs, approval badge
- Realtime: existing `ws.Hub` events (`agent.started/thinking/tool_completed/completed`, `task.updated`) subscribed by Articles UI

No new core tables. Optional migration only if new low-risk tools needed (e.g., `generate_article_outline`); otherwise reuse seeded tools.

## 4. Components

### 4.1 Agents (seeded via `builder.GeneratePreview`)

- Researcher (`capabilities=[research]`, `autonomy=collaborative`): allowed tools `web_search`, `browser.search`, `browser.navigate`, `browser.extract`, `knowledge.search`. Instructions: collect 5-10 sources, extract quotes, emit outline + citation list as JSON. Low-risk only.
- Writer (`capabilities=[content_creation]`, `autonomy=collaborative`): allowed tools `create_task`, `send_channel_message`, `knowledge.search`. Instructions: expand outline to draft honoring `length/tone/format` brief params, inline `[source N]` citations, SEO meta block when `format=seo`. Writes `documents` draft via `send_message` + `complete_task` with markdown body.
- Editor (`capabilities=[content_creation, performance_analysis]`, `autonomy=collaborative`): allowed tools `knowledge.search`. Instructions: fix style/grammar/SEO, verify citations map to retrieved chunks, flag unsupported claims, produce final markdown + change summary. Triggers `request_approval` for publish.
- Coordinator: existing `team.Orchestrator` (no custom code). Team workflow: `coordinator → researcher → writer → editor → human`.

Permissions follow least-privilege (`organization:read`, `tool:execute` scoped), `applySafeguards` downgrades `autonomous → collaborative`.

### 4.2 Backend (reuse + thin facade)

Reuse: `AgentHandler`, `TeamHandler.StartTeamTask`, `TaskHandler.Create/Assign`, `KnowledgeHandler.CreateDocument/Search`, `RunHandler`, `ApprovalHandler`, `Document` repository.

New only if needed for UX: `POST /api/v1/articles` facade that creates parent Task + ingests per-article inputs + calls `orchestrator.StartTeamTask`, and `GET /api/v1/articles/:id` that aggregates Task + Documents versions + Runs + Approvals. Alternative v1 without new endpoints: drive entirely via `/tasks` + `/knowledge/documents` + `/teams/:id/start` from frontend service.

### 4.3 Frontend (`features/articles/`)

- `core/article.service.ts` mirroring `task.service.ts` signals (`articles`, `selected`, `loading`): `list/create/get/publish`, delegating to `/tasks` + `/knowledge/documents` + `/teams` or new `/articles` facade.
- `articles.component.ts`: copy `tasks.component.ts` inline create (topic, length select, tone select, format select, project/agent selects, URLs textarea, file picker via `knowledge.service.createDocument`, knowledge-collection multi-select) + `knowledge.component.ts` tabs (`prompt|upload|url`).
- `article-detail.component.ts`: copy `task-detail` (hero badges, brief, research sources with scores, draft preview markdown, versions, runs with tokenMetadata, approvals, WS live events).
- Routes in `app.routes.ts`: `articles`, `articles/:id` (ordered before `:id` catch-alls); sidebar link in `shell.component.ts` reusing `.card/.badge/.tabs` styles.

## 5. Data Flow

1. User submits brief in Articles UI (topic, length, tone, format, project, collections, URLs/files).
2. Frontend creates parent Task (`description`=brief JSON) + uploads inputs via `POST /knowledge/documents` (scoped to project, linked via `project_knowledge`), then `POST /teams/:id/start` with `correlation_id`.
3. Coordinator enqueues `agent_run` for researcher. Researcher calls `knowledge.HybridSearch` + `web_search`/`browser.extract`, stores bundle as `documents` v1, completes research subtask with outline JSON.
4. Writer run loads bundle + brand memories (`memory.Retrieve` project scope) + brief params, calls LLM `Complete`, emits markdown draft → `documents` v2, completes draft subtask.
5. Editor run loads draft + sources, revises, emits final markdown → `documents` v3 + `request_approval` (publish).
6. UI polls/subscribes WS, shows preview + diff summary + citations. Human edits (creates `document_versions` entry) and approves.
7. On approve, `publish_social_post` or `send_channel_message` executes (high-risk path gated), parent Task `completed`, `task_events` + `agent_runs` retain prompts, RAG hits, token usage.

## 6. Error Handling

- Empty RAG: fall back to web results; if both empty, researcher returns `waiting` with `needs_human` (request more inputs) instead of hallucinating.
- LLM JSON parse fail: existing `retryDecision` + fence-tolerant `extractJSON`; after retries, run `failed` with error preserved, parent stays `running` for retry.
- Tool fail/rate-limit: `tool_executions` row `failed`, `AgentAction` logged, orchestrator `Evaluate` retries or escalates per `execution_budgets` (100 tasks/day, 500 tools/day, depth 5).
- Publish without approval: blocked by `validateDecision` (`publish_content` requires approval); `approvalSvc` must be `approved`.
- Sanitization: all user/URL content via `security.SanitizeUserContent`, `SystemPromptHardening`; file ingest caps (10s timeout, 2MB web, 50k truncate).

## 7. Testing

- `builder` preview test: brief → preview contains researcher/writer/editor capabilities, low-risk tools only, collaborative autonomy.
- `team` orchestration test: mocked `Provider` returns canned research/draft/edit decisions; assert subtask order, document versions v1-v3, approval created.
- RAG grounding test: seeded docs + stub embedder; assert citations resolve to chunk IDs, unsupported claim flagged by editor.
- Handler test: article facade (if built) creates Task + Documents + team run; auth isolation (`organization_id` enforced).
- Frontend: service signal updates, detail renders versions/runs/approvals (manual + existing lint `npm run lint`).

## 8. Rollout

1. Seed 3 agents + team via builder preview (no migration).
2. Articles UI v1 driving existing `/tasks`, `/knowledge`, `/teams` endpoints (no backend change).
3. Add `/articles` facade + optional `generate_article_outline` tool migration only after v1 validated.
4. Docs update (`docs/api.md`) + e2e brief → publish in Docker (`docker compose up` includes `migrate`).
