package builder

import (
	"context"
	"testing"

	"openagent/internal/domain"
	"openagent/internal/llm"
)

func TestGeneratePreview_SocialMedia(t *testing.T) {
	svc := New(llm.NewRegistry())
	in := Input{
		Description: "I need an agent that manages our company's social media accounts, creates content, analyzes performance and coordinates with other agents.",
		Responsibilities: []string{"Create content", "Analyze performance"},
		InformationSources: []string{"Social media guidelines"},
		Actions: []string{"Publish to social media", "Schedule posts"},
		AutonomyPreference: "task_executor",
	}
	preview, err := svc.GeneratePreview(context.Background(), in)
	if err != nil { t.Fatalf("generate failed: %v", err) }
	if preview.Name == "" { t.Fatal("name empty") }
	if preview.Purpose == "" { t.Fatal("purpose empty") }
	if len(preview.Capabilities) == 0 { t.Fatal("capabilities empty") }
	// should detect publish_content and require approval
	foundPublish := false
	for _, c := range preview.Capabilities {
		if c.Name == "publish_content" { foundPublish = true }
	}
	if !foundPublish { t.Fatalf("expected publish_content capability, got %v", preview.Capabilities) }
	// approval must require publish_content
	req, ok := preview.ApprovalPolicy["require_approval_for"]
	if !ok { t.Fatal("expected require_approval_for") }
	arr, ok := req.([]string)
	if !ok { t.Fatalf("wrong type %T", req) }
	found := false
	for _, v := range arr { if v == "publish_content" { found = true } }
	if !found { t.Fatalf("approval should contain publish_content, got %v", arr) }
	// autonomy should remain task_executor (not autonomous)
	if preview.AutonomyLevel != "task_executor" { t.Fatalf("expected task_executor got %s", preview.AutonomyLevel) }
}

func TestDangerousSafeguards_AutonomyDowngrade(t *testing.T) {
	svc := New(llm.NewRegistry())
	in := Input{
		Description: "publish to social media and send emails",
		Actions: []string{"Publish to social media", "Send emails"},
		AutonomyPreference: "autonomous",
	}
	preview, err := svc.GeneratePreview(context.Background(), in)
	if err != nil { t.Fatalf("generate: %v", err) }
	if preview.AutonomyLevel == "autonomous" {
		t.Fatalf("should downgrade autonomous to collaborative when dangerous, got %s", preview.AutonomyLevel)
	}
	if len(preview.Warnings) == 0 { t.Fatal("expected warnings about downgrade") }
}

func TestValidatePreview_DangerousMustRequireApproval(t *testing.T) {
	// valid preview passes
	p := &domain.AgentBuilderPreview{
		Name: "Test Agent",
		Description: "desc",
		Purpose: "purpose",
		Role: "Manager",
		Objective: "objective",
		Responsibilities: []string{"r1"},
		Capabilities: []domain.AgentCapability{{Name: "publish_content", Description: "Publish", Enabled: true}},
		ApprovalPolicy: map[string]any{"require_approval_for": []string{"publish_content"}},
		BehavioralRules: []string{"rule"},
		RecommendedTools: []string{"tool"},
		AutonomyLevel: "task_executor",
	}
	if err := ValidatePreview(p); err != nil { t.Fatalf("should pass: %v", err) }
	// missing approval should fail
	p2 := &domain.AgentBuilderPreview{
		Name: "Test Agent",
		Description: "desc",
		Purpose: "purpose",
		Role: "Manager",
		Objective: "objective",
		Responsibilities: []string{"r1"},
		Capabilities: []domain.AgentCapability{{Name: "publish_content", Enabled: true}},
		ApprovalPolicy: map[string]any{"require_approval_for": []string{}},
		BehavioralRules: []string{"rule"},
		RecommendedTools: []string{"tool"},
		AutonomyLevel: "task_executor",
	}
	if err := ValidatePreview(p2); err == nil {
		t.Fatal("should fail when dangerous capability lacks approval")
	}
	_ = &domain.AgentBuilderPreview{
		Name: "Test Agent",
		Description: "desc",
		Purpose: "purpose",
		Role: "Manager",
		Objective: "objective",
		Responsibilities: []string{"r1"},
		Capabilities: []domain.AgentCapability{{Name: "publish_content", Enabled: true}},
		ApprovalPolicy: map[string]any{"require_approval_for": []string{"publish_content"}},
		BehavioralRules: []string{"rule"},
		RecommendedTools: []string{"tool"},
		AutonomyLevel: "autonomous",
	}
}

func TestValidatePreview_Basic(t *testing.T) {
	p := &domain.AgentBuilderPreview{Name: "", AutonomyLevel: "assistant"}
	if err := ValidatePreview(p); err == nil { t.Fatal("should fail empty name") }
	p.Name = "Ok"
	p.AutonomyLevel = "invalid"
	if err := ValidatePreview(p); err == nil { t.Fatal("should fail invalid autonomy") }
	p.AutonomyLevel = "assistant"
	p.Description = "desc"
	p.Purpose = "purpose"
	p.Role = "role"
	p.Objective = "objective"
	p.Responsibilities = []string{"r"}
	p.Capabilities = []domain.AgentCapability{{Name: "communication", Enabled: true}}
	p.ApprovalPolicy = map[string]any{"require_approval_for": []string{}}
	p.BehavioralRules = []string{"rule"}
	p.RecommendedTools = []string{"tool"}
	if err := ValidatePreview(p); err != nil { t.Fatalf("should pass minimal: %v", err) }
}

func TestBuilderOutput_ResponsibilitiesAndKnowledge(t *testing.T) {
	svc := New(nil)
	in := Input{
		Description: "research topics and create reports",
		AutonomyPreference: "assistant",
	}
	preview, _ := svc.GeneratePreview(context.Background(), in)
	if len(preview.Responsibilities) == 0 { t.Fatal("should infer responsibilities") }
	if len(preview.KnowledgeNeeds) == 0 { t.Fatal("should infer knowledge") }
	if preview.Role == "" { t.Fatal("role empty") }
	if preview.Instructions == "" { t.Fatal("instructions empty") }
}

func TestGeneratePreview_ArticleResearcher(t *testing.T) {
	svc := New(nil)
	in := Input{
		Description: "Research information for article on vector databases from docs and web",
		Responsibilities: []string{"Collect 5-10 sources", "Emit outline with citations"},
		InformationSources: []string{"Product Documentation", "Brand Guidelines"},
		Actions: []string{"research", "search knowledge"},
		AutonomyPreference: "collaborative",
	}
	preview, err := svc.GeneratePreview(context.Background(), in)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	found := false
	for _, c := range preview.Capabilities {
		if c.Name == "research" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected research capability, got %v", preview.Capabilities)
	}
	if preview.AutonomyLevel != "collaborative" {
		t.Fatalf("expected collaborative got %s", preview.AutonomyLevel)
	}
}

func TestGeneratePreview_ArticleWriter(t *testing.T) {
	svc := New(nil)
	in := Input{
		Description: "Write blog article content with citations in friendly tone",
		Responsibilities: []string{"Expand outline to draft", "Add citations"},
		InformationSources: []string{"Research bundle", "Brand voice"},
		Actions: []string{"write content", "create task"},
		AutonomyPreference: "collaborative",
	}
	preview, err := svc.GeneratePreview(context.Background(), in)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	found := false
	for _, c := range preview.Capabilities {
		if c.Name == "content_creation" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected content_creation capability, got %v", preview.Capabilities)
	}
}
