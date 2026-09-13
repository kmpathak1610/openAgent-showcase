# Default Home Channel + Agent Auto-Reply — Design Spec

Date: 2026-09-09
Status: Approved (signup, routing, safeguards)
Project: D:\ai_projects\openagent

## 1. Context

OpenAgent signup (`backend/internal/http/handlers/auth.go:Register`) creates user + organization + default assistant, but no channels. Channel creation is manual via `POST /channels` (`backend/internal/http/handlers/channel.go`, `backend/internal/repository/repository.go:CreateChannel`). Message creation (`backend/internal/http/handlers/message.go`) persists + broadcasts, but org-level channels (projectID nil) trigger zero agent work; project channels only trigger on @mention or explicit autonomy triggers. Agent replies flow only through `runtime.ExecuteTask` via `worker agent_run` tasks.

Result: new users land with no home, and org chat never gets agent replies.

## 2. Requirements (validated)

- Default channel created on signup: org-level `general` (name `general`, display `General`, type `standard`, projectID nil), owner as admin, system welcome message.
- One place for agent communication: `general` is the home for all user↔agent chat.
- Auto-reply: concerned agent automatically replies (smart router picks responder).
- Agent-to-agent communication present, grouped in threads under the triggering user message.
- Existing orgs without `general` get it lazily.

Out of scope (YAGNI): starter project creation, new execution engine, scheduled digests,DMs, custom thread UI beyond existing `thread_id` + replies.

## 3. Architecture

Signup owns channel bootstrap; message path owns routing. Both reuse existing primitives (`organizations`, `channels`, `channel_members`, `messages`, `message_threads`, `tasks`, `agent_runs`, `ws.Hub`, `worker.Pool`).

- `Register` → `CreateUser` → `CreateOrganization` → `EnsureDefaultAssistant` → `EnsureGeneralChannel(orgID, ownerID)` (idempotent by `(orgID, name=general)` lookup, transaction: channel + owner admin + welcome).
- `Message.Create` in `general` with `sender_type=user` → `RouteGeneralMessage` (mention wins → eligible/LLM smart pick → default assistant fallback) → responder replies inline (`sender_type=agent`), delegations create subtasks whose messages carry `thread_id=userMessageID`.
- Agent messages never retrigger routing (loop guard). All writes enforce `organization_id` isolation.

## 4. Components

### 4.1 Signup bootstrap

`EnsureGeneralChannel` in repository or assistant service: `GetChannelByName(orgID, general)` else `CreateChannel(orgID, nil, general, General, standard, ownerID)` + welcome `CreateMessage(system)`. Called after assistant ensure; errors logged, never fail signup. Lazy path: `Login` and `Message.List/Create` ensure if missing.

### 4.2 Smart router

`RouteGeneralMessage(orgID, channelID, userMessageID, body)`: parse `@mention` via existing mention resolver; if matched and agent in org, select it. Else list org agents (active, non-archived), score by capabilities/keywords or delegate to team orchestrator / LLM pick; fallback `openagent-assistant`. Creates responder task (org-level task or direct run) and enqueues `agent_run`. Responder `send_message`/`completed Response` posts inline agent message in `general`.

### 4.3 Threaded collaboration

When responder uses `delegate_task`/`create_task`, child outputs post with `thread_id` set to original user message. `message_threads` row maintained by existing `CreateMessage` thread logic. UI shows inline final answers + expandable thread for agent-to-agent steps. WS emits `message.created` and `agent.message` for both.

## 5. Data Flow

1. `POST /auth/register` → user, org, assistant, `general` + welcome → `{user, organization, token}`.
2. User posts in `general` → `CreateMessage(user)` + `message.created` broadcast.
3. Router picks agent → task/run enqueued → `ExecuteTask` loop (RAG + memory + tools as today).
4. Agent reply inline in `general` → `agent.message` broadcast.
5. Delegations → subtask runs → thread replies under user message.
6. Existing orgs: first post-login or message ensures `general` before routing.

## 6. Error Handling

- Channel bootstrap failure: log, continue signup/login/message (never 500 the parent).
- Duplicate `general`: unique lookup + `ON CONFLICT DO NOTHING` semantics, safe retry.
- Router no agents: fallback assistant; assistant missing: re-ensure then silent system note.
- Provider/rate-limit/approval failures: existing runtime guards apply; user sees graceful agent message or nothing (no crash).
- Loop guard: `sender_type != user` skips routing entirely.
- Tenancy: every lookup scoped by `organization_id`; private-channel checks unchanged.

## 7. Testing

- Signup creates `general` once (second register/login does not duplicate).
- Org message from user triggers router; agent message does not retrigger.
- Mention routes to mentioned agent; no mention routes via smart pick with assistant fallback.
- Delegated work appears with `thread_id` equal to user message; thread metadata increments.
- Bootstrap failure does not fail signup (fault-injected repo test).

## 8. Rollout

1. Repository ensure + Register/Login hook (no migration; `general` fits existing schema).
2. Router on `general` user messages (mention → smart → fallback).
3. Thread linkage for delegations + WS verification in Docker.
4. Docs update (`docs/api.md`) + e2e register → post → auto-reply → thread.
