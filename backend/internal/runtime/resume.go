package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"openagent/internal/domain"
	"openagent/internal/llm"
	"openagent/internal/security"
	"openagent/internal/ws"
)

// ResumeRun continues a WAITING_FOR_APPROVAL run in the same logical AgentRun (P0-1)
func (r *Runtime) ResumeRun(ctx context.Context, orgID, runID uuid.UUID) error {
	run, err := r.repo.GetAgentRun(orgID, runID)
	if err != nil {
		return err
	}
	if run.Status != "WAITING_FOR_APPROVAL" && run.Status != "awaiting_approval" && run.Status != "waiting" {
		return fmt.Errorf("run %s not waiting (status=%s)", runID, run.Status)
	}
	if run.TaskID == nil {
		return fmt.Errorf("run has no task")
	}
	task, err := r.repo.GetTask(orgID, *run.TaskID)
	if err != nil {
		return err
	}
	agent, err := r.repo.GetAgent(orgID, run.AgentID)
	if err != nil {
		return err
	}
	var project *domain.Project
	if task.ProjectID != (uuid.UUID{}) {
		if p, err := r.repo.GetProject(orgID, task.ProjectID); err == nil {
			project = p
		}
	}
	_ = r.repo.UpdateAgentRunStatus(orgID, run.ID, "running", map[string]any{"resumed": true}, nil)
	_ = r.repo.UpdateAgentRunWaiting(orgID, run.ID, "RESUMING", run.CurrentIteration, nil, nil, nil)
	broadcast := func(typ string, payload map[string]any) {
		if r.hub != nil {
			payload["agentId"] = run.AgentID.String()
			payload["taskId"] = task.ID.String()
			payload["runId"] = run.ID.String()
			if run.CorrelationID != nil {
				payload["correlationId"] = run.CorrelationID.String()
			}
			r.hub.BroadcastToOrg(orgID, typ, payload)
			if task.ChannelID != nil {
				r.hub.Broadcast(ws.Event{Type: typ, Payload: payload, Room: "channel:" + task.ChannelID.String()})
			}
		}
	}
	broadcast("agent.resumed", map[string]any{"runId": run.ID.String(), "iteration": run.CurrentIteration})
	var previousOutputs []map[string]any
	if execs, err := r.repo.ListToolExecutions(orgID, &run.AgentID, run.TaskID, 5); err == nil {
		for _, ex := range execs {
			if ex.Status == "succeeded" && ex.Output != nil {
				previousOutputs = append(previousOutputs, map[string]any{"action": "call_tool", "tool": ex.ToolName, "output": ex.Output, "resumed": true})
			} else if ex.Status == "failed" && ex.Error != nil {
				previousOutputs = append(previousOutputs, map[string]any{"action": "call_tool", "tool": ex.ToolName, "error": *ex.Error, "resumed": true})
			}
		}
	}
	ctx2, cancel := context.WithTimeout(ctx, ExecutionTimeout)
	defer cancel()
	for iteration := run.CurrentIteration + 1; iteration <= MaxIterations; iteration++ {
		select {
		case <-ctx2.Done():
			_ = r.repo.UpdateAgentRunStatus(orgID, run.ID, "cancelled", nil, nil)
			return context.Canceled
		default:
		}
		conversation := r.loadConversation(orgID, task.ChannelID, 5)
		sanitizedQuery := security.SanitizeUserContent(task.Title + " " + task.Description)
		var retrieved []domain.RetrievalResult
		if r.retriever != nil {
			filter := domain.RetrievalFilter{OrganizationID: orgID, AgentID: &run.AgentID, ProjectID: &task.ProjectID, Limit: 3}
			if res, err := r.retriever.SemanticSearch(ctx2, sanitizedQuery, filter); err == nil {
				retrieved = res
			}
		}
		var memories []*domain.Memory
		if r.memorySvc != nil {
			if mems, err := r.memorySvc.Retrieve(ctx2, orgID, &run.AgentID, &task.ProjectID, sanitizedQuery, 3, 0.3); err == nil {
				memories = mems
			}
		}
		contextStr := r.buildAgentContext(ctx2, agent, project, task, retrieved, memories, conversation, previousOutputs)
		model := agent.Model
		if agent.Version != nil && agent.Version.ModelConfiguration != nil {
			if m, ok := agent.Version.ModelConfiguration["model"].(string); ok && m != "" {
				model = m
			}
		}
		providerName := agent.Provider
		if strings.HasPrefix(model, "anthropic/") {
			providerName = "anthropic"
		} else if strings.HasPrefix(model, "openai/") {
			providerName = "openai"
		} else if strings.HasPrefix(model, "google/") {
			providerName = "google"
		} else if strings.Contains(model, "/") {
			providerName = "openrouter"
		}
		var provider llm.Provider
		if providerName != "" {
			if p, err := r.llmReg.Get(providerName); err == nil {
				provider = p
			}
		}
		if provider == nil {
			if p, err := r.llmReg.Get("openrouter"); err == nil {
				provider = p
			}
		}
		if provider == nil {
			if p, err := r.llmReg.Get("stub"); err == nil {
				provider = p
			}
		}
		if provider == nil {
			provider = llm.NewStub("stub")
		}
		hardenedSystem := security.SystemPromptHardening()
		instructions := ""
		if agent.Version != nil {
			instructions = agent.Version.Instructions
		}
		if instructions == "" {
			instructions = agent.SystemPrompt
		}
		if instructions == "" {
			instructions = "You are a helpful assistant. Complete the assigned task."
		}
		hardenedSystem += instructions
		toolDefs := r.getToolDefinitionsForAgent(orgID, run.AgentID)
		messages := []llm.Message{
			{Role: llm.RoleSystem, Content: hardenedSystem},
			{Role: llm.RoleUser, Content: contextStr + "\n\nRespond ONLY with valid JSON matching AgentDecision schema:\n" + "{\"status\":\"continue|delegate|waiting|approval_required|needs_human|completed|failed\",\"response\":\"summary\",\"actions\":[]}\n" + "Previous tool results: " + fmt.Sprintf("%v", previousOutputs)},
		}
		for _, out := range previousOutputs {
			messages = append(messages, llm.Message{Role: llm.RoleUser, Content: fmt.Sprintf("Previous tool/output %v", out)})
		}
		if len(toolDefs) > 0 {
			messages[1].Content += fmt.Sprintf("\nAvailable tools: %v", toolDefs)
		}
		_, _ = r.repo.CreateAgentAction(run.ID, (iteration-1)*10, "llm_call", nil, map[string]any{"messages": messages, "model": model, "iteration": iteration}, nil, "running", run.CorrelationID)
		broadcast("agent.thinking", map[string]any{"iteration": iteration, "resumed": true})
		llmResp, err := provider.Complete(ctx2, llm.CompletionRequest{Model: model, Messages: messages})
		if err != nil {
			errStr := err.Error()
			_ = r.repo.UpdateAgentRunStatus(orgID, run.ID, "failed", map[string]any{"error": errStr}, &errStr)
			_, _ = r.repo.UpdateTaskStatus(orgID, task.ID, "failed", "agent", run.AgentID, map[string]any{"error": errStr})
			broadcast("agent.failed", map[string]any{"error": errStr})
			return err
		}
		_, _ = r.repo.CreateAgentAction(run.ID, (iteration-1)*10+1, "llm_call", nil, map[string]any{"response": llmResp.Content, "iteration": iteration, "usage": llmResp.Usage}, nil, "succeeded", run.CorrelationID)
		_ = r.repo.UpdateAgentRunTokens(orgID, run.ID, llmResp.Usage)
		decision, err := ParseAgentDecision(llmResp.Content)
		if err != nil {
			for retry := 1; retry <= MaxDecisionRetries && err != nil; retry++ {
				repairPrompt := fmt.Sprintf("Validation failed: %s — Provide valid AgentDecision JSON only.", err.Error())
				repairMessages := []llm.Message{{Role: llm.RoleSystem, Content: hardenedSystem}, {Role: llm.RoleUser, Content: repairPrompt}}
				repairResp, repErr := provider.Complete(ctx2, llm.CompletionRequest{Model: model, Messages: repairMessages})
				if repErr != nil {
					err = repErr
					break
				}
				decision, err = ParseAgentDecision(repairResp.Content)
			}
			if err != nil {
				errStr := fmt.Sprintf("invalid decision after retries: %v", err)
				_ = r.repo.UpdateAgentRunStatus(orgID, run.ID, "failed", map[string]any{"error": errStr}, &errStr)
				return err
			}
		}
		if err := r.validateDecision(ctx2, orgID, agent, task, decision, len(previousOutputs)); err != nil {
			_ = r.repo.UpdateAgentRunStatus(orgID, run.ID, "failed", map[string]any{"error": err.Error()}, nil)
			return err
		}
		outputs, _, err := r.executeDecisionActions(ctx2, orgID, agent, task, project, run, decision, run.CorrelationID, broadcast)
		if err != nil {
			if decision.Status == "approval_required" || decision.Status == "waiting" {
				reason := decision.Status
				_ = r.repo.UpdateAgentRunWaiting(orgID, run.ID, "WAITING_FOR_APPROVAL", iteration, &reason, nil, map[string]any{"approval": decision.Status})
				return nil
			}
			return err
		}
		previousOutputs = append(previousOutputs, outputs...)
		if decision.Status == "completed" {
			_ = r.repo.UpdateAgentRunStatus(orgID, run.ID, "succeeded", map[string]any{"response": decision.Response}, nil)
			_, _ = r.repo.UpdateTaskStatus(orgID, task.ID, "completed", "agent", run.AgentID, nil)
			broadcast("agent.completed", map[string]any{"taskId": task.ID.String()})
			return nil
		}
		if decision.Status == "failed" {
			_ = r.repo.UpdateAgentRunStatus(orgID, run.ID, "failed", map[string]any{"response": decision.Response}, nil)
			_, _ = r.repo.UpdateTaskStatus(orgID, task.ID, "failed", "agent", run.AgentID, nil)
			return nil
		}
		_ = r.repo.UpdateAgentRunWaiting(orgID, run.ID, "running", iteration, nil, nil, nil)
	}
	_ = r.repo.UpdateAgentRunStatus(orgID, run.ID, "failed", map[string]any{"error": "max iterations"}, nil)
	return fmt.Errorf("max iterations")
}
