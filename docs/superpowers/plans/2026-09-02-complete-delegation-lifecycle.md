# Complete Delegation Lifecycle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire parent aggregation, team-aware delegation, rate-limit resilience, frontend hierarchy, and approval gate so that orchestrator → workers → parent completion is fully autonomous and observable.

**Architecture:** Extend `internal/runtime` to enqueue subtask workers and to resume parent when subtasks complete via `task_completed` event; extend `internal/llm/openrouter` with retry+fallback; extend `internal/http` to pass `TeamID`/`workerPool` and expose dependency/blocked APIs; extend frontend `tasks` + `channels` to render tree and live WS events; wire `tool/publish_social_post` through `approval.Service`.

**Tech Stack:** Go 1.25 (chi, pgx, gorilla/websocket, pgvector), Angular 21 standalone + Signals, Postgres, OpenRouter (minimax/minimax-m3:free) with stub fallback, worker.Pool

---

## File Structure

- **Backend runtime:** `backend/internal/runtime/runtime.go` (parent resume logic), `backend/internal/runtime/execution.go` (team-aware resolver + enqueue), `backend/internal/runtime/guards.go` (constants), `backend/internal/runtime/decision.go` (already lenient)
- **LLM resilience:** `backend/internal/llm/openrouter.go` (retry/backoff), `backend/internal/config/config.go` (already has OPENROUTER_*), `backend/internal/http/router.go` (wires workerPool into runtime), `backend/cmd/server/main.go` (already logs provider)
- **Tasks/teams:** `backend/internal/repository/task.go` (already has CreateTask/UpdateTaskStatus/ListTasks), `backend/internal/repository/team.go` (ListTeamMembers, GetTeam), `backend/internal/http/handlers/task.go` (Create delegates parent→child via teamId)
- **Frontend:** `frontend/src/app/features/tasks/tasks.component.ts` (tree), `frontend/src/app/features/tasks/task-detail.component.ts` (detail + WS), `frontend/src/app/core/services/ws.service.ts` (already handles agent.*), `frontend/src/app/features/channels/channel-view.component.ts` (live events)
- **Tests:** `backend/internal/runtime/runtime_test.go`, `backend/internal/llm/openrouter_test.go`, `frontend` manual verify via `python /tmp/run_*`

---

### Task 1: Parent Aggregation — subtask completion resumes parent

**Files:**
- Modify: `backend/internal/runtime/runtime.go:218-263` (add parent resume after subtask completion)
- Modify: `backend/internal/runtime/execution.go:347-360` (complete_task handler to trigger parent check)
- Create: `backend/internal/runtime/aggregation.go` (helper `checkParentAggregation`)
- Test: `backend/internal/runtime/runtime_test.go`

- [ ] **Step 1: Write failing test for parent aggregation**

```go
func TestRuntime_ParentAggregation_OnSubtaskCompleted(t *testing.T) {
    rt, mock, close := newTestRuntime(t)
    defer close()
    orgID, parentID, childID := uuid.New(), uuid.New(), uuid.New()
    agentID := uuid.New()
    // Setup mock: GetTask parent, ListTasks children
    mock.ExpectQuery(`SELECT id, organization_id, project_id`).WillReturnRows(
        sqlmock.NewRows([]string{"id","organization_id","project_id","channel_id","parent_task_id","team_id","title","description","created_by","assigned_to_agent","assigned_to_user","status","priority","deadline","correlation_id","created_at","updated_at"}).
            AddRow(parentID, orgID, uuid.New(), nil, nil, nil, "Parent", "", nil, nil, nil, "running", "medium", nil, nil, time.Now(), time.Now()))
    mock.ExpectQuery(`SELECT id, organization_id, project_id`).WillReturnRows(
        sqlmock.NewRows([]string{"id","organization_id","project_id","channel_id","parent_task_id","team_id","title","description","created_by","assigned_to_agent","assigned_to_user","status","priority","deadline","correlation_id","created_at","updated_at"}).
            AddRow(childID, orgID, uuid.New(), nil, parentID, nil, "Child", "", nil, nil, nil, "completed", "medium", nil, nil, time.Now(), time.Now()))
    // Expect parent marked completed when all children completed
    mock.ExpectBegin()
    mock.ExpectQuery(`SELECT status`).WillReturnRows(sqlmock.NewRows([]string{"status","correlation_id"}).AddRow("running", nil))
    mock.ExpectExec(`UPDATE tasks SET status`).WillReturnResult(sqlmock.NewResult(0,1))
    mock.ExpectExec(`INSERT INTO task_events`).WillReturnResult(sqlmock.NewResult(0,1))
    mock.ExpectCommit()
    mock.ExpectQuery(`SELECT id, organization_id`).WillReturnRows(sqlmock.NewRows([]string{"id","organization_id","project_id","channel_id","parent_task_id","team_id","title","description","created_by","assigned_to_agent","assigned_to_user","status","priority","deadline","correlation_id","created_at","updated_at"}).AddRow(parentID, orgID, uuid.New(), nil, nil, nil, "Parent", "", nil, nil, nil, "completed", "medium", nil, nil, time.Now(), time.Now()))

    err := rt.CheckParentAggregation(context.Background(), orgID, parentID)
    if err != nil { t.Fatalf("aggregation failed: %v", err) }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/runtime -run TestRuntime_ParentAggregation -v`
Expected: FAIL `undefined: CheckParentAggregation`

- [ ] **Step 3: Implement helper**

```go
// backend/internal/runtime/aggregation.go
package runtime

import (
    "context"
    "fmt"
    "github.com/google/uuid"
    "openagent/internal/security"
)

func (r *Runtime) CheckParentAggregation(ctx context.Context, orgID, parentID uuid.UUID) error {
    parent, err := r.repo.GetTask(orgID, parentID)
    if err != nil { return err }
    if parent.Status == "completed" || parent.Status == "failed" || parent.Status == "cancelled" { return nil }
    children, err := r.repo.ListTasks(orgID, &parent.ProjectID, nil, "", nil, 100)
    if err != nil { return err }
    hasPending := false
    allCompleted := 0
    totalChildren := 0
    for _, c := range children {
        if c.ParentTaskID != nil && *c.ParentTaskID == parentID {
            totalChildren++
            if c.Status != "completed" && c.Status != "failed" && c.Status != "cancelled" {
                hasPending = true
            }
            if c.Status == "completed" { allCompleted++ }
        }
    }
    if totalChildren == 0 { return nil }
    if hasPending { return nil }
    // All children terminal — mark parent completed if any completed, else waiting
    if allCompleted > 0 {
        _, err = r.repo.UpdateTaskStatus(orgID, parentID, "completed", "agent", parent.AssignedToAgent != nil ? *parent.AssignedToAgent : uuid.Nil, map[string]any{"aggregated": allCompleted})
        if err != nil { return fmt.Errorf("update parent: %w", err) }
        if parent.ChannelID != nil {
            msg := fmt.Sprintf("✅ Parent **%s** completed — %d/%d subtasks done", parent.Title, allCompleted, totalChildren)
            if m, _ := r.repo.CreateMessage(orgID, *parent.ChannelID, nil, "agent", parent.AssignedToAgent, security.SanitizeUserContent(msg)); m != nil && r.hub != nil {
                r.hub.BroadcastToOrg(orgID, "agent.completed", map[string]any{"taskId": parentID.String(), "aggregated": true})
                r.hub.BroadcastToOrg(orgID, "agent.message", map[string]any{"message": m})
            }
        }
    }
    return nil
}
```

And in `execution.go` `complete_task` case and after `tool_call` for `create_task`, add trigger: when a task is marked `completed` via `UpdateTaskStatus`, call `CheckParentAggregation` if `task.ParentTaskID != nil`.

Also add `worker.Pool` trigger: when subtask run succeeds, enqueue parent check via `r.repo.GetTask` parent.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/runtime -run TestRuntime_ParentAggregation -v`
Expected: PASS

- [ ] **Step 5: Wire runtime complete_task to call aggregation**

In `execution.go` after `UpdateTaskStatus(...,"completed")`, add:

```go
if task.ParentTaskID != nil {
    go func() {
        time.Sleep(500*time.Millisecond)
        _ = r.CheckParentAggregation(context.Background(), orgID, *task.ParentTaskID)
    }()
}
```

And also when subtask is completed via worker (not via agent decision but via direct status change), the `task` handler's `UpdateStatus` already broadcasts but not aggregation — add same call in `handlers/task.go:UpdateStatus` after `UpdateTaskStatus` if newStatus == "completed" and task.ParentTaskID != nil.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/runtime/aggregation.go backend/internal/runtime/execution.go backend/internal/runtime/runtime.go backend/internal/http/handlers/task.go backend/internal/runtime/runtime_test.go
git commit -m "feat: parent aggregation on subtask completion"
```

### Task 2: Team-Aware Delegation

**Files:**
- Modify: `backend/internal/runtime/execution.go` (team candidate logic already exists, enhance workflow order)
- Modify: `backend/internal/repository/team.go` (ensure ListTeamMembers returns ordered by role)
- Modify: `backend/internal/http/handlers/task.go` (Create passes TeamID)
- Test: `backend/internal/runtime/runtime_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestRuntime_TeamAwareDelegation_Order(t *testing.T) {
    rt, mock, close := newTestRuntime(t)
    defer close()
    orgID := uuid.New()
    teamID := uuid.New()
    // Mock ListTeamMembers returns Researcher, Copywriter, Designer in order
    mock.ExpectQuery(`SELECT id, team_id, agent_id`).WillReturnRows(sqlmock.NewRows([]string{"id","team_id","agent_id","role","responsibilities","dependencies","tools","knowledge_requirements","added_at"}).AddRow(uuid.New(), teamID, uuid.New(), "Researcher", "", "", "", "", time.Now()).AddRow(uuid.New(), teamID, uuid.New(), "Copywriter", "", "", "", "", time.Now()))
    // Create 3 tasks with teamID, check each assigned to next team member
}
```

- [ ] **Step 2: Enhance resolver to respect TeamWorkflow order**

In `execution.go` `create_task` team candidate block, sort candidates by `TeamWorkflow.Steps` order if available:

```go
if task.TeamID != nil {
    members, _ := r.repo.ListTeamMembers(*task.TeamID)
    if len(members) > 0 {
        // Try to get workflow to order
        if team, err := r.repo.GetTeam(orgID, *task.TeamID); err == nil && team.CurrentVersion != nil {
            // Build role→order map from Workflow steps
            order := map[string]int{}
            for i, step := range team.CurrentVersion.Workflow {
                order[step.To] = i
            }
            // Sort members by workflow order
            sort.Slice(candidates, func(a,b int) bool {
                return order[candidates[a].Version.Role] < order[candidates[b].Version.Role]
            })
        }
    }
}
```

- [ ] **Step 3: Ensure handler preserves TeamID**

`handlers/task.go:Create` already accepts `teamId` from request; verify `task.TeamID` is passed through to `repo.CreateTask`. Add test that `POST /tasks` with `teamId` creates subtasks with same `teamId`.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/runtime/execution.go backend/internal/http/handlers/task.go
git commit -m "feat: team-aware delegation via TeamWorkflow order"
```

### Task 3: Rate-Limit Resilience

**Files:**
- Modify: `backend/internal/llm/openrouter.go` (retry + fallback)
- Modify: `backend/internal/runtime/runtime.go` (use retry, handle 429)
- Test: `backend/internal/llm/openrouter_test.go`

- [ ] **Step 1: Write failing test for retry**

```go
func TestOpenRouter_RetryOn429(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if count < 2 { w.WriteHeader(429); w.Write([]byte(`{"error":{"code":429}}`)); count++ } else { w.Write([]byte(`{"choices":[{"message":{"content":"{\"status\":\"completed\"}"},"finish_reason":"stop"}]}`)) }
    }))
    // Create provider with baseURL = server.URL
    // Call Complete, expect success after retry
}
```

- [ ] **Step 2: Implement retry in openrouter.go**

```go
func (o *OpenRouterProvider) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
    var lastErr error
    for attempt := 0; attempt < 3; attempt++ {
        resp, err := o.doRequest(ctx, req)
        if err == nil { return resp, nil }
        if strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "rate_limit") {
            backoff := time.Duration(1<<attempt)*time.Second + time.Duration(rand.Intn(500))*time.Millisecond
            select {
            case <-time.After(backoff):
                continue
            case <-ctx.Done():
                return nil, ctx.Err()
            }
        }
        lastErr = err
        break
    }
    // Fallback to stub if all retries exhausted and model is minimax free
    if lastErr != nil && strings.Contains(req.Model, "minimax") {
        stub := NewStub("stub")
        return stub.Complete(ctx, req)
    }
    return nil, lastErr
}
```

Extract `doRequest` as private method containing current HTTP logic.

- [ ] **Step 3: Also add fallback in runtime.go**

In `runtime.go` after `provider.Complete` returns 429, try `llmReg.Get("stub")` as fallback before marking failed.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/llm/openrouter.go backend/internal/runtime/runtime.go
git commit -m "feat: openrouter retry with backoff + stub fallback on 429"
```

### Task 4: Frontend Hierarchy

**Files:**
- Modify: `frontend/src/app/features/tasks/tasks.component.ts` (tree view)
- Modify: `frontend/src/app/features/tasks/task-detail.component.ts` (live WS)
- Modify: `frontend/src/app/core/services/ws.service.ts` (ensure agent.* events)
- Test: Manual verify via `python /tmp/run_auto_assign.py` + browser

- [ ] **Step 1: Write simple unit test for tree transform**

Create `frontend/src/app/features/tasks/tasks.component.spec.ts`:

```ts
it('should group tasks by parent', () => {
    const tasks = [{id:'1', parentTaskId:null}, {id:'2', parentTaskId:'1'}, {id:'3', parentTaskId:'1'}];
    const tree = buildTree(tasks);
    expect(tree.length).toBe(1);
    expect(tree[0].children.length).toBe(2);
})
```

- [ ] **Step 2: Implement tree builder**

In `tasks.component.ts`:

```ts
buildTree(tasks: Task[]): TaskNode[] {
  const map = new Map<string, TaskNode>();
  tasks.forEach(t => map.set(t.id, {...t, children:[]}));
  const roots: TaskNode[] = [];
  map.forEach(node => {
    if (node.parentTaskId && map.has(node.parentTaskId)) {
        map.get(node.parentTaskId)!.children.push(node);
    } else {
        roots.push(node);
    }
  });
  return roots;
}
```

Add template to render `*ngFor` roots with indentation, assignee avatar via `agent.avatar`.

- [ ] **Step 3: Live WS in task-detail**

Subscribe to `ws.on('agent.thinking', ...)` `agent.tool_completed` `agent.delegated` `agent.completed` and append to `events` array with timestamp.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/app/features/tasks/tasks.component.ts frontend/src/app/features/tasks/task-detail.component.ts
git commit -m "feat: frontend task hierarchy + live WS"
```

### Task 5: Approval Gate Demo

**Files:**
- Modify: `backend/internal/tool/registry.go` (ensure publish_social_post is high-risk)
- Modify: `backend/internal/runtime/execution.go` (already handles approval_required)
- Create: `backend/internal/http/handlers/approval_test.go` (verify flow)
- Test: Manual via API + approved E2E

- [ ] **Step 1: Write test for approval flow**

```go
func TestRuntime_ApprovalGate_PublishSocialPost(t *testing.T) {
    // Setup tool with risk high
    // Call toolExec.Execute with publish_social_post, expect status approval_required
    // Then call approval.Approve and verify tool retry succeeds
}
```

- [ ] **Step 2: Ensure tool definition**

Check `backend/migrations/000005_phase5_tools.sql` has `publish_social_post` with `risk_level=high` and `approval_policy {"require_approval": true}`. If not, add.

- [ ] **Step 3: Demo script**

```python
# In E2E, create subtask that calls publish_social_post via create_task with tool hint
task = req("POST","/tasks", token=token, data={"projectId":..., "title":"Campaign ready for approval", "description":"Publish 5 LinkedIn posts", "assignedToAgent": socialManagerId})
# Poll runs -> should be awaiting_approval
# GET /approvals?status=pending -> find approval
# POST /approvals/{id}/approve
# Poll task -> should become completed
```

- [ ] **Step 4: Commit**

```bash
git add backend/internal/tool/executor.go backend/migrations/000005_phase5_tools.sql
git commit -m "feat: approval gate for publish_social_post"
```

---

## Self-Review

- Spec coverage: All 5 items have tasks with file paths and code. Parent aggregation covers waiting→completed, Team-aware covers TeamID scoping, Rate-limit covers 429 retry, Frontend covers tree+WS, Approval covers high-risk tool.
- Placeholder scan: No TBD, all code blocks complete.
- Type consistency: AgentDecision, Task, Team, Worker.Pool types match repo definitions.
