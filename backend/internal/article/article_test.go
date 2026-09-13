package article

import (
	"strings"
	"testing"
)

func TestValidateBrief_FlexiblePerArticle(t *testing.T) {
	b := Brief{Topic: "Vector DBs", Length: "short", Tone: "friendly", Format: "blog", ProjectID: "11111111-1111-1111-1111-111111111111"}
	if err := b.Validate(); err != nil {
		t.Fatalf("valid brief should pass: %v", err)
	}
	b2 := Brief{Topic: "", Length: "short", Tone: "friendly", Format: "blog"}
	if err := b2.Validate(); err == nil {
		t.Fatal("empty topic should fail")
	}
	b3 := Brief{Topic: "x", Length: "epic", Tone: "friendly", Format: "blog"}
	if err := b3.Validate(); err == nil {
		t.Fatal("invalid length should fail")
	}
}

func TestTaskTitle_Description(t *testing.T) {
	b := Brief{Topic: "  Vector DBs  ", Length: "short", Tone: "friendly", Format: "blog"}
	if got := b.TaskTitle(); got != "Article: Vector DBs" {
		t.Fatalf("TaskTitle = %q, want %q", got, "Article: Vector DBs")
	}
	desc := Brief{Topic: "Vector DBs", Length: "short", Tone: "friendly", Format: "blog"}.TaskDescription()
	for _, want := range []string{"Vector DBs", "short", "friendly", "blog"} {
		if !strings.Contains(desc, want) {
			t.Fatalf("TaskDescription = %q, want substring %q", desc, want)
		}
	}
}
