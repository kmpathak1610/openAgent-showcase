# OpenAgent Diagrams

Interactive technical and workflow diagrams for the OpenAgent showcase, generated with Archify (showcase quality profile, 9/9 validation checks each).

## Files

| Diagram | Type | Interactive | Preview |
|---|---|---|---|
| System architecture | architecture | [architecture.html](architecture.html) | [architecture.png](architecture.png) |
| Tool execution with approval | workflow | [workflow-tool-approval.html](workflow-tool-approval.html) | [workflow-tool-approval.png](workflow-tool-approval.png) |
| Message to agent reply | sequence | [sequence-message-reply.html](sequence-message-reply.html) | [sequence-message-reply.png](sequence-message-reply.png) |
| RAG knowledge flow | dataflow | [dataflow-rag.html](dataflow-rag.html) | [dataflow-rag.png](dataflow-rag.png) |

PNG previews are 1440x900 light-theme captures for inline viewing on GitHub. Open the HTML files locally for the full explorable experience (pan/zoom, search, focus, relationship tracing, theme switching, light/dark, truthful SVG/PNG exports).

## 1. System architecture

`architecture.html` — components and trust boundaries: Angular frontend, Go HTTP API, WebSocket hub, agent runtime, LLM providers, RAG retriever, memory service, approval-gated tool executor, and PostgreSQL + pgvector. The main path follows a user task from the workspace to durable state; side views isolate reasoning dependencies and safety/realtime paths.

## 2. Tool execution with approval

`workflow-tool-approval.html` — the agent planning loop across User Interface, Agent Runtime, Policy & Recovery, and Tool Execution lanes: channel intake, context loading (agent/project/RAG/memory, sanitized), structured LLM decision with repair retries, approval gate for high-risk calls, validated execution with audit and idempotency, evidence recording, and final reply. Denials wait and resume; failures return to retry without losing trace.

## 3. Message to agent reply

`sequence-message-reply.html` — request lifecycle for a channel message that triggers agent work: `POST /messages` persists message and task, the API enqueues `agent_run` and returns immediately, the worker drives `ExecuteTask` (load context, provider completion, persist run/events), the hub broadcasts progress, and the client reconciles via WebSocket with `GET /tasks/:id` as fallback.

## 4. RAG knowledge flow

`dataflow-rag.html` — knowledge pipeline from sources to grounded prompts: uploads parsed, normalized, chunked (500 tokens/50 overlap), embedded, and stored as HNSW vectors; queries embed, permission-filter (`GetAllowedDocIDs`), hybrid-merge keyword and vector hits with rerank, inject sanitized top-k chunks with attribution, and ground the runtime prompt.

## Provenance

Authored from the showcased tree (`backend/internal/runtime/`, `llm/`, `tool/`, `knowledge/`, `memory/`, `team/`, `worker/`, `ws/`, `http/`, `repository/`, migrations). No production secrets or internal design docs were used as sources.

Delivery receipts (deterministic `deliver` checks, 9/9 each, 0 errors, 0 warnings):

- architecture: spec `8f7aa63a…` (3,918 B), artifact `86a74894…` (812,722 B)
- workflow: spec `9508bc71…` (5,046 B), artifact `9b0bac22…` (818,277 B)
- sequence: spec `6cc6c2eb…` (11,719 B), artifact `ebd88cb1…` (812,920 B)
- dataflow: spec `b811f01e…` (5,302 B), artifact `910deafb…` (813,331 B)

Browser evidence (`visual-check`, automated Chromium, stills at 1440x900 / 1600x1000 / 1920x1080 / 2048x1320, light + dark):

- architecture: pass, no overflow at any checked viewport
- workflow: content intact; viewer scrolls ~65px vertically at 1440x900 (965px content vs 900px viewport); fully explorable
- sequence: content intact; 15-step vertical timeline scrolls (1,445px at 1440x900); expected for a full request lifecycle; fully explorable
- dataflow: content intact; 5-stage pipeline scrolls (1,300px at 1440x900); fully explorable

Perceptual visual review was not performed by an image-capable reviewer; PNG previews above are provided for direct inspection.
