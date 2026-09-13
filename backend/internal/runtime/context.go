package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"openagent/internal/domain"
	"openagent/internal/security"
)

// buildAgentContext assembles bounded, permission-aware context for LLM
func (r *Runtime) buildAgentContext(ctx context.Context, agent *domain.Agent, project *domain.Project, task *domain.Task, retrieved []domain.RetrievalResult, memories []*domain.Memory, conversation []string, previousOutputs []map[string]any) string {
	var sb strings.Builder
	sb.WriteString("AGENT DEFINITION:\n")
	sb.WriteString(fmt.Sprintf("Name: %s\nRole: %s\nPurpose: %s\nObjective: %s\n", agent.Name, getRole(agent), agent.Purpose, getObjective(agent)))
	if agent.Version != nil {
		sb.WriteString(fmt.Sprintf("Instructions: %s\n", security.SanitizeUserContent(agent.Version.Instructions)))
		sb.WriteString(fmt.Sprintf("Capabilities: %v\n", agent.Version.Capabilities))
		sb.WriteString(fmt.Sprintf("Behavioral Rules: %v\n", agent.Version.BehavioralRules))
	}
	sb.WriteString("\nPROJECT:\n")
	if project != nil {
		sb.WriteString(fmt.Sprintf("Name: %s\nObjective: %s\nDescription: %s\n", project.Name, project.Objective, project.Description))
	}
	sb.WriteString("\nTASK:\n")
	sb.WriteString(fmt.Sprintf("Title: %s\nDescription: %s\nStatus: %s\nPriority: %s\n", security.SanitizeUserContent(task.Title), security.SanitizeUserContent(task.Description), task.Status, task.Priority))
	if task.ParentTaskID != nil {
		sb.WriteString(fmt.Sprintf("Parent: %s\n", task.ParentTaskID.String()))
	}
	sb.WriteString("\nCONVERSATION (last 5):\n")
	if len(conversation) > 5 { conversation = conversation[len(conversation)-5:] }
	for _, msg := range conversation {
		sb.WriteString(security.SanitizeUserContent(msg) + "\n")
	}
	sb.WriteString("\nRETRIEVED KNOWLEDGE (permission-filtered, top 3):\n")
	if len(retrieved) == 0 {
		sb.WriteString("none\n")
	} else {
		for i, r := range retrieved {
			if i >= 3 { break }
			content := r.Chunk.Content
			if len(content) > 300 { content = content[:300] }
			sb.WriteString(fmt.Sprintf("[%d] %s: %s (score %.2f)\n", i+1, r.Chunk.DocumentTitle, security.SanitizeUserContent(content), r.Score))
		}
	}
	sb.WriteString("\nRELEVANT MEMORIES (bounded, importance >=0.3, ACTIVE only, deduped, confidence-aware):\n")
	if len(memories) == 0 {
		sb.WriteString("none\n")
	} else {
		// Context quality: remove duplicates, superseded, enforce token/size limit, sort by relevance/confidence
		// Memories already filtered to ACTIVE and sorted by confidence/importance in Service.Retrieve, but do additional dedup here for LLM context
		seenContent := make(map[string]bool)
		var filtered []*domain.Memory
		for _, m := range memories {
			if m.Status == "archived" || m.Status == "superseded" || m.Status == "stale" || m.Status == "merged" {
				continue
			}
			norm := strings.ToLower(strings.TrimSpace(m.Content))
			if seenContent[norm] {
				continue
			}
			seenContent[norm] = true
			filtered = append(filtered, m)
		}
		// Sort by confidence descending then importance
		// already sorted but re-sort for confidence
		for i := 0; i < len(filtered); i++ {
			for j := i + 1; j < len(filtered); j++ {
				if filtered[j].Confidence > filtered[i].Confidence || (filtered[j].Confidence == filtered[i].Confidence && filtered[j].Importance > filtered[i].Importance) {
					filtered[i], filtered[j] = filtered[j], filtered[i]
				}
			}
		}
		// Enforce token/size limit: max 3 memories, each max 250 chars, total <= 800 chars for memory section
		total := 0
		count := 0
		for _, m := range filtered {
			if count >= 3 {
				break
			}
			content := m.Content
			if len(content) > 250 {
				content = content[:250] + "..."
			}
			// Include confidence and source when useful (>0.7 or explicit)
			confStr := ""
			if m.Confidence > 0 {
				confStr = fmt.Sprintf(" [conf:%.2f]", m.Confidence)
			}
			sourceStr := ""
			if m.Source != "" && m.Source != "manual" && m.Source != "interaction" {
				sourceStr = fmt.Sprintf(" src:%s", m.Source)
			}
			line := fmt.Sprintf("[%d] (%s:%s%s%s) %s\n", count+1, m.MemoryType, m.Scope, confStr, sourceStr, security.SanitizeUserContent(content))
			if total+len(line) > 1000 {
				break
			}
			sb.WriteString(line)
			total += len(line)
			count++
		}
		if count == 0 {
			sb.WriteString("none (filtered duplicates/superseded)\n")
		}
	}
	sb.WriteString("\nTEAM CONTEXT:\n")
	if task.TeamID != nil {
		if members, err := r.repo.ListTeamMembers(*task.TeamID); err == nil {
			for _, m := range members {
				sb.WriteString(fmt.Sprintf("- %s (%s): %s\n", m.AgentID.String()[:8], m.Role, m.Responsibilities))
			}
		}
	}
	sb.WriteString("\nAVAILABLE PROJECT AGENTS (for delegation — choose by ID if you need help):\n")
	if project != nil {
		if agents, err := r.repo.ListProjectAgents(task.ProjectID); err == nil {
			for _, ag := range agents {
				if ag.ID == agent.ID { continue }
				role := ag.Purpose
				if ag.Version != nil && ag.Version.Role != "" { role = ag.Version.Role }
				caps := ""
				if ag.Version != nil && len(ag.Version.Capabilities) > 0 {
					var names []string
					for _, c := range ag.Version.Capabilities { names = append(names, c.Name) }
					caps = strings.Join(names, ",")
				} else if len(ag.Capabilities) > 0 {
					caps = strings.Join(ag.Capabilities, ",")
				}
				sb.WriteString(fmt.Sprintf("- %s | ID:%s | Role:%s | Capabilities:%s\n", ag.Name, ag.ID.String(), role, caps))
			}
		} else {
			// Fallback to ListAgents filtered by org
			if agents, err := r.repo.ListAgents(agent.OrganizationID); err == nil {
				for _, ag := range agents {
					if ag.ID == agent.ID { continue }
					role := ag.Purpose
					if ag.Version != nil && ag.Version.Role != "" { role = ag.Version.Role }
					sb.WriteString(fmt.Sprintf("- %s | ID:%s | Role:%s\n", ag.Name, ag.ID.String(), role))
				}
			}
		}
	}
	sb.WriteString("\nPERMISSIONS/POLICIES:\n")
	sb.WriteString(fmt.Sprintf("Autonomy: %s\n", agent.AutonomyLevel))
	if agent.Version != nil && agent.Version.ApprovalPolicy != nil {
		sb.WriteString(fmt.Sprintf("Approval: %v\n", agent.Version.ApprovalPolicy))
	}
	sb.WriteString("\nPREVIOUS TOOL RESULTS (last 2):\n")
	if len(previousOutputs) == 0 {
		sb.WriteString("none\n")
	} else {
		start := len(previousOutputs) - 2
		if start < 0 { start = 0 }
		for _, out := range previousOutputs[start:] {
			sb.WriteString(fmt.Sprintf("%v\n", out))
		}
	}
	// Truncate to 8000 chars
	contextStr := sb.String()
	if len(contextStr) > 8000 {
		contextStr = contextStr[:8000] + " [truncated]"
	}
	return contextStr
}

func getRole(agent *domain.Agent) string {
	if agent.Version != nil && agent.Version.Role != "" { return agent.Version.Role }
	return agent.Purpose
}
func getObjective(agent *domain.Agent) string {
	if agent.Version != nil && agent.Version.Objective != "" { return agent.Version.Objective }
	return agent.Purpose
}

// loadConversation loads last N messages from channel for context (bounded)
func (r *Runtime) loadConversation(orgID uuid.UUID, channelID *uuid.UUID, limit int) []string {
	if channelID == nil { return nil }
	if limit <=0 { limit = 5 }
	msgs, err := r.repo.ListMessages(orgID, *channelID, nil, time.Time{}, limit, false)
	if err != nil || len(msgs)==0 { return nil }
	var out []string
	for _, m := range msgs {
		// Do not expose raw credentials or secrets
		body := security.SanitizeUserContent(m.Body)
		out = append(out, fmt.Sprintf("%s: %s", m.SenderType, body))
	}
	return out
}

// isValidTransition checks if task status transition is allowed
var validTransitions = map[string][]string{
	"pending": {"assigned", "cancelled"},
	"assigned": {"running", "cancelled"},
	"running": {"waiting", "blocked", "approval_required", "completed", "failed", "cancelled"},
	"waiting": {"running", "completed", "failed", "cancelled"},
	"blocked": {"running", "cancelled"},
	"approval_required": {"running", "completed", "failed", "cancelled"},
	"completed": {},
	"failed": {"pending", "cancelled"}, // retry
	"cancelled": {},
}

func isValidTransition(from, to string) bool {
	allowed, ok := validTransitions[from]
	if !ok { return false }
	for _, a := range allowed { if a == to { return true } }
	return false
}
