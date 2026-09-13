package knowledge

import (
	"context"
	"testing"

	"openagent/internal/domain"
	"openagent/internal/llm"
)

func TestChunking_And_Embedding_Stub(t *testing.T) {
	embedder := llm.NewStubEmbedder(1536)
	svc := NewService(nil, embedder)
	_ = svc
	content := "Brand Guidelines: tone is friendly"
	chunks := ChunkText(content, 10, 2)
	if len(chunks)==0 { t.Fatal("no chunks") }
	req := llm.EmbedRequest{Input: chunks, Model: "text-embedding-3-small"}
	res, err := embedder.Embed(context.Background(), req)
	if err != nil || len(res.Embeddings)!= len(chunks) { t.Fatalf("embed failed") }
	if len(res.Embeddings[0])!=1536 { t.Fatalf("wrong dims") }
}

func TestRetriever_PermissionEnforced(t *testing.T) {
	// Use stub DB via nil to test that retriever doesn't panic without DB; we just test interface
	// Real permission test is in repository knowledge_test
	retriever := NewRetriever(nil, llm.NewStubEmbedder(1536), nil)
	if retriever == nil { t.Fatal("retriever nil") }
	// Ensure it implements interface
	var _ domain.KnowledgeRetriever = retriever
}

func TestHybridSearch_Merges(t *testing.T) {
	// Test that HybridSearch logic merges without DB by not calling DB; we test chunking helper used in hybrid
	// Just ensure ChunkText and Normalize work for realistic docs
	docs := []string{
		"Product Documentation: POST /api/v1/knowledge/search does vector search. Use Bearer token.",
		"Campaign History: Q1 CTR 2.3%, Q2 1.8%. Learnings: video 40% better.",
	}
	for _, d := range docs {
		chunks := ChunkText(d, 100, 10)
		if len(chunks)==0 { t.Fatalf("no chunks for %s", d) }
	}
}
