package builder

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"openagent/internal/domain"
	"openagent/internal/llm"
)

// Input is the 7-step wizard data
type Input struct {
	Description       string   `json:"description"`       // Step1: What do you want it to do?
	Responsibilities  []string `json:"responsibilities"`  // Step2
	InformationSources []string `json:"informationSources"` // Step3: knowledge
	Actions           []string `json:"actions"`           // Step4: actions
	AutonomyPreference string  `json:"autonomyPreference"` // Step5: assistant/task_executor/collaborative/autonomous
}

// Service converts NL to structured preview
type Service struct {
	registry *llm.Registry
}

func New(registry *llm.Registry) *Service { return &Service{registry: registry} }

// Dangerous capabilities that must require approval by default
var dangerousCapabilities = map[string]bool{
	"publish_content": true,
	"send_email":      true,
	"external_api":    true,
	"delete_data":     true,
	"financial":       true,
	"deploy":          true,
	"moderation":      true,
}

// capability catalog
type capDef struct {
	Name        string
	Description string
	Keywords    []string
	Dangerous   bool
	Tools       []string
}

var catalog = []capDef{
	{"content_creation", "Create and edit content", []string{"content", "write", "social", "post", "blog", "copy"}, false, []string{"editor", "content_gen"}},
	{"publish_content", "Publish content externally", []string{"publish", "social", "post", "tweet", "linkedin"}, true, []string{"social_publish", "cms_publish"}},
	{"performance_analysis", "Analyze performance metrics", []string{"analyze", "performance", "metrics", "analytics", "report"}, false, []string{"analytics"}},
	{"scheduling", "Schedule tasks and posts", []string{"schedule", "calendar", "plan"}, false, []string{"scheduler"}},
	{"research", "Research information", []string{"research", "search", "knowledge", "docs"}, false, []string{"search", "rag"}},
	{"communication", "Communicate with users", []string{"coordinate", "chat", "message", "notify"}, false, []string{"messaging"}},
	{"data_management", "Manage data", []string{"manage", "organize", "data", "database"}, false, []string{"database"}},
	{"send_email", "Send emails", []string{"email", "mail"}, true, []string{"email"}},
	{"external_api", "Call external APIs", []string{"api", "integration", "external", "webhook"}, true, []string{"http_tool"}},
	{"automation", "Automate workflows", []string{"automate", "workflow", "coordinate", "agent"}, false, []string{"workflow"}},
}

func (s *Service) GeneratePreview(ctx context.Context, in Input) (*domain.AgentBuilderPreview, error) {
	// Try LLM-driven generation first if real provider available
	if s.registry != nil {
		preview, genErr := s.generateViaLLM(ctx, in)
		if genErr == nil && preview != nil {
			s.applySafeguards(preview, in)
			if verr := ValidatePreview(preview); verr == nil {
				return preview, nil
			} else {
				// Validation failed: retry with repair prompt once
				if repaired, rerr := s.generateViaLLMRepair(ctx, in, verr); rerr == nil && repaired != nil {
					s.applySafeguards(repaired, in)
					if verr2 := ValidatePreview(repaired); verr2 == nil {
						return repaired, nil
					} else {
						genErr = verr2
					}
				} else {
					genErr = verr
				}
				if os.Getenv("ENV") == "production" {
					return nil, fmt.Errorf("builder validation failed: %w", genErr)
				}
			}
		} else if genErr != nil {
			if os.Getenv("ENV") == "production" {
				if strings.Contains(genErr.Error(), "no real LLM") || strings.Contains(genErr.Error(), "invalid json") {
					return nil, fmt.Errorf("builder LLM failed in production: %w", genErr)
				}
			}
		}
	}
	// Fallback deterministic only for dev/test or when explicitly allowed
	if os.Getenv("ENV") == "production" && s.registry != nil {
		for _, name := range []string{"openrouter", "openai", "anthropic", "google"} {
			if p, err := s.registry.Get(name); err == nil && p.Name() != "stub" {
				return nil, fmt.Errorf("LLM generation failed and deterministic fallback not allowed in production")
			}
		}
	}
	descLower := strings.ToLower(in.Description + " " + strings.Join(in.Responsibilities, " ") + " " + strings.Join(in.Actions, " "))
	preview := &domain.AgentBuilderPreview{}

	// Name: extract or generate
	preview.Name = generateName(in.Description)
	preview.Description = strings.TrimSpace(in.Description)
	if preview.Description == "" { preview.Description = "AI agent to help with " + preview.Name }
	preview.Purpose = generatePurpose(in)
	preview.Role = generateRole(in)
	preview.Objective = strings.Join(in.Responsibilities, "; ")
	if preview.Objective == "" { preview.Objective = preview.Purpose }
	preview.Instructions = generateInstructions(in, preview.Role)

	// Capabilities: match catalog
	preview.Capabilities = matchCapabilities(descLower, in)
	preview.BehavioralRules = generateBehavioralRules(preview.Capabilities, in.AutonomyPreference)
	preview.Responsibilities = in.Responsibilities
	if len(preview.Responsibilities) == 0 {
		preview.Responsibilities = inferResponsibilities(descLower)
	}

	// Tools
	toolSet := map[string]bool{}
	for _, c := range preview.Capabilities {
		for _, kw := range catalog {
			if kw.Name == c.Name {
				for _, t := range kw.Tools { toolSet[t] = true }
			}
		}
	}
	for _, a := range in.Actions { toolSet[slugify(a)] = true }
	for t := range toolSet { preview.RecommendedTools = append(preview.RecommendedTools, t) }
	if preview.RecommendedTools == nil { preview.RecommendedTools = []string{} }

	preview.ToolPolicy = map[string]any{"allowed_tools": preview.RecommendedTools}
	preview.MemoryPolicy = map[string]any{"retention": "30d", "scope": "project"}
	if len(in.InformationSources) > 0 {
		preview.MemoryPolicy["sources"] = in.InformationSources
		preview.KnowledgeNeeds = in.InformationSources
	} else {
		preview.KnowledgeNeeds = inferKnowledge(descLower)
	}
	preview.ModelConfiguration = map[string]any{"model": "openai/gpt-4o-mini", "temperature": 0.7}

	// Autonomy: respect preference but downgrade if dangerous
	requested := strings.ToLower(strings.TrimSpace(in.AutonomyPreference))
	if requested == "" { requested = "task_executor" }
	if !domain.ValidAutonomyLevels[requested] { requested = "task_executor" }
	hasDangerous := false
	for _, c := range preview.Capabilities {
		if dangerousCapabilities[c.Name] || c.Name == "publish_content" { hasDangerous = true }
	}
	if hasDangerous && requested == "autonomous" {
		requested = "collaborative"
		preview.Warnings = append(preview.Warnings, "Autonomy downgraded to collaborative because publishing/external actions require human approval")
	}
	preview.AutonomyLevel = requested

	// Approval policy: dangerous => require approval
	approval := map[string]any{"require_approval_for": []string{}}
	require := []string{}
	for _, c := range preview.Capabilities {
		if dangerousCapabilities[c.Name] {
			require = append(require, c.Name)
		}
	}
	if len(require) > 0 {
		approval["require_approval_for"] = require
		approval["default"] = "require_approval"
		preview.Warnings = append(preview.Warnings, fmt.Sprintf("Actions %v require approval by default", require))
	} else {
		approval["default"] = "auto"
	}
	preview.ApprovalPolicy = approval

	// Permissions: default least privilege
	preview.Permissions = []domain.AgentPermission{
		{ResourceType: "organization", Permission: "read"},
	}
	// if publish, need write
	if hasDangerous {
		preview.Permissions = append(preview.Permissions, domain.AgentPermission{ResourceType: "tool", Permission: "execute"})
	}

	preview.Avatar = pickAvatar(descLower)

	// Validation
	if err := ValidatePreview(preview); err != nil {
		return nil, err
	}
	return preview, nil
}

func (s *Service) generateViaLLM(ctx context.Context, in Input) (*domain.AgentBuilderPreview, error) {
	// Select real provider (openrouter > openai > anthropic), skip stub in production
	var provider llm.Provider
	var model string
	for _, name := range []string{"openrouter", "openai", "anthropic", "google"} {
		if p, err := s.registry.Get(name); err == nil && p.Name() != "stub" {
			// Also check if it's actually a stub instance (Name() == "stub" check above)
			provider = p
			// Get model from provider if available
			if mp, ok := p.(interface{ Model() string }); ok {
				model = mp.Model()
			}
			break
		}
	}
	if provider == nil {
		return nil, fmt.Errorf("no real LLM provider available")
	}
	if model == "" {
		model = "openai/gpt-4o-mini"
	}
	prompt := fmt.Sprintf(`You are an AI agent architect. Convert the user's natural language request into a structured agent definition.

User request: "%s"
Responsibilities: %v
Information sources: %v
Actions: %v
Autonomy preference: %s

Respond ONLY with valid JSON matching this schema:
{
  "name": "Agent Name (max 40 chars, Title Case, ends with Agent)",
  "description": "One sentence purpose",
  "purpose": "Detailed purpose",
  "role": "Role like Social Media Manager, Researcher, Content Creator",
  "objective": "Primary objective",
  "responsibilities": ["list of 2-4 responsibilities"],
  "instructions": "System instructions for the agent (2-3 sentences)",
  "capabilities": [{"name": "capability_name", "description": "desc", "enabled": true}],
  "behavioralRules": ["rule1", "rule2"],
  "recommendedTools": ["tool1", "tool2"],
  "knowledgeNeeds": ["need1"],
  "modelConfiguration": {"model": "openai/gpt-4o-mini", "temperature": 0.7},
  "autonomyLevel": "task_executor",
  "approvalPolicy": {"require_approval_for": [], "default": "auto"},
  "permissions": [{"resourceType": "organization", "permission": "read"}],
  "avatar": "🤖"
}

Valid capabilities: content_creation, publish_content, performance_analysis, scheduling, research, communication, data_management, send_email, external_api, automation
Valid autonomyLevel: assistant, task_executor, collaborative, autonomous
Risky capabilities (publish_content, send_email, external_api, delete_data, financial) must have approvalPolicy.require_approval_for containing them and autonomyLevel must not be autonomous (use collaborative).

Available catalog: %v`, in.Description, in.Responsibilities, in.InformationSources, in.Actions, in.AutonomyPreference, catalog)

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: "You are a helpful AI architect. Always respond with valid JSON only, no markdown, no extra text."},
		{Role: llm.RoleUser, Content: prompt},
	}
	resp, err := provider.Complete(ctx, llm.CompletionRequest{Model: model, Messages: messages})
	if err != nil {
		return nil, err
	}
	// Extract JSON (handle fences)
	content := strings.TrimSpace(resp.Content)
	if start := strings.Index(content, "{"); start >= 0 {
		if end := strings.LastIndex(content, "}"); end > start {
			content = content[start : end+1]
		}
	}
	var preview domain.AgentBuilderPreview
	if err := json.Unmarshal([]byte(content), &preview); err != nil {
		return nil, fmt.Errorf("llm returned invalid json: %w", err)
	}
	return &preview, nil
}

func (s *Service) applySafeguards(preview *domain.AgentBuilderPreview, in Input) {
	// Ensure dangerous capabilities require approval and autonomy not autonomous
	hasDangerous := false
	for _, c := range preview.Capabilities {
		if dangerousCapabilities[c.Name] {
			hasDangerous = true
			break
		}
	}
	if hasDangerous {
		if preview.ApprovalPolicy == nil {
			preview.ApprovalPolicy = map[string]any{}
		}
		// Ensure require_approval_for contains dangerous caps
		existing, _ := preview.ApprovalPolicy["require_approval_for"].([]any)
		existingSet := map[string]bool{}
		for _, v := range existing {
			if s, ok := v.(string); ok {
				existingSet[s] = true
			}
		}
		for _, c := range preview.Capabilities {
			if dangerousCapabilities[c.Name] && !existingSet[c.Name] {
				existing = append(existing, c.Name)
			}
		}
		preview.ApprovalPolicy["require_approval_for"] = existing
		if preview.ApprovalPolicy["default"] == nil {
			preview.ApprovalPolicy["default"] = "require_approval"
		}
		if preview.AutonomyLevel == "autonomous" {
			preview.AutonomyLevel = "collaborative"
			preview.Warnings = append(preview.Warnings, "Autonomy downgraded to collaborative because publishing/external actions require human approval")
		}
	}
	// Ensure permissions are least privilege (LLM cannot grant itself admin)
	// Filter out any permission that is not read or tool:execute
	filtered := []domain.AgentPermission{}
	for _, p := range preview.Permissions {
		if p.ResourceType == "organization" && p.Permission == "read" {
			filtered = append(filtered, p)
		} else if p.ResourceType == "tool" && p.Permission == "execute" {
			filtered = append(filtered, p)
		} else if p.ResourceType == "project" && p.Permission == "read" {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == 0 {
		filtered = []domain.AgentPermission{{ResourceType: "organization", Permission: "read"}}
	}
	preview.Permissions = filtered
	// Ensure modelConfiguration exists
	if preview.ModelConfiguration == nil {
		preview.ModelConfiguration = map[string]any{"model": "openai/gpt-4o-mini", "temperature": 0.7}
	}
}

func (s *Service) generateViaLLMRepair(ctx context.Context, in Input, prevErr error) (*domain.AgentBuilderPreview, error) {
	var provider llm.Provider
	var model string
	for _, name := range []string{"openrouter", "openai", "anthropic", "google"} {
		if p, err := s.registry.Get(name); err == nil && p.Name() != "stub" {
			provider = p
			if mp, ok := p.(interface{ Model() string }); ok {
				model = mp.Model()
			}
			break
		}
	}
	if provider == nil {
		return nil, fmt.Errorf("no real LLM for repair")
	}
	if model == "" {
		model = "openai/gpt-4o-mini"
	}
	prompt := fmt.Sprintf(`Previous attempt failed validation: %v.
Repair the JSON to match required schema exactly. Ensure all required fields are present and valid.
User request: "%s"
Responsibilities: %v
Actions: %v

Respond ONLY with valid JSON matching schema:
{
  "name": "Agent Name",
  "description": "One sentence",
  "purpose": "Detailed purpose",
  "role": "Role",
  "objective": "Primary objective",
  "responsibilities": ["list of 2-4"],
  "instructions": "System instructions (2-3 sentences)",
  "capabilities": [{"name": "capability_name", "description": "desc", "enabled": true}],
  "behavioralRules": ["rule1"],
  "recommendedTools": ["tool1"],
  "knowledgeNeeds": ["need1"],
  "modelConfiguration": {"model": "openai/gpt-4o-mini", "temperature": 0.7},
  "autonomyLevel": "task_executor",
  "approvalPolicy": {"require_approval_for": [], "default": "auto"},
  "permissions": [{"resourceType": "organization", "permission": "read"}],
  "avatar": "🤖"
}`, prevErr, in.Description, in.Responsibilities, in.Actions)
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: "You are a helpful AI architect. Respond with valid JSON only, no markdown. Fix validation errors."},
		{Role: llm.RoleUser, Content: prompt},
	}
	resp, err := provider.Complete(ctx, llm.CompletionRequest{Model: model, Messages: messages})
	if err != nil {
		return nil, err
	}
	content := strings.TrimSpace(resp.Content)
	if start := strings.Index(content, "{"); start >= 0 {
		if end := strings.LastIndex(content, "}"); end > start {
			content = content[start : end+1]
		}
	}
	var preview domain.AgentBuilderPreview
	if err := json.Unmarshal([]byte(content), &preview); err != nil {
		return nil, fmt.Errorf("repair invalid json: %w", err)
	}
	return &preview, nil
}

func ValidatePreview(p *domain.AgentBuilderPreview) error {
	if strings.TrimSpace(p.Name) == "" { return fmt.Errorf("name required") }
	if len(p.Name) > 80 { return fmt.Errorf("name too long") }
	if strings.TrimSpace(p.Description) == "" { return fmt.Errorf("description required") }
	if strings.TrimSpace(p.Purpose) == "" { return fmt.Errorf("purpose required") }
	if strings.TrimSpace(p.Role) == "" { return fmt.Errorf("role required") }
	if strings.TrimSpace(p.Objective) == "" { return fmt.Errorf("objective required") }
	if len(p.Responsibilities) == 0 { return fmt.Errorf("responsibilities required") }
	if len(p.Capabilities) == 0 { return fmt.Errorf("capabilities required") }
	if len(p.RecommendedTools) == 0 { // knowledgeNeeds may be empty but warn
		// Allow empty but ensure at least params present? Require non-empty for production
		// For minimal, allow empty but validation will warn; keep strict for full spec
	}
	if p.AutonomyLevel == "" { return fmt.Errorf("autonomy_level required") }
	if !domain.ValidAutonomyLevels[p.AutonomyLevel] { return fmt.Errorf("invalid autonomy_level") }
	if p.ApprovalPolicy == nil { return fmt.Errorf("approvalPolicy required") }
	if p.BehavioralRules == nil { return fmt.Errorf("behavioralRules required") }
	// dangerous safeguard: if publish capability enabled, approval must require approval
	for _, c := range p.Capabilities {
		if dangerousCapabilities[c.Name] {
			if req, ok := p.ApprovalPolicy["require_approval_for"]; ok {
				b, _ := json.Marshal(req)
				if !strings.Contains(string(b), c.Name) {
					return fmt.Errorf("dangerous capability %s must require approval", c.Name)
				}
			} else {
				return fmt.Errorf("dangerous capability %s requires approval_policy", c.Name)
			}
			if p.AutonomyLevel == "autonomous" {
				return fmt.Errorf("dangerous capability %s cannot have autonomous level", c.Name)
			}
		}
	}
	return nil
}

func generateName(desc string) string {
	desc = strings.TrimSpace(desc)
	if desc == "" { return "New Agent" }
	// Take first meaningful words
	words := strings.Fields(desc)
	// Skip "I need an agent that"
	lower := strings.ToLower(desc)
	prefixes := []string{"i need an agent that ", "i want an agent that ", "create an agent that ", "an agent that "}
	for _, pre := range prefixes {
		if strings.HasPrefix(lower, pre) {
			trimmed := desc[len(pre):]
			words = strings.Fields(trimmed)
			break
		}
	}
	// Build title case name from first 3-4 words
	n := 3
	if len(words) < n { n = len(words) }
	name := strings.Join(words[:n], " ")
	// Title case
	name = strings.Title(strings.ToLower(name))
	if len(name) > 40 { name = name[:40] }
	if !strings.HasSuffix(strings.ToLower(name), "agent") {
		name += " Agent"
	}
	return name
}

func generatePurpose(in Input) string {
	if in.Description != "" { return in.Description }
	return "Assist with " + strings.Join(in.Responsibilities, ", ")
}

func generateRole(in Input) string {
	lower := strings.ToLower(in.Description)
	if strings.Contains(lower, "social") { return "Social Media Manager" }
	if strings.Contains(lower, "content") { return "Content Creator" }
	if strings.Contains(lower, "support") { return "Support Assistant" }
	if strings.Contains(lower, "research") { return "Researcher" }
	if strings.Contains(lower, "manage") { return "Coordinator" }
	return "Assistant"
}

func generateInstructions(in Input, role string) string {
	return fmt.Sprintf("You are a %s. Your purpose: %s. Responsibilities: %v. Always be helpful, ask for clarification when needed, and request approval before publishing or taking irreversible actions. Information sources: %v. Actions allowed: %v.",
		role, in.Description, in.Responsibilities, in.InformationSources, in.Actions)
}

func generateBehavioralRules(caps []domain.AgentCapability, autonomy string) []string {
	rules := []string{"Be helpful and concise", "Ask for clarification when request is ambiguous"}
	if autonomy == "assistant" { rules = append(rules, "Always wait for explicit human confirmation before acting") }
	if autonomy == "autonomous" { rules = append(rules, "Act proactively within your permissions") }
	for _, c := range caps {
		if dangerousCapabilities[c.Name] {
			rules = append(rules, fmt.Sprintf("Require human approval before %s", c.Name))
		}
	}
	return rules
}

func matchCapabilities(descLower string, in Input) []domain.AgentCapability {
	matched := map[string]domain.AgentCapability{}
	for _, def := range catalog {
		score := 0
		for _, kw := range def.Keywords {
			if strings.Contains(descLower, kw) { score++ }
		}
		for _, a := range in.Actions {
			if strings.Contains(strings.ToLower(a), def.Name) || strings.Contains(strings.ToLower(a), strings.Join(def.Keywords, "")) { score++ }
		}
		if score > 0 {
			matched[def.Name] = domain.AgentCapability{Name: def.Name, Description: def.Description, Enabled: true}
		}
	}
	// at least one
	if len(matched) == 0 {
		matched["communication"] = domain.AgentCapability{Name: "communication", Description: "Communicate with users", Enabled: true}
	}
	out := []domain.AgentCapability{}
	for _, v := range matched { out = append(out, v) }
	return out
}

func inferResponsibilities(descLower string) []string {
	resp := []string{}
	if strings.Contains(descLower, "social") { resp = append(resp, "Create social media content") }
	if strings.Contains(descLower, "analyze") { resp = append(resp, "Analyze performance") }
	if strings.Contains(descLower, "coordinate") { resp = append(resp, "Coordinate with team") }
	if len(resp) == 0 { resp = append(resp, "Assist with requested tasks") }
	return resp
}

func inferKnowledge(descLower string) []string {
	needs := []string{}
	if strings.Contains(descLower, "social") { needs = append(needs, "Social media guidelines", "Brand voice") }
	if strings.Contains(descLower, "performance") { needs = append(needs, "Analytics data") }
	if len(needs) == 0 { needs = append(needs, "General workspace knowledge") }
	return needs
}

func pickAvatar(descLower string) string {
	if strings.Contains(descLower, "social") { return "📱" }
	if strings.Contains(descLower, "support") { return "💬" }
	if strings.Contains(descLower, "research") { return "🔍" }
	return "🤖"
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "_")
	return s
}
