package team

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"openagent/internal/domain"
	"openagent/internal/llm"
)

type Input struct {
	Outcome string `json:"outcome"` // user describes business outcome
}

type Service struct {
	registry *llm.Registry
}

func New(reg *llm.Registry) *Service { return &Service{registry: reg} }

func (s *Service) Propose(ctx context.Context, in Input) (*domain.TeamBuilderPreview, error) {
	// Try LLM-driven generation first if real provider available
	if s.registry != nil {
		preview, genErr := s.proposeViaLLM(ctx, in)
		if genErr == nil && preview != nil {
			if verr := ValidatePreview(preview); verr == nil {
				return preview, nil
			} else {
				// retry with repair
				if repaired, rerr := s.proposeViaLLMRepair(ctx, in, verr); rerr == nil && repaired != nil {
					if verr2 := ValidatePreview(repaired); verr2 == nil {
						return repaired, nil
					} else {
						genErr = verr2
					}
				} else {
					genErr = verr
				}
				if os.Getenv("ENV") == "production" {
					return nil, fmt.Errorf("team builder validation failed: %w", genErr)
				}
			}
		} else if genErr != nil && os.Getenv("ENV") == "production" {
			if strings.Contains(genErr.Error(), "no real LLM") || strings.Contains(genErr.Error(), "invalid") {
				return nil, fmt.Errorf("team builder LLM failed in production: %w", genErr)
			}
		}
	}
	if os.Getenv("ENV") == "production" && s.registry != nil {
		for _, name := range []string{"openrouter", "openai", "anthropic", "google"} {
			if p, err := s.registry.Get(name); err == nil && p.Name() != "stub" {
				return nil, fmt.Errorf("LLM generation failed and deterministic fallback not allowed in production")
			}
		}
	}
	lower := strings.ToLower(in.Outcome)
	preview := &domain.TeamBuilderPreview{}

	// Deterministic fallback (dev/test or LLM unavailable) — kept for offline use
	if strings.Contains(lower, "social media") {
		preview.Name = "Social Media Team"
		preview.Objective = "Manage company's social media presence end-to-end"
		preview.Description = "AI team to manage social media accounts, create content, analyze performance and coordinate"
		preview.Agents = []domain.TeamAgentProposal{
			{Name: "Social Media Manager", Role: "Social Media Manager", Responsibilities: "Own social strategy, coordinate team, approve content, manage publishing", Dependencies: []string{}, Tools: []string{"publish_social_post", "read_social_analytics", "create_task"}, Knowledge: []string{"Brand Guidelines", "Campaign History"}, Autonomy: "collaborative", ApprovalPolicy: "high"},
			{Name: "Content Strategist", Role: "Content Strategist", Responsibilities: "Develop content strategy and campaign plans", Dependencies: []string{"Social Media Manager"}, Tools: []string{"web_search", "create_task"}, Knowledge: []string{"Product Documentation", "Brand Guidelines"}, Autonomy: "task_executor", ApprovalPolicy: "medium"},
			{Name: "Copywriter", Role: "Copywriter", Responsibilities: "Draft post copy and variations", Dependencies: []string{"Content Strategist"}, Tools: []string{"web_search"}, Knowledge: []string{"Brand Guidelines"}, Autonomy: "task_executor", ApprovalPolicy: "low"},
			{Name: "Creative Agent", Role: "Creative Agent", Responsibilities: "Create visuals and media assets", Dependencies: []string{"Content Strategist"}, Tools: []string{"upload_file"}, Knowledge: []string{"Brand Guidelines"}, Autonomy: "task_executor", ApprovalPolicy: "low"},
			{Name: "Analytics Agent", Role: "Analytics Agent", Responsibilities: "Analyze performance and report metrics", Dependencies: []string{"Social Media Manager"}, Tools: []string{"read_social_analytics"}, Knowledge: []string{"Analytics data"}, Autonomy: "task_executor", ApprovalPolicy: "low"},
			{Name: "Community Manager", Role: "Community Manager", Responsibilities: "Engage community, moderate comments", Dependencies: []string{"Social Media Manager"}, Tools: []string{"send_channel_message"}, Knowledge: []string{"Community Guidelines"}, Autonomy: "collaborative", ApprovalPolicy: "medium"},
		}
		preview.Workflow = []domain.WorkflowStep{
			{From: "human", To: "Social Media Manager", Action: "create_campaign"},
			{From: "Social Media Manager", To: "Content Strategist", Action: "delegate_plan"},
			{From: "Content Strategist", To: "Copywriter", Action: "delegate_copy"},
			{From: "Content Strategist", To: "Creative Agent", Action: "delegate_creative"},
			{From: "Copywriter", To: "Social Media Manager", Action: "review"},
			{From: "Creative Agent", To: "Social Media Manager", Action: "review"},
			{From: "Social Media Manager", To: "Analytics Agent", Action: "request_analysis"},
			{From: "Analytics Agent", To: "Social Media Manager", Action: "report"},
			{From: "Social Media Manager", To: "human", Action: "request_approval"},
			{From: "human", To: "Social Media Manager", Action: "publish"},
		}
		preview.CommunicationRules = []string{"Manager coordinates, Strategist plans, Copywriter and Creative work in parallel, Analytics reports to Manager"}
		preview.DelegationRules = []string{"Manager delegates plan to Strategist, Strategist delegates copy/creative, Manager delegates analytics, max depth 3"}
		preview.Permissions = map[string]any{"team": "project", "channels": "read/write"}
		preview.ApprovalPolicy = map[string]any{"publish": "require_approval", "external": "require_approval"}
		return preview, nil
	}

	// Generic fallback for other outcomes
	preview.Name = generateTeamName(in.Outcome)
	preview.Objective = in.Outcome
	preview.Description = "AI team to achieve: " + in.Outcome
	preview.Agents = []domain.TeamAgentProposal{
		{Name: "Coordinator", Role: "Coordinator", Responsibilities: "Coordinate team and manage tasks", Tools: []string{"create_task", "send_channel_message"}, Knowledge: []string{"Project docs"}, Autonomy: "collaborative", ApprovalPolicy: "medium"},
		{Name: "Specialist", Role: "Specialist", Responsibilities: "Execute core tasks", Tools: []string{"web_search"}, Knowledge: []string{"General knowledge"}, Autonomy: "task_executor", ApprovalPolicy: "low"},
		{Name: "Analyst", Role: "Analyst", Responsibilities: "Analyze results and report", Tools: []string{"read_social_analytics"}, Knowledge: []string{"Analytics"}, Autonomy: "task_executor", ApprovalPolicy: "low"},
	}
	preview.Workflow = []domain.WorkflowStep{
		{From: "human", To: "Coordinator", Action: "start"},
		{From: "Coordinator", To: "Specialist", Action: "delegate"},
		{From: "Specialist", To: "Analyst", Action: "handoff"},
		{From: "Analyst", To: "Coordinator", Action: "report"},
		{From: "Coordinator", To: "human", Action: "approval"},
	}
	preview.CommunicationRules = []string{"Coordinator delegates, Specialist executes, Analyst reports"}
	preview.DelegationRules = []string{"max depth 3, retry 3, timeout 5m"}
	preview.Permissions = map[string]any{"scope": "project"}
	preview.ApprovalPolicy = map[string]any{"default": "medium"}

	return preview, nil
}

func (s *Service) proposeViaLLM(ctx context.Context, in Input) (*domain.TeamBuilderPreview, error) {
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
		return nil, fmt.Errorf("no real LLM")
	}
	if model == "" {
		model = "openai/gpt-4o-mini"
	}
	prompt := fmt.Sprintf(`You are an AI team architect. Convert the user's outcome into a structured team proposal.

Outcome: "%s"

Respond ONLY with valid JSON matching:
{
  "name": "Team Name",
  "objective": "Team objective",
  "description": "Description",
  "agents": [
    {"name": "Agent Name", "role": "Role", "responsibilities": "What they do", "dependencies": [], "tools": ["tool1"], "knowledge": ["need1"], "autonomy": "task_executor", "approvalPolicy": "low|medium|high"}
  ],
  "workflow": [
    {"from": "human", "to": "Manager", "action": "create_campaign"},
    {"from": "Manager", "to": "Member", "action": "delegate"}
  ],
  "communicationRules": ["rule"],
  "delegationRules": ["rule"],
  "permissions": {"scope": "project"},
  "approvalPolicy": {"default": "medium"}
}

Valid tools: publish_social_post, read_social_analytics, web_search, create_task, send_channel_message, upload_file, http_request, send_email, create_calendar_event
Valid autonomy: task_executor, collaborative, autonomous
High-risk tools (publish_social_post, send_email) must have approvalPolicy high.

Keep to 3-5 agents, workflow should reflect dependencies.`, in.Outcome)
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: "You are a helpful AI architect. Respond with valid JSON only, no markdown."},
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
	var preview domain.TeamBuilderPreview
	if err := json.Unmarshal([]byte(content), &preview); err != nil {
		return nil, err
	}
	return &preview, nil
}

func (s *Service) proposeViaLLMRepair(ctx context.Context, in Input, prevErr error) (*domain.TeamBuilderPreview, error) {
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
	prompt := fmt.Sprintf(`Previous attempt failed validation: %v. Repair JSON to match required schema. Outcome: "%s" Respond ONLY with valid JSON matching schema { "name": "Team Name", "objective": "...", "description": "...", "agents": [{"name":"...","role":"...","responsibilities":"...","dependencies":[],"tools":[],"knowledge":[],"autonomy":"task_executor","approvalPolicy":"low"}], "workflow":[{"from":"human","to":"Manager","action":"create_campaign"}], "communicationRules":["rule"], "delegationRules":["rule"], "permissions":{"scope":"project"}, "approvalPolicy":{"default":"medium"} }`, prevErr, in.Outcome)
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: "You are a helpful AI architect. Respond with valid JSON only. Fix validation errors."},
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
	var preview domain.TeamBuilderPreview
	if err := json.Unmarshal([]byte(content), &preview); err != nil {
		return nil, err
	}
	return &preview, nil
}

func generateTeamName(outcome string) string {
	words := strings.Fields(outcome)
	if len(words) > 4 { words = words[:4] }
	name := strings.Title(strings.ToLower(strings.Join(words, " ")))
	if !strings.Contains(strings.ToLower(name), "team") {
		name += " Team"
	}
	return name
}

func ValidatePreview(p *domain.TeamBuilderPreview) error {
	if p.Name == "" { return fmt.Errorf("team name required") }
	if strings.TrimSpace(p.Objective) == "" { return fmt.Errorf("objective required") }
	if strings.TrimSpace(p.Description) == "" { return fmt.Errorf("description required") }
	if len(p.Agents) == 0 { return fmt.Errorf("at least one agent required") }
	if len(p.Workflow) == 0 { return fmt.Errorf("workflow required") }
	seen := map[string]bool{}
	for _, a := range p.Agents {
		if a.Name == "" { return fmt.Errorf("agent name required") }
		if seen[a.Name] { return fmt.Errorf("duplicate agent %s", a.Name) }
		seen[a.Name]=true
		if a.Role == "" { return fmt.Errorf("agent %s role required", a.Name) }
		if a.Responsibilities == "" { return fmt.Errorf("agent %s responsibilities required", a.Name) }
		if a.Autonomy == "" { return fmt.Errorf("agent %s autonomy required", a.Name) }
		if !domain.ValidAutonomyLevels[a.Autonomy] { return fmt.Errorf("agent %s invalid autonomy %s", a.Name, a.Autonomy) }
	}
	return nil
}
