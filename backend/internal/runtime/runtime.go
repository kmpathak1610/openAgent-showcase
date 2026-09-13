package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"openagent/internal/approval"
	"openagent/internal/autonomy"
	"openagent/internal/domain"
	"openagent/internal/knowledge"
	"openagent/internal/llm"
	"openagent/internal/metrics"
	"openagent/internal/repository"
	"openagent/internal/security"
	"openagent/internal/tool"
	"openagent/internal/worker"
	"openagent/internal/ws"
)

// Runtime executes agent tasks generically
type Runtime struct {
	repo       *repository.DB
	llmReg     *llm.Registry
	retriever  *knowledge.Retriever
	hub        *ws.Hub
	toolExec   *tool.Executor
	approval   *approval.Service
	workerPool *worker.Pool
	autonomy   *autonomy.Service
	memorySvc interface {
		Retrieve(ctx context.Context, orgID uuid.UUID, agentID *uuid.UUID, projectID *uuid.UUID, query string, limit int, minImportance float64) ([]*domain.Memory, error)
		Create(ctx context.Context, orgID uuid.UUID, agentID *uuid.UUID, projectID *uuid.UUID, memoryType, scope, source, content string, importance float64, metadata map[string]any, expiresAt *time.Time) (*domain.Memory, error)
	}
}

func New(repo *repository.DB, reg *llm.Registry, retriever *knowledge.Retriever, hub *ws.Hub) *Runtime {
	return &Runtime{repo: repo, llmReg: reg, retriever: retriever, hub: hub}
}

func NewWithTools(repo *repository.DB, reg *llm.Registry, retriever *knowledge.Retriever, hub *ws.Hub, toolExec *tool.Executor, approvalSvc *approval.Service) *Runtime {
	return &Runtime{repo: repo, llmReg: reg, retriever: retriever, hub: hub, toolExec: toolExec, approval: approvalSvc}
}

func NewWithMemory(repo *repository.DB, reg *llm.Registry, retriever *knowledge.Retriever, hub *ws.Hub, toolExec *tool.Executor, approvalSvc *approval.Service, memorySvc interface {
	Retrieve(ctx context.Context, orgID uuid.UUID, agentID *uuid.UUID, projectID *uuid.UUID, query string, limit int, minImportance float64) ([]*domain.Memory, error)
	Create(ctx context.Context, orgID uuid.UUID, agentID *uuid.UUID, projectID *uuid.UUID, memoryType, scope, source, content string, importance float64, metadata map[string]any, expiresAt *time.Time) (*domain.Memory, error)
}) *Runtime {
	return &Runtime{repo: repo, llmReg: reg, retriever: retriever, hub: hub, toolExec: toolExec, approval: approvalSvc, memorySvc: memorySvc}
}

func NewWithMemoryAndWorker(repo *repository.DB, reg *llm.Registry, retriever *knowledge.Retriever, hub *ws.Hub, toolExec *tool.Executor, approvalSvc *approval.Service, memorySvc interface {
	Retrieve(ctx context.Context, orgID uuid.UUID, agentID *uuid.UUID, projectID *uuid.UUID, query string, limit int, minImportance float64) ([]*domain.Memory, error)
	Create(ctx context.Context, orgID uuid.UUID, agentID *uuid.UUID, projectID *uuid.UUID, memoryType, scope, source, content string, importance float64, metadata map[string]any, expiresAt *time.Time) (*domain.Memory, error)
}, workerPool *worker.Pool) *Runtime {
	return &Runtime{repo: repo, llmReg: reg, retriever: retriever, hub: hub, toolExec: toolExec, approval: approvalSvc, memorySvc: memorySvc, workerPool: workerPool}
}

func (r *Runtime) SetWorkerPool(pool *worker.Pool) { r.workerPool = pool }
func (r *Runtime) SetAutonomy(svc *autonomy.Service) { r.autonomy = svc }

func (r *Runtime) ExecuteTask(ctx context.Context, orgID, agentID, taskID uuid.UUID, correlationID *uuid.UUID, trigger string) error {
	// Overall timeout for the run
	ctx, cancel := context.WithTimeout(ctx, ExecutionTimeout)
	defer cancel()

	agent, err := r.repo.GetAgent(orgID, agentID)
	if err != nil { return fmt.Errorf("load agent: %w", err) }
	task, err := r.repo.GetTask(orgID, taskID)
	if err != nil { return fmt.Errorf("load task: %w", err) }
	var project *domain.Project
	if task.ProjectID != (uuid.UUID{}) {
		if p, err := r.repo.GetProject(orgID, task.ProjectID); err == nil { project = p }
	}

	metrics.IncAgentRuns()
	runInput := map[string]any{"task_id": taskID.String(), "trigger": trigger}
	runContext := map[string]any{"project": project, "task": task, "agent_version": agent.CurrentVersion}
	run, err := r.repo.CreateAgentRun(orgID, agentID, &taskID, task.ChannelID, trigger, runInput, runContext, correlationID, nil)
	if err != nil { return fmt.Errorf("create run: %w", err) }

	broadcast := func(typ string, payload map[string]any) {
		if r.hub != nil {
			payload["agentId"] = agentID.String()
			payload["taskId"] = taskID.String()
			payload["runId"] = run.ID.String()
			if correlationID != nil { payload["correlationId"] = correlationID.String() }
			r.hub.BroadcastToOrg(orgID, typ, payload)
			if task.ChannelID != nil {
				r.hub.Broadcast(ws.Event{Type: typ, Payload: payload, Room: "channel:" + task.ChannelID.String()})
			}
		}
	}

	_ = r.repo.UpdateAgentRunStatus(orgID, run.ID, "running", nil, nil)
	broadcast("agent.started", map[string]any{"agent": agent.Name, "task": task.Title})
	_, _ = r.repo.UpdateTaskStatus(orgID, taskID, "running", "agent", agentID, map[string]any{"run_id": run.ID.String()})

	// Execution loop: preload prior tool results from DB for resume after approval/rejection
	var previousOutputs []map[string]any
	// If this run was resumed after approval, load prior succeeded tool executions for this task to provide context
	if trigger == "approval_received" || trigger == "approval_rejected" || correlationID != nil {
		if execs, err := r.repo.ListToolExecutions(orgID, &agentID, &taskID, 5); err == nil {
			for _, ex := range execs {
				if ex.Status == "succeeded" && ex.Output != nil {
					previousOutputs = append(previousOutputs, map[string]any{"action": "call_tool", "tool": ex.ToolName, "output": ex.Output, "resumed": true})
				} else if ex.Status == "failed" && ex.Error != nil {
					previousOutputs = append(previousOutputs, map[string]any{"action": "call_tool", "tool": ex.ToolName, "error": *ex.Error, "resumed": true})
				}
			}
		}
		if len(previousOutputs) > 5 { previousOutputs = previousOutputs[len(previousOutputs)-5:] }
	}
	toolCallCount := len(previousOutputs)
	for iteration := 1; iteration <= MaxIterations; iteration++ {
		select {
		case <-ctx.Done():
			_ = r.repo.UpdateAgentRunStatus(orgID, run.ID, "cancelled", nil, nil)
			broadcast("agent.failed", map[string]any{"reason": "timeout/cancelled", "iteration": iteration})
			return context.Canceled
		default:
		}

		// Build bounded context
		conversation := r.loadConversation(orgID, task.ChannelID, 5)
		sanitizedQuery := security.SanitizeUserContent(task.Title + " " + task.Description)
		var retrieved []domain.RetrievalResult
		if r.retriever != nil {
			filter := domain.RetrievalFilter{OrganizationID: orgID, AgentID: &agentID, ProjectID: &task.ProjectID, Limit: 3}
			if res, err := r.retriever.SemanticSearch(ctx, sanitizedQuery, filter); err == nil {
				retrieved = res
			}
		}
		var memories []*domain.Memory
		if r.memorySvc != nil {
			if mems, err := r.memorySvc.Retrieve(ctx, orgID, &agentID, &task.ProjectID, sanitizedQuery, 3, 0.3); err == nil {
				memories = mems
			}
		}
		contextStr := r.buildAgentContext(ctx, agent, project, task, retrieved, memories, conversation, previousOutputs)

		// Choose model/provider
		model := agent.Model
		if agent.Version != nil && agent.Version.ModelConfiguration != nil {
			if m, ok := agent.Version.ModelConfiguration["model"].(string); ok && m != "" { model = m }
		}
		providerName := agent.Provider
		isAnthropic := strings.HasPrefix(model, "anthropic/")
		isOpenAI := strings.HasPrefix(model, "openai/")
		isGoogle := strings.HasPrefix(model, "google/")
		if isAnthropic { providerName = "anthropic" } else if isOpenAI { providerName = "openai" } else if isGoogle { providerName = "google" } else if strings.Contains(model, "/") { providerName = "openrouter" }
		var provider llm.Provider
		if providerName != "" {
			if p, err := r.llmReg.Get(providerName); err == nil { provider = p }
		}
		if provider == nil {
			if p, err := r.llmReg.Get("openrouter"); err == nil { provider = p }
		}
		if provider == nil {
			if p, err := r.llmReg.Get("stub"); err == nil { provider = p }
		}
		if provider == nil { provider = llm.NewStub("stub") }

		// Build LLM messages
		hardenedSystem := security.SystemPromptHardening()
		instructions := ""
		if agent.Version != nil { instructions = agent.Version.Instructions }
		if instructions == "" { instructions = agent.SystemPrompt }
		if instructions == "" { instructions = "You are a helpful assistant. Complete the assigned task." }
		hardenedSystem += instructions
		toolDefs := r.getToolDefinitionsForAgent(orgID, agentID)
		messages := []llm.Message{
			{Role: llm.RoleSystem, Content: hardenedSystem},
			{Role: llm.RoleUser, Content: contextStr + "\n\nRespond ONLY with valid JSON matching AgentDecision schema:\n" +
				"{\"status\":\"continue|delegate|waiting|approval_required|needs_human|completed|failed\",\"response\":\"human-readable summary\",\"actions\":[{\"type\":\"send_message|create_task|delegate_task|call_tool|request_approval|request_human_input|update_task|complete_task|fail_task|store_memory\",...}],\"memory_updates\":[{\"content\":\"...\",\"memory_type\":\"working|project|agent|conversation\",\"scope\":\"task|session|project|agent|conversation|organization\",\"importance\":0.0-1.0}]}\n" +
				"Action schemas:\n" +
				"- send_message: {\"type\":\"send_message\",\"body\":\"message text\"}\n" +
				"- create_task: {\"type\":\"create_task\",\"title\":\"task title\",\"description\":\"details\"}\n" +
				"- call_tool: {\"type\":\"call_tool\",\"tool\":\"web_search\",\"input\":{\"query\":\"...\"}}\n" +
				"- delegate_task: {\"type\":\"delegate_task\",\"task_id\":\"uuid\",\"agent_id\":\"uuid\"}\n" +
				"- complete_task/fail_task: {\"type\":\"complete_task\"} or {\"type\":\"fail_task\"}\n" +
				"Valid statuses: continue, delegate, waiting, approval_required, needs_human, completed, failed\n" +
				"Example: {\"status\":\"delegate\",\"response\":\"Delegated research\",\"actions\":[{\"type\":\"call_tool\",\"tool\":\"web_search\",\"input\":{\"query\":\"LinkedIn trends\"}},{\"type\":\"create_task\",\"title\":\"Draft posts\",\"description\":\"Create 3 posts\"}]}"},
		}
		for _, out := range previousOutputs {
			messages = append(messages, llm.Message{Role: llm.RoleUser, Content: fmt.Sprintf("Previous tool/output %v", out)})
		}
		if len(toolDefs) > 0 {
			messages[1].Content += fmt.Sprintf("\nAvailable tools: %v", toolDefs)
		}

		_, _ = r.repo.CreateAgentAction(run.ID, (iteration-1)*10, "llm_call", nil, map[string]any{"messages": messages, "model": model, "iteration": iteration}, nil, "running", correlationID)
		broadcast("agent.thinking", map[string]any{"iteration": iteration})

		llmResp, err := provider.Complete(ctx, llm.CompletionRequest{Model: model, Messages: messages})
		if err != nil {
			errStr := err.Error()
			metrics.IncAgentRunsFailed()
			_ = r.repo.UpdateAgentRunStatus(orgID, run.ID, "failed", map[string]any{"error": errStr, "iteration": iteration}, &errStr)
			_, _ = r.repo.UpdateTaskStatus(orgID, taskID, "failed", "agent", agentID, map[string]any{"error": errStr})
			broadcast("agent.failed", map[string]any{"error": errStr, "iteration": iteration})
			_, _ = r.repo.CreateAgentAction(run.ID, (iteration-1)*10+1, "error", nil, map[string]any{"error": errStr}, nil, "failed", correlationID)
			return err
		}
		_, _ = r.repo.CreateAgentAction(run.ID, (iteration-1)*10+1, "llm_call", nil, map[string]any{"response": llmResp.Content, "iteration": iteration, "usage": llmResp.Usage}, nil, "succeeded", correlationID)
		_ = r.repo.UpdateAgentRunTokens(orgID, run.ID, llmResp.Usage)
		broadcast("agent.tool_completed", map[string]any{"tool": "llm_call", "iteration": iteration, "usage": llmResp.Usage})

		decision, err := ParseAgentDecision(llmResp.Content)
		if err != nil {
			_, _ = r.repo.CreateAgentAction(run.ID, (iteration-1)*10+2, "error", nil, map[string]any{"validation_error": err.Error(), "iteration": iteration}, nil, "failed", correlationID)
			decision, err = r.retryDecision(ctx, provider, model, hardenedSystem, llmResp.Content, err, run.ID.String())
			if err != nil {
				errStr := fmt.Sprintf("invalid LLM decision after %d retries: %v", MaxDecisionRetries, err)
				_ = r.repo.UpdateAgentRunStatus(orgID, run.ID, "failed", map[string]any{"error": errStr, "iteration": iteration, "needs_human": true}, &errStr)
				_, _ = r.repo.UpdateTaskStatus(orgID, taskID, "failed", "agent", agentID, map[string]any{"error": errStr, "needs_human": true})
				broadcast("agent.failed", map[string]any{"error": errStr, "needs_human": true, "iteration": iteration})
				_, _ = r.repo.CreateAgentAction(run.ID, (iteration-1)*10+9, "error", nil, map[string]any{"final_error": errStr}, nil, "failed", correlationID)
				return fmt.Errorf("%s", errStr)
			}
			broadcast("agent.thinking", map[string]any{"repaired": true, "iteration": iteration})
		}

		// Validate decision (schema, permissions, autonomy, limits, approval)
		if err := r.validateDecision(ctx, orgID, agent, task, decision, toolCallCount); err != nil {
			_ = r.repo.UpdateAgentRunStatus(orgID, run.ID, "failed", map[string]any{"error": err.Error()}, nil)
			_, _ = r.repo.UpdateTaskStatus(orgID, taskID, "failed", "agent", agentID, map[string]any{"error": err.Error()})
			broadcast("agent.failed", map[string]any{"error": err.Error(), "iteration": iteration})
			return err
		}

		// Execute actions
		outputs, _, err := r.executeDecisionActions(ctx, orgID, agent, task, project, run, decision, correlationID, broadcast)
		if err != nil {
			if decision.Status == "approval_required" || decision.Status == "waiting" {
				reason := "approval_required"
				if decision.Status != "" { reason = decision.Status }
				_ = r.repo.UpdateAgentRunWaiting(orgID, run.ID, "WAITING_FOR_APPROVAL", iteration, &reason, nil, map[string]any{"approval": decision.Status, "iteration": iteration})
				_ = r.repo.UpdateAgentRunStatus(orgID, run.ID, "awaiting_approval", map[string]any{"approval": decision.Status, "iteration": iteration}, nil)
				_, _ = r.repo.UpdateTaskStatus(orgID, taskID, "approval_required", "agent", agentID, map[string]any{"decision": decision.Status, "run_id": run.ID.String()})
				broadcast("agent.waiting", map[string]any{"reason": decision.Status, "iteration": iteration, "runId": run.ID.String()})
				return nil
			}
			return err
		}
		_ = r.repo.UpdateAgentRunWaiting(orgID, run.ID, "running", iteration, nil, nil, nil)
		previousOutputs = append(previousOutputs, outputs...)
		toolCallCount += countToolCalls(decision)

		// Check decision status
		switch decision.Status {
		case "completed":
			_ = r.repo.UpdateAgentRunStatus(orgID, run.ID, "succeeded", map[string]any{"response": decision.Response, "iteration": iteration}, nil)
			// Handle memory updates
			for _, mu := range decision.MemoryUpdates {
				_, _ = r.createMemoryFromUpdate(ctx, orgID, agentID, task.ProjectID, mu)
			}
			_, _ = r.repo.UpdateTaskStatus(orgID, taskID, "completed", "agent", agentID, map[string]any{"run_id": run.ID.String()})
			broadcast("agent.completed", map[string]any{"taskId": taskID.String(), "response": decision.Response, "iteration": iteration})
			if task.ChannelID != nil && decision.Response != "" {
				threadID := r.resolveThreadID(orgID, task)
				if msg, _ := r.repo.CreateMessage(orgID, *task.ChannelID, threadID, "agent", &agentID, security.SanitizeUserContent(decision.Response)); msg != nil {
					broadcast("agent.message", map[string]any{"message": msg, "channelId": task.ChannelID.String()})
				}
			}
			_, _ = r.repo.CreateTaskEvent(taskID, "agent", &agentID, "completed", map[string]any{"run_id": run.ID.String()}, correlationID)
			if task.ParentTaskID != nil {
				go func(pid uuid.UUID) {
					time.Sleep(500 * time.Millisecond)
					_ = r.CheckParentAggregation(context.Background(), orgID, pid)
				}(*task.ParentTaskID)
			}
			if r.autonomy != nil {
				go r.autonomy.HandleProjectEvent(context.Background(), orgID, task.ProjectID, "task.completed", map[string]any{"taskId": taskID.String(), "title": task.Title})
			}
			return nil
		case "failed":
			_ = r.repo.UpdateAgentRunStatus(orgID, run.ID, "failed", map[string]any{"response": decision.Response}, nil)
			_, _ = r.repo.UpdateTaskStatus(orgID, taskID, "failed", "agent", agentID, map[string]any{"response": decision.Response})
			broadcast("agent.failed", map[string]any{"response": decision.Response, "iteration": iteration})
			if r.autonomy != nil {
				go r.autonomy.HandleProjectEvent(context.Background(), orgID, task.ProjectID, "task.failed", map[string]any{"taskId": taskID.String(), "title": task.Title})
			}
			return fmt.Errorf("agent decided failed: %s", decision.Response)
		case "needs_human", "waiting", "approval_required":
			_ = r.repo.UpdateAgentRunStatus(orgID, run.ID, "awaiting_approval", map[string]any{"status": decision.Status, "response": decision.Response}, nil)
			_, _ = r.repo.UpdateTaskStatus(orgID, taskID, "waiting", "agent", agentID, map[string]any{"status": decision.Status})
			broadcast("agent.waiting", map[string]any{"reason": decision.Status, "response": decision.Response, "iteration": iteration})
			if decision.Response != "" && task.ChannelID != nil {
				threadID := r.resolveThreadID(orgID, task)
				if msg, _ := r.repo.CreateMessage(orgID, *task.ChannelID, threadID, "agent", &agentID, security.SanitizeUserContent(decision.Response)); msg != nil {
					broadcast("agent.message", map[string]any{"message": msg})
				}
			}
			return nil
		case "continue", "delegate":
			if toolCallCount > MaxToolCalls {
				_ = r.repo.UpdateAgentRunStatus(orgID, run.ID, "failed", map[string]any{"error": "max tool calls exceeded"}, nil)
				return fmt.Errorf("max tool calls %d exceeded", MaxToolCalls)
			}
			continue
		default:
			continue
		}
	}

	_ = r.repo.UpdateAgentRunStatus(orgID, run.ID, "failed", map[string]any{"error": "max iterations exceeded"}, nil)
	_, _ = r.repo.UpdateTaskStatus(orgID, taskID, "failed", "agent", agentID, map[string]any{"error": "max iterations"})
	broadcast("agent.failed", map[string]any{"error": "max iterations"})
	return fmt.Errorf("max iterations %d exceeded", MaxIterations)}

func min(a,b int) int { if a<b { return a }; return b }
func strPtr(s string) *string { return &s }
