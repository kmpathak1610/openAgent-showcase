package runtime

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"openagent/internal/domain"
)

var validDecisionStatuses = map[string]bool{
	"continue": true, "delegate": true, "waiting": true, "approval_required": true,
	"needs_human": true, "completed": true, "failed": true,
}

var validActionTypes = map[string]bool{
	"send_message": true, "create_task": true, "delegate_task": true, "call_tool": true,
	"request_approval": true, "request_human_input": true, "update_task": true, "complete_task": true, "fail_task": true, "store_memory": true,
}

type AgentDecision struct {
	Response      string           `json:"response"`
	Status        string           `json:"status"`
	Actions       []DecisionAction `json:"actions"`
	MemoryUpdates []MemoryUpdate   `json:"memory_updates,omitempty"`
}

type DecisionAction struct {
	Type    string         `json:"type"`
	Params  map[string]any `json:"params,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	AgentID     string `json:"agent_id,omitempty"`
	TaskID      string `json:"task_id,omitempty"`
	ChannelID   string `json:"channel_id,omitempty"`
	Body        string `json:"body,omitempty"`
	Tool        string `json:"tool,omitempty"`
	Input       map[string]any `json:"input,omitempty"`
}

// UnmarshalJSON leniently captures unknown fields into Params and handles LLM aliases
func (a *DecisionAction) UnmarshalJSON(data []byte) error {
	type Alias DecisionAction
	// First get raw map
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil { return err }
	// Unmarshal known fields via Alias
	if err := json.Unmarshal(data, (*Alias)(a)); err != nil { return err }
	// Collect unknown fields into Params
	known := map[string]bool{"type": true, "params": true, "title": true, "description": true, "agent_id": true, "task_id": true, "channel_id": true, "body": true, "tool": true, "input": true}
	if a.Params == nil { a.Params = map[string]any{} }
	for k, v := range raw {
		if !known[k] {
			var val any
			if err := json.Unmarshal(v, &val); err == nil {
				a.Params[k] = val
			}
		}
	}
	// Also if Params had nested input already, keep it
	return nil
}

type MemoryUpdate struct {
	Content     string `json:"content"`
	MemoryType  string `json:"memory_type"`
	Scope       string `json:"scope"`
	Importance  float64 `json:"importance"`
	ProjectID   *string `json:"project_id,omitempty"`
	AgentID     *string `json:"agent_id,omitempty"`
}

func (m *MemoryUpdate) UnmarshalJSON(data []byte) error {
	type Alias MemoryUpdate
	aux := &struct {
		*Alias
		TypeAlias *string `json:"type"`
	}{
		Alias: (*Alias)(m),
	}
	if err := json.Unmarshal(data, &aux); err != nil { return err }
	if m.MemoryType == "" && aux.TypeAlias != nil {
		m.MemoryType = *aux.TypeAlias
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err == nil {
		if m.Content == "" {
			if v, ok := raw["content"]; ok {
				var s string
				if json.Unmarshal(v, &s) == nil { m.Content = s }
			}
			// aliases: text, value, data, summary
			if m.Content == "" {
				for _, k := range []string{"text", "value", "data", "summary", "message"} {
					if v, ok := raw[k]; ok {
						var s string
						if json.Unmarshal(v, &s) == nil && s != "" { m.Content = s; break }
						// try non-string (e.g., object) marshaled as string
						var anyVal any
						if json.Unmarshal(v, &anyVal) == nil {
							if b, err := json.Marshal(anyVal); err == nil { m.Content = string(b) }
							break
						}
					}
				}
			}
			// key + value combo: e.g., {"key":"september_launch_progress","value":"..."}
			if m.Content != "" {
				if kv, ok := raw["key"]; ok {
					var ks string
					if json.Unmarshal(kv, &ks) == nil && ks != "" && !strings.Contains(m.Content, ks) {
						m.Content = ks + ": " + m.Content
					}
				}
			} else if m.Content == "" {
				if kv, ok := raw["key"]; ok {
					if vv, ok2 := raw["value"]; ok2 {
						var ks, vs string
						_ = json.Unmarshal(kv, &ks)
						_ = json.Unmarshal(vv, &vs)
						if ks != "" && vs != "" { m.Content = ks + ": " + vs } else if vs != "" { m.Content = vs } else if ks != "" { m.Content = ks }
					}
				}
			}
		}
		// importance alias
		if m.Importance == 0 {
			if v, ok := raw["importance"]; ok {
				var f float64
				if json.Unmarshal(v, &f) == nil { m.Importance = f }
			}
		}
		// scope alias
		if m.Scope == "" {
			if v, ok := raw["scope"]; ok {
				var s string
				if json.Unmarshal(v, &s) == nil { m.Scope = s }
			}
		}
	}
	return nil
}

// ParseAgentDecision extracts JSON from LLM content (handles markdown fences) and validates
func ParseAgentDecision(content string) (*AgentDecision, error) {
	jsonStr := extractJSON(content)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in LLM output")
	}
	var dec AgentDecision
	if err := json.Unmarshal([]byte(jsonStr), &dec); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	normalizeDecision(&dec)
	if err := ValidateDecision(&dec); err != nil {
		return nil, err
	}
	return &dec, nil
}

func normalizeDecision(d *AgentDecision) {
	// Normalize memory_updates
	for i := range d.MemoryUpdates {
		mu := &d.MemoryUpdates[i]
		// Map common hallucinations to valid types
		switch strings.ToLower(mu.MemoryType) {
		case "context", "project_context", "project_memory", "contextual", "project_note", "team_context":
			mu.MemoryType = "project"
		case "preference", "user_preference", "agent_preference", "preferences":
			mu.MemoryType = "agent"
		case "episodic", "working_memory", "task_memory", "episode", "task_update", "working_note":
			mu.MemoryType = "working"
		case "conversation_history", "chat", "conversation_memory":
			mu.MemoryType = "conversation"
		case "", "fact", "summary", "type":
			mu.MemoryType = "agent"
		}
		if !isValidMemoryType(mu.MemoryType) {
			// Try to infer from content or fallback
			lower := strings.ToLower(mu.MemoryType)
			if strings.Contains(lower, "project") { mu.MemoryType = "project" } else if strings.Contains(lower, "task") || strings.Contains(lower, "working") { mu.MemoryType = "working" } else { mu.MemoryType = "agent" }
		}
		if mu.Scope == "" {
			mu.Scope = mu.MemoryType
			if mu.Scope == "" { mu.Scope = "agent" }
		}
		if !domain.ValidMemoryScopes[mu.Scope] {
			mu.Scope = "agent"
		}
		if mu.Importance == 0 {
			mu.Importance = 0.5
		}
	}
	// Normalize actions: LLM often returns {"tool":"web_search","query":"..."} without type
	for i := range d.Actions {
		act := &d.Actions[i]
		// If type is missing but tool is present, infer type
		if act.Type == "" && act.Tool != "" {
			toolLower := strings.ToLower(act.Tool)
			switch toolLower {
			case "send_message", "create_task", "delegate_task", "call_tool", "request_approval", "request_human_input", "update_task", "complete_task", "fail_task", "store_memory":
				act.Type = toolLower
				act.Tool = ""
			default:
				// Real tool name -> call_tool
				// Preserve tool, set type
				act.Type = "call_tool"
				// Build Input from Params + any top-level extras captured in Params
				if act.Input == nil && len(act.Params) > 0 {
					input := map[string]any{}
					for k, v := range act.Params {
						if k == "tool" || k == "type" || k == "purpose" { continue }
						input[k] = v
					}
					if len(input) > 0 { act.Input = input }
				}
			}
		}
		// Handle case where LLM put tool in Params.tool or Params.action instead of top-level
		if act.Type == "" && act.Params != nil {
			var t string
			if v, ok := act.Params["tool"].(string); ok && v != "" { t = v }
			if t == "" {
				if v, ok := act.Params["action"].(string); ok && v != "" { t = v }
			}
			// Also check if action was captured as known field alias
			if t == "" && act.Params["action"] != nil {
				// already handled above, but also check raw action string in Params
			}
			if t != "" {
				toolLower := strings.ToLower(t)
				switch toolLower {
				case "send_message", "create_task", "delegate_task", "call_tool", "request_approval", "request_human_input", "update_task", "complete_task", "fail_task", "store_memory":
					act.Type = toolLower
				case "web_search", "http_request", "publish_social_post", "send_email", "create_calendar_event", "upload_file", "send_channel_message":
					act.Type = "call_tool"
					act.Tool = t
					if act.Input == nil {
						input := map[string]any{}
						for k, v := range act.Params {
							if k == "tool" || k == "type" || k == "action" || k == "purpose" { continue }
							input[k] = v
						}
						if len(input) > 0 { act.Input = input }
					}
				default:
					// Check if it's an action-like string like "web_search" vs "create_task"
					if strings.Contains(toolLower, "task") { act.Type = "create_task" } else if strings.Contains(toolLower, "message") { act.Type = "send_message" } else {
						act.Type = "call_tool"
						act.Tool = t
						if act.Input == nil {
							input := map[string]any{}
							for k, v := range act.Params {
								if k == "tool" || k == "type" || k == "action" || k == "purpose" { continue }
								input[k] = v
							}
							if len(input) > 0 { act.Input = input }
						}
					}
				}
			}
		}
		// Handle case where LLM used "action" at top level as type alias (e.g., {"action":"web_search","params":{...}})
		if act.Type == "" && act.Params != nil {
			if v, ok := act.Params["action"].(string); ok && v != "" {
				// If action was captured into Params because known didn't include "action"
				toolLower := strings.ToLower(v)
				if toolLower == "web_search" || toolLower == "http_request" {
					act.Type = "call_tool"
					act.Tool = v
					// Params already contains nested params, need to merge
					if nested, ok := act.Params["params"].(map[string]any); ok && len(nested) > 0 {
						act.Input = nested
					}
				}
			}
		}
		// Aliases for create_task (including nested params)
		if act.Type == "create_task" && act.Title == "" {
			if act.Params != nil {
				// Check nested params first: {"action":"create_task","params":{"title":...}}
				var src map[string]any = act.Params
				if nested, ok := act.Params["params"].(map[string]any); ok && len(nested) > 0 {
					src = nested
					// Also keep nested for Input if needed
					if act.Description == "" {
						// Use nested for title/description extraction
					}
				}
				for _, k := range []string{"task_title", "name", "taskName", "task_name", "title"} {
					if v, ok := src[k].(string); ok && v != "" { act.Title = v; break }
				}
				if act.Description == "" {
					for _, k := range []string{"details", "description", "content", "body", "detail"} {
						if v, ok := src[k].(string); ok && v != "" { act.Description = v; break }
					}
				}
				if act.Description == "" {
					if v, ok := src["purpose"].(string); ok && v != "" { act.Description = v }
				}
				// If still not found, check top-level Params again (fallback)
				if act.Title == "" {
					for _, k := range []string{"task_title", "name", "taskName", "task_name", "title"} {
						if v, ok := act.Params[k].(string); ok && v != "" { act.Title = v; break }
					}
				}
			}
		}
		// Aliases for send_message
		if act.Type == "send_message" && act.Body == "" {
			if act.Params != nil {
				for _, k := range []string{"message", "content", "text", "body"} {
					if v, ok := act.Params[k].(string); ok && v != "" { act.Body = v; break }
				}
			}
		}
		// For call_tool, ensure Input is populated (including nested params)
		if act.Type == "call_tool" && act.Input == nil {
			// Check nested params: {"action":"web_search","params":{"query":...}}
			if nested, ok := act.Params["params"].(map[string]any); ok && len(nested) > 0 {
				// Filter purpose etc.
				input := map[string]any{}
				for k, v := range nested {
					if k == "purpose" { continue }
					input[k] = v
				}
				if len(input) > 0 { act.Input = input }
			} else if len(act.Params) > 0 {
				input := map[string]any{}
				for k, v := range act.Params {
					if k == "tool" || k == "type" || k == "action" || k == "params" { continue }
					if k == "purpose" { continue }
					input[k] = v
				}
				if len(input) > 0 { act.Input = input }
			}
		}
		// If call_tool still has no Input but Params had query, ensure it's there
		if act.Type == "call_tool" && act.Tool == "web_search" && act.Input == nil {
			// Try to build from top-level captured extras
			if act.Params != nil {
				if q, ok := act.Params["query"].(string); ok && q != "" {
					act.Input = map[string]any{"query": q}
				}
			}
		}
	}
}

func extractJSON(content string) string {
	content = strings.TrimSpace(content)
	re := regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*?\\})\\s*```")
	if m := re.FindStringSubmatch(content); len(m) > 1 {
		return m[1]
	}
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >=0 && end > start {
		return content[start : end+1]
	}
	return content
}

func ValidateDecision(d *AgentDecision) error {
	if d.Status == "" { return fmt.Errorf("status required") }
	if !validDecisionStatuses[d.Status] {
		return fmt.Errorf("invalid status %s", d.Status)
	}
	for i, act := range d.Actions {
		if !validActionTypes[act.Type] {
			return fmt.Errorf("action %d: invalid type %s", i, act.Type)
		}
		if err := validateAction(act); err != nil {
			return fmt.Errorf("action %d (%s): %w", i, act.Type, err)
		}
	}
	for i, mu := range d.MemoryUpdates {
		if mu.Content == "" { return fmt.Errorf("memory_update %d: content required", i) }
		if mu.MemoryType != "" && !isValidMemoryType(mu.MemoryType) {
			return fmt.Errorf("memory_update %d: invalid memory_type %s", i, mu.MemoryType)
		}
		if len(mu.Content) > 5000 {
			return fmt.Errorf("memory_update %d: content too large", i)
		}
		lower := strings.ToLower(mu.Content)
		if strings.Contains(lower, "api_key") || strings.Contains(lower, "password") || strings.Contains(lower, "secret") {
			if strings.Contains(lower, "sk-") || strings.Contains(lower, "bearer") {
				return fmt.Errorf("memory_update %d: refusing to store potential secret", i)
			}
		}
	}
	for _, act := range d.Actions {
		if act.Params != nil {
			if _, ok := act.Params["code"]; ok {
				return fmt.Errorf("action %s: code execution not allowed", act.Type)
			}
			if _, ok := act.Params["exec"]; ok {
				return fmt.Errorf("action %s: exec not allowed", act.Type)
			}
		}
	}
	return nil
}

func validateAction(act DecisionAction) error {
	switch act.Type {
	case "send_message":
		body := act.Body
		if body == "" {
			if act.Params != nil {
				if b, ok := act.Params["body"].(string); ok { body = b }
				if b, ok := act.Params["message"].(string); ok && body == "" { body = b }
				if b, ok := act.Params["content"].(string); ok && body == "" { body = b }
			}
		}
		if body == "" { return fmt.Errorf("body required") }
		if len(body) > 5000 { return fmt.Errorf("body too large") }
	case "create_task":
		title := act.Title
		if title == "" && act.Params != nil {
			if t, ok := act.Params["title"].(string); ok { title = t }
			if t, ok := act.Params["task_title"].(string); ok && title == "" { title = t }
		}
		if title == "" { return fmt.Errorf("title required") }
	case "delegate_task":
		taskID := act.TaskID
		agentID := act.AgentID
		if taskID == "" && act.Params != nil {
			if v, ok := act.Params["taskId"].(string); ok { taskID = v }
			if v, ok := act.Params["task_id"].(string); ok { taskID = v }
		}
		if agentID == "" && act.Params != nil {
			if v, ok := act.Params["agentId"].(string); ok { agentID = v }
			if v, ok := act.Params["agent_id"].(string); ok { agentID = v }
		}
		if taskID == "" { return fmt.Errorf("task_id required") }
		if agentID == "" { return fmt.Errorf("agent_id required") }
		if _, err := uuid.Parse(taskID); err != nil { return fmt.Errorf("invalid task_id") }
		if _, err := uuid.Parse(agentID); err != nil { return fmt.Errorf("invalid agent_id") }
	case "call_tool":
		tool := act.Tool
		if tool == "" && act.Params != nil {
			if t, ok := act.Params["tool"].(string); ok { tool = t }
		}
		if tool == "" { return fmt.Errorf("tool required") }
	case "request_approval", "request_human_input":
	case "update_task", "complete_task", "fail_task":
	case "store_memory":
		if act.Params != nil {
			if c, ok := act.Params["content"].(string); !ok || c == "" {
				return fmt.Errorf("content required for store_memory")
			}
		}
	default:
		return fmt.Errorf("unsupported action %s", act.Type)
	}
	return nil
}

func isValidMemoryType(s string) bool {
	return s == "working" || s == "project" || s == "agent" || s == "conversation"
}
