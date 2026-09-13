package team

import (
	"context"
	"testing"

	"openagent/internal/llm"
)

func TestTeamBuilder_SocialMedia(t *testing.T) {
	svc := New(llm.NewRegistry())
	preview, err := svc.Propose(context.Background(), Input{Outcome: "I want AI to manage our company's social media."})
	if err != nil { t.Fatalf("propose: %v", err) }
	if preview.Name != "Social Media Team" { t.Fatalf("expected Social Media Team, got %s", preview.Name) }
	if len(preview.Agents) != 6 { t.Fatalf("expected 6 agents, got %d", len(preview.Agents)) }
	found := map[string]bool{}
	for _, a := range preview.Agents { found[a.Name]=true }
	for _, expected := range []string{"Social Media Manager","Content Strategist","Copywriter","Creative Agent","Analytics Agent","Community Manager"} {
		if !found[expected] { t.Fatalf("missing %s", expected) }
	}
	if len(preview.Workflow)==0 { t.Fatal("workflow empty") }
	if err:=ValidatePreview(preview); err!=nil { t.Fatalf("validate: %v", err) }
}

func TestTeamBuilder_ValidateDuplicate(t *testing.T) {
	svc := New(nil)
	preview2, _ := svc.Propose(context.Background(), Input{Outcome: "test"})
	preview2.Agents = append(preview2.Agents, preview2.Agents[0]) // duplicate
	if err := ValidatePreview(preview2); err == nil { t.Fatal("should fail duplicate") }
}

func TestTeamBuilder_Generic(t *testing.T) {
	svc := New(nil)
	preview, _ := svc.Propose(context.Background(), Input{Outcome: "Manage customer support"})
	if len(preview.Agents) <2 { t.Fatal("should have at least 2 agents") }
	if preview.Name == "" { t.Fatal("name empty") }
}
