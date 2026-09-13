package knowledge

import (
	"testing"
	"strings"
)

func TestChunkText_Basic(t *testing.T) {
	content := strings.Repeat("This is a test sentence. ", 100) // ~2400 chars -> should chunk into 2
	chunks := ChunkText(content, 500, 50)
	if len(chunks) < 2 { t.Fatalf("expected at least 2 chunks, got %d", len(chunks)) }
	// ensure no chunk exceeds limit
	for _, ch := range chunks {
		if EstimateTokens(ch) > 600 { t.Fatalf("chunk too large: %d tokens", EstimateTokens(ch)) }
	}
	// ensure overlap: second chunk should contain part of first
	if len(chunks) >=2 && !strings.Contains(chunks[1], "test sentence") {
		t.Fatalf("expected overlap")
	}
}

func TestChunkText_Short(t *testing.T) {
	chunks := ChunkText("short text", 500, 50)
	if len(chunks)!=1 || chunks[0]!="short text" { t.Fatalf("short text should be single chunk") }
}

func TestNormalizeContent(t *testing.T) {
	in := "  hello   \r\n\n  world \n\n\n test "
	out := NormalizeContent(in)
	if strings.Contains(out, "\r") { t.Fatal("should remove CR") }
	if strings.Contains(out, "\n\n\n") { t.Fatal("should collapse triple newlines") }
	if out[0] == ' ' { t.Fatal("should trim") }
}

func TestEstimateTokens(t *testing.T) {
	if EstimateTokens("abcd") !=1 { t.Fatalf("4 chars ~1 token") }
	if EstimateTokens(strings.Repeat("a", 4000)) !=1000 { t.Fatalf("4000 chars ~1000 tokens") }
}

func TestStripHTML(t *testing.T) {
	html := "<h1>Title</h1><p>Hello <b>world</b></p>"
	text := stripHTML(html)
	if !strings.Contains(text, "Title") || !strings.Contains(text, "world") { t.Fatalf("strip failed: %s", text) }
	if strings.Contains(text, "<") { t.Fatalf("tag not stripped") }
}

func TestRealisticDocuments_Chunking(t *testing.T) {
	docs := []string{
		"Brand Guidelines: Our brand voice is friendly, professional, and concise. Use active voice. Colors: #0f1117 primary, #3dd68c accent. Logo clear space 16px. Tone: helpful, not salesy.",
		"Product Documentation: API endpoints: POST /api/v1/agents, GET /api/v1/knowledge/search. Auth via Bearer JWT. Rate limit 100/min. Error codes: 400 validation, 401 unauthorized, 403 forbidden.",
		"Campaign History Q1: Launched winter campaign, CTR 2.3%, conversion 1.1%. Top channel: Instagram. Learnings: video outperforms image 40%. Next: test UGC.",
	}
	for i, d := range docs {
		chunks := ChunkText(d, 200, 20)
		if len(chunks)==0 { t.Fatalf("doc %d: no chunks", i)}
		for _, ch := range chunks {
			if len(ch) ==0 { t.Fatal("empty chunk") }
		}
	}
}
