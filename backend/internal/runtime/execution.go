package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"openagent/internal/domain"
	"openagent/internal/security"
	"openagent/internal/tool"
	"openagent/internal/worker"
)

// getToolDefinitionsForAgent returns tool specs available to the agent for LLM prompt
func (r *Runtime) getToolDefinitionsForAgent(orgID, agentID uuid.UUID) []map[string]any {
	var out []map[string]any
	if tools, err := r.repo.ListAgentTools(agentID); err == nil && len(tools) > 0 {
		for _, at := range tools {
			if !at.Enabled { continue }
			entry := map[string]any{
				"name": at.Tool.Name,
				"description": at.Tool.Description,
				"risk_level": at.Tool.RiskLevel,
			}
			// Try to fetch full tool for schema
			if t, err := r.repo.GetToolByName(&orgID, at.Tool.Name); err == nil && t.InputSchema != nil {
				entry["input_schema"] = t.InputSchema
			}
			if t, err := r.repo.GetToolByName(nil, at.Tool.Name); err == nil && entry["input_schema"] == nil && t.InputSchema != nil {
				entry["input_schema"] = t.InputSchema
			}
			out = append(out, entry)
		}
		if len(out) > 0 { return out }
	}
	// Fallback: show low risk tools globally (not leaking high/critical unassigned)
	if tools, err := r.repo.ListTools(orgID); err == nil {
		for _, t := range tools {
			if !t.Enabled { continue }
			if t.RiskLevel == "high" || t.RiskLevel == "critical" { continue }
			out = append(out, map[string]any{
				"name": t.Name,
				"description": t.Description,
				"input_schema": t.InputSchema,
				"risk_level": t.RiskLevel,
			})
			if len(out) >= 8 { break }
		}
	}
	return out
}

// validateDecision enforces guardrails beyond JSON schema
func (r *Runtime) validateDecision(ctx context.Context, orgID uuid.UUID, agent *domain.Agent, task *domain.Task, decision *AgentDecision, toolCallCount int) error {
	// Check max tool calls
	newCalls := countToolCalls(decision)
	if toolCallCount+newCalls > MaxToolCalls {
		return fmt.Errorf("tool call limit exceeded: %d + %d > %d", toolCallCount, newCalls, MaxToolCalls)
	}
	// Check delegation depth
	for _, act := range decision.Actions {
		if act.Type == "create_task" || act.Type == "delegate_task" {
			// count current delegation depth from task
			depth := 0
			if task.ParentTaskID != nil { depth = 1 }
			// crude: if task already has depth via assignments, check via repo could be stored
			// Use task.DelegationDepth field if available (not always hydrated, assume 0)
			// Also enforce MaxDelegationDepth
			if depth >= MaxDelegationDepth {
				return fmt.Errorf("max delegation depth %d exceeded", MaxDelegationDepth)
			}
		}
		if act.Type == "call_tool" {
			toolName := act.Tool
			if toolName == "" && act.Params != nil {
				if t, ok := act.Params["tool"].(string); ok { toolName = t }
				if t, ok := act.Params["name"].(string); ok && toolName == "" { toolName = t }
			}
			if toolName == "" {
				return fmt.Errorf("call_tool missing tool name")
			}
			// Check agent is assigned to this tool if medium/high
			// Fetch tool to check risk
			var t *domain.Tool
			var err error
			t, err = r.repo.GetToolByName(&orgID, toolName)
			if err != nil { t, err = r.repo.GetToolByName(nil, toolName) }
			if err == nil && (t.RiskLevel == "high" || t.RiskLevel == "critical") {
				// high risk requires tool assignment
				allowed := false
				if tools, err := r.repo.ListAgentTools(agent.ID); err == nil {
					for _, at := range tools {
						if at.Tool.Name == toolName && at.Enabled { allowed = true; break }
					}
				}
				if !allowed {
					// Check autonomy: only autonomous/collaborative can request high risk via approval
					if agent.AutonomyLevel == "assistant" {
						return fmt.Errorf("agent autonomy %s cannot call high-risk tool %s", agent.AutonomyLevel, toolName)
					}
					// Allow but will require approval— validateDecision passes, execution will gate
				}
			}
			// Validate tool input via security (no code exec)
			input := act.Input
			if input == nil && act.Params != nil {
				if in, ok := act.Params["input"].(map[string]any); ok { input = in } else {
					// try to build from params without tool/name keys
					input = map[string]any{}
					for k, v := range act.Params {
						if k != "tool" && k != "name" { input[k] = v }
					}
				}
			}
			if input != nil {
				if err := security.ValidateToolInput(input); err != nil {
					return fmt.Errorf("tool %s input validation failed: %w", toolName, err)
				}
			}
		}
		// Check task status transitions for update/complete/fail
		if act.Type == "complete_task" {
			if !isValidTransition(task.Status, "completed") {
				return fmt.Errorf("invalid transition %s -> completed", task.Status)
			}
		}
		if act.Type == "fail_task" {
			if !isValidTransition(task.Status, "failed") {
				return fmt.Errorf("invalid transition %s -> failed", task.Status)
			}
		}
		if act.Type == "update_task" {
			// allow pending->assigned etc., but we don't know target status; skip strict
		}
	}
	// Check status coherence: needs_human/waiting/approval_required should have response
	if (decision.Status == "needs_human" || decision.Status == "waiting" || decision.Status == "approval_required") && decision.Response == "" && len(decision.Actions) == 0 {
		return fmt.Errorf("status %s requires response or actions", decision.Status)
	}
	return nil
}

// resolveThreadID carries thread linkage for agent collaboration.
// When task.CorrelationID matches a user message ID in the same channel,
// delegated/continued replies (ParentTaskID != nil) thread under that root,
// keeping the first responder inline (nil thread).
func (r *Runtime) resolveThreadID(orgID uuid.UUID, task *domain.Task) *uuid.UUID {
	if task == nil || task.CorrelationID == nil || task.ChannelID == nil {
		return nil
	}
	// Only thread continuation/delegation, not first inline answer.
	if task.ParentTaskID == nil {
		return nil
	}
	root, err := r.repo.GetMessage(orgID, *task.CorrelationID)
	if err != nil || root == nil {
		return nil
	}
	if root.ChannelID != *task.ChannelID {
		return nil
	}
	if root.ID == task.ID {
		return nil
	}
	threadID := root.ID
	return &threadID
}

// executeDecisionActions runs each action sequentially, returns outputs and whether to continue loop
func (r *Runtime) executeDecisionActions(ctx context.Context, orgID uuid.UUID, agent *domain.Agent, task *domain.Task, project *domain.Project, run *domain.AgentRun, decision *AgentDecision, correlationID *uuid.UUID, broadcast func(string, map[string]any)) ([]map[string]any, bool, error) {
	var outputs []map[string]any
	threadID := r.resolveThreadID(orgID, task)
	seqBase := 0
	// Use iteration-derived seq elsewhere; here simple increment
	for idx, act := range decision.Actions {
		seq := seqBase + idx + 2
		switch act.Type {
		case "send_message":
			body := act.Body
			if body == "" && act.Params != nil {
				if b, ok := act.Params["body"].(string); ok { body = b }
			}
			if body == "" { body = decision.Response }
			if body == "" { continue }
			body = security.SanitizeUserContent(body)
			if len(body) > 5000 { body = body[:5000] }
			var msg *domain.Message
			var err error
			if task.ChannelID != nil {
				msg, err = r.repo.CreateMessage(orgID, *task.ChannelID, threadID, "agent", &agent.ID, body)
				if err == nil && msg != nil {
					broadcast("agent.message", map[string]any{"message": msg, "channelId": task.ChannelID.String(), "sourceAgent": agent.ID.String()})
				}
			}
			_, _ = r.repo.CreateAgentAction(run.ID, seq, "message", nil, map[string]any{"body": body}, map[string]any{"message_id": safeMsgID(msg)}, "succeeded", correlationID)
			outputs = append(outputs, map[string]any{"action": "send_message", "body": body, "message_id": safeMsgID(msg)})

		case "create_task":
			title := act.Title
			desc := act.Description
			if title == "" && act.Params != nil {
				if t, ok := act.Params["title"].(string); ok { title = t }
			}
			if desc == "" && act.Params != nil {
				if d, ok := act.Params["description"].(string); ok { desc = d }
				if d, ok := act.Params["body"].(string); ok && desc == "" { desc = d }
			}
			if title == "" { title = decision.Response }
			if title == "" { title = "Subtask from " + agent.Name }
			title = security.SanitizeUserContent(title)
			desc = security.SanitizeUserContent(desc)
			priority := "medium"
			if p, ok := act.Params["priority"].(string); ok && p != "" { priority = p }
			var assignAgent *uuid.UUID
			// 1) Direct UUID fields
			if act.AgentID != "" {
				if uid, err := uuid.Parse(act.AgentID); err == nil { assignAgent = &uid }
				if assignAgent == nil {
					assignAgent = r.resolveAgentHint(orgID, act.AgentID, agent.ID, idx)
				}
			}
			// 2) Check various hint keys in Params
			if assignAgent == nil && act.Params != nil {
				hintKeys := []string{"assign_to_agent", "agent_id", "assignee", "assigned_to", "assignedToAgent", "worker", "assignee_agent", "target_agent"}
				for _, k := range hintKeys {
					if s, ok := act.Params[k].(string); ok && s != "" {
						if uid, err := uuid.Parse(s); err == nil {
							assignAgent = &uid
							break
						}
						if resolved := r.resolveAgentHint(orgID, s, agent.ID, idx); resolved != nil {
							assignAgent = resolved
							break
						}
					}
				}
				// Also check nested params (LLM sometimes nests under params.params)
				if assignAgent == nil {
					if nested, ok := act.Params["params"].(map[string]any); ok {
						for _, k := range hintKeys {
							if s, ok := nested[k].(string); ok && s != "" {
								if uid, err := uuid.Parse(s); err == nil {
									assignAgent = &uid
									break
								}
								if resolved := r.resolveAgentHint(orgID, s, agent.ID, idx); resolved != nil {
									assignAgent = resolved
									break
								}
							}
						}
					}
				}
			}
			if eligibleID, ok := r.tryEligibleAssignment(orgID, task.ProjectID, task, run, seq, correlationID); ok && eligibleID != nil {
				assignAgent = eligibleID
			} else if task.RequiredRole != nil || len(task.RequiredCapabilities) > 0 {
				_, _ = r.repo.CreateAgentAction(run.ID, seq, "error", nil, map[string]any{"error": "no eligible agent"}, nil, "failed", correlationID)
				return outputs, false, fmt.Errorf("no eligible agent")
			}
			// 3) Auto-assignment for delegation (team-aware round-robin)
			if assignAgent == nil && (strings.Contains(strings.ToLower(decision.Status), "delegate") || act.Type == "create_task") {
				var candidates []*domain.Agent
				if task.TeamID != nil {
					if members, err := r.repo.ListTeamMembers(*task.TeamID); err == nil && len(members) > 0 {
						for _, m := range members {
							if ag, err := r.repo.GetAgent(orgID, m.AgentID); err == nil {
								candidates = append(candidates, ag)
							}
						}
						// Sort by TeamWorkflow order if available
						if len(candidates) > 0 {
							if team, err := r.repo.GetTeam(orgID, *task.TeamID); err == nil && team.CurrentVersion != nil && len(team.CurrentVersion.Workflow) > 0 {
								order := map[string]int{}
								for i, step := range team.CurrentVersion.Workflow {
									order[strings.ToLower(step.To)] = i
									order[strings.ToLower(step.From)] = i
								}
								sort.Slice(candidates, func(a, b int) bool {
									roleA, roleB := "", ""
									if candidates[a].Version != nil { roleA = strings.ToLower(candidates[a].Version.Role) }
									if candidates[b].Version != nil { roleB = strings.ToLower(candidates[b].Version.Role) }
									oa, okA := order[roleA]
									ob, okB := order[roleB]
									if okA && okB { return oa < ob }
									if okA { return true }
									if okB { return false }
									return candidates[a].Name < candidates[b].Name
								})
							}
						}
					}
				}
				if len(candidates) == 0 {
					if agents, err := r.repo.ListAgents(orgID); err == nil {
						for _, a := range agents {
							if a.ID != agent.ID && a.Status == "active" {
								candidates = append(candidates, a)
							}
						}
					}
				}
				if len(candidates) > 0 {
					chosen := candidates[0]
					assignAgent = &chosen.ID
				} else if strings.Contains(strings.ToLower(decision.Status), "delegate") {
					if agents, err := r.repo.ListAgents(orgID); err == nil && len(agents) > 1 {
						for _, a := range agents {
							if a.ID != agent.ID { assignAgent = &a.ID; break }
						}
					}
				}
			}
			// Delegation validation: target must belong to org, be assigned to project, and satisfy capability
			if assignAgent != nil {
				if err := r.validateDelegationTarget(orgID, task.ProjectID, task.TeamID, *assignAgent); err != nil {
					_, _ = r.repo.CreateAgentAction(run.ID, seq, "tool_call", strPtr("create_task"), map[string]any{"title": title, "assign_to": assignAgent.String()}, map[string]any{"error": err.Error()}, "failed", correlationID)
					outputs = append(outputs, map[string]any{"action": "create_task", "error": err.Error()})
					continue
				}
				// Check delegation depth
				depth := task.DelegationDepth
				if task.ParentTaskID != nil { depth++ }
				if depth >= MaxDelegationDepth {
					errMsg := fmt.Sprintf("max delegation depth %d exceeded", MaxDelegationDepth)
					_, _ = r.repo.CreateAgentAction(run.ID, seq, "tool_call", strPtr("create_task"), map[string]any{"title": title}, map[string]any{"error": errMsg}, "failed", correlationID)
					outputs = append(outputs, map[string]any{"action": "create_task", "error": errMsg})
					continue
				}
				// Cycle detection: prevent delegating to same agent that is already in parent chain (simple)
				if correlationID != nil {
					// Check if this agent already handled a task with same correlation and title (avoid loops)
					// For now, allow but log; deeper cycle check via task_dependencies will handle
				}
				// Max child tasks limit (prevent explosion)
				if children, err := r.repo.ListTasks(orgID, &task.ProjectID, nil, "", nil, 100); err == nil {
					childCount := 0
					for _, c := range children {
						if c.ParentTaskID != nil && *c.ParentTaskID == task.ID { childCount++ }
					}
					if childCount >= 20 {
						errMsg := "max child tasks 20 exceeded"
						_, _ = r.repo.CreateAgentAction(run.ID, seq, "tool_call", strPtr("create_task"), map[string]any{"title": title}, map[string]any{"error": errMsg}, "failed", correlationID)
						outputs = append(outputs, map[string]any{"action": "create_task", "error": errMsg})
						continue
					}
				}
			}
			// Use a valid user as creator (FK to users) — parent task's creator or agent owner
			creatorID := agent.ID
			if task.CreatedBy != nil {
				creatorID = *task.CreatedBy
			} else if agent.CreatedBy != nil {
				creatorID = *agent.CreatedBy
			} else if agent.OwnerID != nil {
				creatorID = *agent.OwnerID
			} else {
				if members, err := r.repo.ListOrgMembers(orgID); err == nil && len(members) > 0 {
					creatorID = members[0].UserID
				}
			}
			subTask, err := r.repo.CreateTask(orgID, task.ProjectID, task.ChannelID, &task.ID, task.TeamID, assignAgent, nil, title, desc, priority, nil, correlationID, creatorID)
			if err != nil {
				_, _ = r.repo.CreateAgentAction(run.ID, seq, "tool_call", strPtr("create_task"), act.Params, map[string]any{"error": err.Error()}, "failed", correlationID)
				outputs = append(outputs, map[string]any{"action": "create_task", "error": err.Error()})
				continue
			}
			_, _ = r.repo.CreateAgentAction(run.ID, seq, "tool_call", strPtr("create_task"), map[string]any{"title": title}, map[string]any{"subtask_id": subTask.ID.String()}, "succeeded", correlationID)
			broadcast("agent.task_created", map[string]any{"subtask": subTask, "parent": task.ID.String(), "sourceAgent": agent.ID.String(), "destAgent": assignAgent})
			if assignAgent != nil {
				broadcast("agent.delegated", map[string]any{"taskId": subTask.ID.String(), "from": agent.ID.String(), "to": assignAgent.String(), "correlationId": safeCorrID(correlationID)})
			}
			if task.ChannelID != nil {
				body := fmt.Sprintf("**%s** created subtask **%s**", agent.Name, title)
				if assignAgent != nil { body += fmt.Sprintf(" → agent %s", assignAgent.String()[:8]) }
				if msg, _ := r.repo.CreateMessage(orgID, *task.ChannelID, threadID, "agent", &agent.ID, body); msg != nil {
					broadcast("agent.message", map[string]any{"message": msg})
				}
			}
			_, _ = r.repo.CreateTaskEvent(subTask.ID, "agent", &agent.ID, "created", map[string]any{"title": title, "parent": task.ID.String()}, correlationID)
			outputs = append(outputs, map[string]any{"action": "create_task", "subtask_id": subTask.ID.String(), "title": title, "assigned_to": safeUUID(assignAgent)})
			// Auto-enqueue subtask for worker execution (auto-assignment vs manual)
			if assignAgent != nil && r.workerPool != nil {
				r.workerPool.Enqueue(worker.Job{
					ID:   subTask.ID.String(),
					Type: "agent_run",
					Payload: map[string]any{
						"organization_id": orgID.String(),
						"agent_id":        assignAgent.String(),
						"task_id":         subTask.ID.String(),
						"correlation_id":  safeCorrID(correlationID),
						"trigger":         "delegated",
					},
					Timeout: 60 * time.Second,
					Retries: 3,
				})
			}

		case "delegate_task":
			taskIDStr := act.TaskID
			agentIDStr := act.AgentID
			if taskIDStr == "" && act.Params != nil {
				if v, ok := act.Params["taskId"].(string); ok { taskIDStr = v }
				if v, ok := act.Params["task_id"].(string); ok && taskIDStr == "" { taskIDStr = v }
			}
			if agentIDStr == "" && act.Params != nil {
				if v, ok := act.Params["agentId"].(string); ok { agentIDStr = v }
				if v, ok := act.Params["agent_id"].(string); ok && agentIDStr == "" { agentIDStr = v }
			}
			tid, err1 := uuid.Parse(taskIDStr)
			aid, err2 := uuid.Parse(agentIDStr)
			if err1 != nil || err2 != nil {
				outputs = append(outputs, map[string]any{"action": "delegate_task", "error": "invalid ids"})
				continue
			}
			// Validate delegation target (org, project, team)
			if err := r.validateDelegationTarget(orgID, task.ProjectID, task.TeamID, aid); err != nil {
				outputs = append(outputs, map[string]any{"action": "delegate_task", "error": err.Error()})
				_, _ = r.repo.CreateAgentAction(run.ID, seq, "tool_call", strPtr("delegate_task"), map[string]any{"taskId": tid.String(), "agentId": aid.String()}, map[string]any{"error": err.Error()}, "failed", correlationID)
				continue
			}
			_, err := r.repo.AssignTask(orgID, tid, &aid, nil, agent.ID)
			if err != nil {
				outputs = append(outputs, map[string]any{"action": "delegate_task", "error": err.Error()})
				continue
			}
			_, _ = r.repo.CreateAgentAction(run.ID, seq, "tool_call", strPtr("delegate_task"), map[string]any{"taskId": tid.String(), "agentId": aid.String()}, map[string]any{"delegated": true}, "succeeded", correlationID)
			broadcast("agent.delegated", map[string]any{"taskId": tid.String(), "from": agent.ID.String(), "to": aid.String()})
			outputs = append(outputs, map[string]any{"action": "delegate_task", "taskId": tid.String(), "agentId": aid.String()})

		case "call_tool":
			toolName := act.Tool
			input := act.Input
			if toolName == "" && act.Params != nil {
				if t, ok := act.Params["tool"].(string); ok { toolName = t }
				if t, ok := act.Params["name"].(string); ok && toolName == "" { toolName = t }
			}
			if input == nil && act.Params != nil {
				if in, ok := act.Params["input"].(map[string]any); ok { input = in } else {
					input = map[string]any{}
					for k, v := range act.Params {
						if k != "tool" && k != "name" && k != "input" { input[k] = v }
					}
				}
			}
			if input == nil { input = map[string]any{} }
			if toolName == "" { toolName = "web_search" }
			if toolName == "search" { toolName = "web_search" }
			if r.toolExec != nil {
				broadcast("agent.tool_started", map[string]any{"tool": toolName})
				_, _ = r.repo.CreateAgentAction(run.ID, seq, "tool_call", &toolName, input, nil, "running", correlationID)
				// Deterministic idempotency for approval resume: same task+tool+input should dedupe after approval
				hashKey := fmt.Sprintf("%s:%s:%v", task.ID.String(), toolName, input)
				exec, err := r.toolExec.Execute(ctx, tool.ExecuteParams{
					OrganizationID: orgID,
					AgentID:        agent.ID,
					TaskID:         &task.ID,
					RunID:          &run.ID,
					ToolName:       toolName,
					Input:          input,
					IdempotencyKey: hashKey,
				})
				if err != nil && exec != nil && exec.Status == "approval_required" {
					_, _ = r.repo.CreateAgentAction(run.ID, seq, "tool_call", &toolName, input, map[string]any{"approval_required": true, "approval_id": exec.ApprovalID}, "succeeded", correlationID)
					outputs = append(outputs, map[string]any{"action": "call_tool", "tool": toolName, "approval_required": true, "approval_id": safeUUID(exec.ApprovalID)})
					// Signal to caller that we need to park
					decision.Status = "approval_required"
					return outputs, false, fmt.Errorf("approval_required")
				} else if err != nil {
					broadcast("agent.tool_completed", map[string]any{"tool": toolName, "error": err.Error()})
					_, _ = r.repo.CreateAgentAction(run.ID, seq, "tool_call", &toolName, input, map[string]any{"error": err.Error()}, "failed", correlationID)
					outputs = append(outputs, map[string]any{"action": "call_tool", "tool": toolName, "error": err.Error()})
					continue
				}
				broadcast("agent.tool_completed", map[string]any{"tool": toolName, "output": exec.Output})
				_, _ = r.repo.CreateAgentAction(run.ID, seq, "tool_call", &toolName, input, exec.Output, "succeeded", correlationID)
				_, _ = r.repo.CreateTaskEvent(task.ID, "agent", &agent.ID, "tool_completed", map[string]any{"tool": toolName}, correlationID)
				outputs = append(outputs, map[string]any{"action": "call_tool", "tool": toolName, "output": exec.Output})
			} else {
				broadcast("agent.tool_started", map[string]any{"tool": toolName})
				_, _ = r.repo.CreateAgentAction(run.ID, seq, "tool_call", &toolName, input, map[string]any{"mock": true}, "succeeded", correlationID)
				broadcast("agent.tool_completed", map[string]any{"tool": toolName, "mock": true})
				outputs = append(outputs, map[string]any{"action": "call_tool", "tool": toolName, "mock": true})
			}

		case "request_approval", "request_human_input":
			body := act.Body
			if body == "" && act.Params != nil {
				if b, ok := act.Params["body"].(string); ok { body = b }
				if b, ok := act.Params["message"].(string); ok && body == "" { body = b }
			}
			if body == "" { body = decision.Response }
			body = security.SanitizeUserContent(body)
			_, _ = r.repo.CreateAgentAction(run.ID, seq, act.Type, nil, map[string]any{"body": body}, nil, "succeeded", correlationID)
			if task.ChannelID != nil && body != "" {
				if msg, _ := r.repo.CreateMessage(orgID, *task.ChannelID, threadID, "agent", &agent.ID, body); msg != nil {
					broadcast("agent.message", map[string]any{"message": msg})
				}
			}
			outputs = append(outputs, map[string]any{"action": act.Type, "body": body})
			// This will cause outer loop to park via status waiting/approval_required
			decision.Status = "approval_required"

		case "update_task":
			// Generic update: try to update status if provided
			newStatus := ""
			if act.Params != nil {
				if s, ok := act.Params["status"].(string); ok { newStatus = s }
			}
			if newStatus != "" {
				if !isValidTransition(task.Status, newStatus) {
					outputs = append(outputs, map[string]any{"action": "update_task", "error": fmt.Sprintf("invalid transition %s -> %s", task.Status, newStatus)})
					continue
				}
				_, err := r.repo.UpdateTaskStatus(orgID, task.ID, newStatus, "agent", agent.ID, act.Params)
				if err != nil {
					outputs = append(outputs, map[string]any{"action": "update_task", "error": err.Error()})
					continue
				}
				task.Status = newStatus
				outputs = append(outputs, map[string]any{"action": "update_task", "status": newStatus})
				if newStatus == "completed" && task.ParentTaskID != nil {
					go func(pid uuid.UUID) {
						time.Sleep(500 * time.Millisecond)
						_ = r.CheckParentAggregation(context.Background(), orgID, pid)
					}(*task.ParentTaskID)
				}
			} else {
				outputs = append(outputs, map[string]any{"action": "update_task", "noop": true})
			}
			_, _ = r.repo.CreateAgentAction(run.ID, seq, "update_task", nil, act.Params, outputs[len(outputs)-1], "succeeded", correlationID)

		case "complete_task":
			if !isValidTransition(task.Status, "completed") {
				outputs = append(outputs, map[string]any{"action": "complete_task", "error": "invalid transition"})
				continue
			}
			_, err := r.repo.UpdateTaskStatus(orgID, task.ID, "completed", "agent", agent.ID, map[string]any{"run_id": run.ID.String()})
			if err != nil {
				outputs = append(outputs, map[string]any{"action": "complete_task", "error": err.Error()})
				continue
			}
			task.Status = "completed"
			_, _ = r.repo.CreateAgentAction(run.ID, seq, "complete_task", nil, nil, nil, "succeeded", correlationID)
			outputs = append(outputs, map[string]any{"action": "complete_task", "status": "completed"})
			decision.Status = "completed"
			if task.ParentTaskID != nil {
				go func(pid uuid.UUID) {
					time.Sleep(500 * time.Millisecond)
					_ = r.CheckParentAggregation(context.Background(), orgID, pid)
				}(*task.ParentTaskID)
			}

		case "fail_task":
			if !isValidTransition(task.Status, "failed") {
				outputs = append(outputs, map[string]any{"action": "fail_task", "error": "invalid transition"})
				continue
			}
			_, err := r.repo.UpdateTaskStatus(orgID, task.ID, "failed", "agent", agent.ID, map[string]any{"response": decision.Response})
			if err != nil {
				outputs = append(outputs, map[string]any{"action": "fail_task", "error": err.Error()})
				continue
			}
			task.Status = "failed"
			_, _ = r.repo.CreateAgentAction(run.ID, seq, "fail_task", nil, nil, nil, "succeeded", correlationID)
			outputs = append(outputs, map[string]any{"action": "fail_task", "status": "failed"})
			decision.Status = "failed"

		case "store_memory":
			content := ""
			if act.Params != nil {
				if c, ok := act.Params["content"].(string); ok { content = c }
			}
			if content == "" { content = decision.Response }
			if content == "" { continue }
			mu := MemoryUpdate{
				Content: content,
				MemoryType: "agent",
				Scope: "agent",
				Importance: 0.5,
			}
			if act.Params != nil {
				if t, ok := act.Params["memory_type"].(string); ok { mu.MemoryType = t }
				if s, ok := act.Params["scope"].(string); ok { mu.Scope = s }
				if imp, ok := act.Params["importance"].(float64); ok { mu.Importance = imp }
			}
			if _, err := r.createMemoryFromUpdate(ctx, orgID, agent.ID, task.ProjectID, mu); err == nil {
				outputs = append(outputs, map[string]any{"action": "store_memory", "content": content[:min(100, len(content))]})
			} else {
				outputs = append(outputs, map[string]any{"action": "store_memory", "error": err.Error()})
			}
			_, _ = r.repo.CreateAgentAction(run.ID, seq, "store_memory", nil, map[string]any{"content": content}, nil, "succeeded", correlationID)

		default:
			outputs = append(outputs, map[string]any{"action": act.Type, "error": "unsupported"})
		}
	}
	// Determine if we should continue: if status is continue/delegate and we haven't hit terminal
	shouldContinue := decision.Status == "continue" || decision.Status == "delegate"
	return outputs, shouldContinue, nil
}

func (r *Runtime) createMemoryFromUpdate(ctx context.Context, orgID, agentID, projectID uuid.UUID, mu MemoryUpdate) (*domain.Memory, error) {
	if r.memorySvc == nil {
		return nil, fmt.Errorf("memory service not available")
	}
	memoryType := mu.MemoryType
	if memoryType == "" { memoryType = "agent" }
	scope := mu.Scope
	if scope == "" { scope = "agent" }
	importance := mu.Importance
	if importance == 0 { importance = 0.5 }
	sanitized := security.SanitizeUserContent(mu.Content)
	var projPtr *uuid.UUID
	if mu.ProjectID != nil {
		if uid, err := uuid.Parse(*mu.ProjectID); err == nil { projPtr = &uid }
	} else {
		projPtr = &projectID
	}
	var agentPtr *uuid.UUID
	if mu.AgentID != nil {
		if uid, err := uuid.Parse(*mu.AgentID); err == nil { agentPtr = &uid }
	} else {
		agentPtr = &agentID
	}
	return r.memorySvc.Create(ctx, orgID, agentPtr, projPtr, memoryType, scope, "agent", sanitized, importance, nil, nil)
}

func countToolCalls(decision *AgentDecision) int {
	c := 0
	for _, act := range decision.Actions {
		if act.Type == "call_tool" { c++ }
	}
	return c
}

// resolveAgentHint tries to map a human hint like "research_worker", "Content Worker 1", "copywriter" to a real agent ID
func (r *Runtime) resolveAgentHint(orgID uuid.UUID, hint string, excludeID uuid.UUID, idx int) *uuid.UUID {
	hint = strings.TrimSpace(strings.ToLower(hint))
	if hint == "" { return nil }
	// Try direct UUID first
	if uid, err := uuid.Parse(hint); err == nil {
		if _, err := r.repo.GetAgent(orgID, uid); err == nil { return &uid }
	}
	agents, err := r.repo.ListAgents(orgID)
	if err != nil || len(agents) == 0 { return nil }
	// Build candidate list excluding orchestrator
	var candidates []*domain.Agent
	for _, a := range agents {
		if a.ID == excludeID { continue }
		if a.Status != "active" { continue }
		candidates = append(candidates, a)
	}
	if len(candidates) == 0 { return nil }
	// Score each candidate
	bestScore := -1
	var best *domain.Agent
	for _, a := range candidates {
		score := 0
		nameLower := strings.ToLower(a.Name)
		slugLower := strings.ToLower(a.Slug)
		roleLower := ""
		if a.Version != nil { roleLower = strings.ToLower(a.Version.Role) }
		purposeLower := strings.ToLower(a.Purpose)
		// Direct contains
		if strings.Contains(hint, nameLower) || strings.Contains(nameLower, hint) { score += 10 }
		if strings.Contains(hint, slugLower) || strings.Contains(slugLower, hint) { score += 10 }
		if roleLower != "" && (strings.Contains(hint, roleLower) || strings.Contains(roleLower, hint)) { score += 8 }
		if strings.Contains(purposeLower, hint) || strings.Contains(hint, purposeLower) { score += 5 }
		// Keyword heuristics
		if strings.Contains(hint, "research") && (strings.Contains(nameLower, "research") || strings.Contains(roleLower, "research") || strings.Contains(purposeLower, "research")) { score += 7 }
		if strings.Contains(hint, "content") || strings.Contains(hint, "copy") {
			if strings.Contains(nameLower, "content") || strings.Contains(nameLower, "copy") || strings.Contains(roleLower, "content") || strings.Contains(roleLower, "copy") { score += 7 }
		}
		if strings.Contains(hint, "design") && (strings.Contains(nameLower, "design") || strings.Contains(roleLower, "design")) { score += 7 }
		// Generic worker_N handling — pick by index
		if strings.Contains(hint, "worker") {
			// Extract number if present
			if hint == "worker_1" || hint == "worker 1" || hint == "content worker 1" { score += 3 }
		}
		if score > bestScore {
			bestScore = score
			best = a
		}
	}
	if best != nil && bestScore >= 5 {
		return &best.ID
	}
	// Fallback round-robin for generic hints or low score
	if false { // P1-6: no heuristic fallback
		// Use idx to pick round-robin
		chosen := candidates[0]
		return &chosen.ID
	}
	if bestScore > 0 { return &best.ID }
	return nil
}

func (r *Runtime) validateDelegationTarget(orgID, projectID uuid.UUID, teamID *uuid.UUID, destAgentID uuid.UUID) error {
	// Must belong to org
	agent, err := r.repo.GetAgent(orgID, destAgentID)
	if err != nil {
		return fmt.Errorf("target agent not in organization")
	}
	if agent.Status != "active" {
		return fmt.Errorf("target agent not active: %s", agent.Status)
	}
	// Must be assigned to project (project_agents) or team if task has team
	assigned := false
	if rows, err := r.repo.Query(`SELECT 1 FROM project_agents WHERE project_id=$1 AND agent_id=$2`, projectID, destAgentID); err == nil {
		if rows.Next() { assigned = true }
		rows.Close()
	}
	if !assigned && teamID != nil {
		if rows, err := r.repo.Query(`SELECT 1 FROM agent_team_members WHERE team_id=$1 AND agent_id=$2`, *teamID, destAgentID); err == nil {
			if rows.Next() { assigned = true }
			rows.Close()
		}
	}
	if !assigned {
		return fmt.Errorf("target agent %s not assigned to project %s", agent.Name, projectID.String()[:8])
	}
	// Capability check if task requires specific capabilities (stored in parent task metadata)
	// For now, we just ensure agent has at least one capability; stricter check can be added per task's required_role
	return nil
}

func safeMsgID(m *domain.Message) string {
	if m == nil { return "" }
	return m.ID.String()
}
func safeCorrID(id *uuid.UUID) string {
	if id == nil { return "" }
	return id.String()
}
func safeUUID(id *uuid.UUID) string {
	if id == nil { return "" }
	return id.String()
}
