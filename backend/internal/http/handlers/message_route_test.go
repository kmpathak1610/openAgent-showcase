package handlers

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"openagent/internal/domain"
)

func mkAgent(name, slug string) *domain.Agent {
	return &domain.Agent{ID: uuid.New(), Name: name, Slug: slug}
}

func TestPickGeneralAgent_AssistantFallback(t *testing.T) {
	agents := []*domain.Agent{
		mkAgent("Other", "other"),
		mkAgent("Assistant", "openagent-assistant"),
	}
	picked := pickGeneralAgent(agents, "hello no mention here")
	if picked == nil {
		t.Fatal("expected agent, got nil")
	}
	if picked.Slug != "openagent-assistant" {
		t.Fatalf("expected openagent-assistant fallback, got %q", picked.Slug)
	}
}

func TestPickGeneralAgent_FirstFallback(t *testing.T) {
	agents := []*domain.Agent{
		mkAgent("First", "first"),
		mkAgent("Second", "second"),
	}
	picked := pickGeneralAgent(agents, "no mentions at all")
	if picked == nil {
		t.Fatal("expected agent, got nil")
	}
	if picked.Slug != "first" {
		t.Fatalf("expected first agent fallback, got %q", picked.Slug)
	}
}

func TestPickGeneralAgent_MentionParity(t *testing.T) {
	agents := []*domain.Agent{
		mkAgent("Data Helper", "data-helper-xyz"),
		mkAgent("Assistant", "openagent-assistant"),
	}
	cases := []string{
		"hey @data-helper-xyz help", // slug exact (different from name-derived, tests slug path)
		"hey @Data Helper help",     // name with space tested via normalize? mention regex splits, so use dash/underscore variants below
		"hey @data-helper help",     // name with '-' (parity with project path)
		"hey @data_helper help",     // name with '_' (parity with project path)
	}
	// First case must pick Data Helper via slug
	if got := pickGeneralAgent(agents, cases[0]); got == nil || got.Name != "Data Helper" {
		t.Fatalf("slug mention failed, got %+v", got)
	}
	// Dash variant must pick Data Helper (parity)
	if got := pickGeneralAgent(agents, cases[2]); got == nil || got.Name != "Data Helper" {
		t.Fatalf("dash mention parity failed, got %+v", got)
	}
	// Underscore variant must pick Data Helper (parity)
	if got := pickGeneralAgent(agents, cases[3]); got == nil || got.Name != "Data Helper" {
		t.Fatalf("underscore mention parity failed, got %+v", got)
	}
}

func TestPickGeneralAgent_NilEmpty(t *testing.T) {
	if got := pickGeneralAgent(nil, "hello"); got != nil {
		t.Fatalf("expected nil for nil agents, got %+v", got)
	}
	if got := pickGeneralAgent([]*domain.Agent{}, "hello"); got != nil {
		t.Fatalf("expected nil for empty agents, got %+v", got)
	}
}

func TestTruncateTitle_RuneSafe(t *testing.T) {
	// 100 runes of multibyte emoji + ascii mix
	long := strings.Repeat("a", 70) + strings.Repeat("🏠", 20) // 90 runes
	got := truncateTitle(long)
	runes := []rune(got)
	if len(runes) > 80 {
		t.Fatalf("expected <=80 runes, got %d", len(runes))
	}
	if len(got) == 0 {
		t.Fatal("expected non-empty title")
	}
	// Must be valid UTF-8 (no broken rune): range should not yield RuneError for our input
	for _, r := range got {
		if r == '\uFFFD' {
			t.Fatal("broken rune in truncated title")
		}
	}
	// Short stays trimmed
	if truncateTitle("  hi  ") != "hi" {
		t.Fatalf("expected trimmed short title, got %q", truncateTitle("  hi  "))
	}
	// Exactly 80 runes stays
	exact := strings.Repeat("x", 80)
	if truncateTitle(exact) != exact {
		t.Fatal("exact 80 should be unchanged")
	}
	// 81 runes cuts to 80
	over := strings.Repeat("y", 81)
	if len([]rune(truncateTitle(over))) != 80 {
		t.Fatal("81 runes should cut to 80")
	}
}

func TestShouldSkipGeneralRoute(t *testing.T) {
	if shouldSkipGeneralRoute(nil) {
		t.Fatal("nil thread should not skip")
	}
	tid := uuid.New()
	if !shouldSkipGeneralRoute(&tid) {
		t.Fatal("non-nil thread should skip to avoid task explosion")
	}
}
