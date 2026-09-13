package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStubProvider(t *testing.T) {
	p := NewStub("stub")
	r := NewRegistry()
	r.Register(p)
	got, err := r.Get("stub")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := got.Complete(context.Background(), CompletionRequest{Model: "test", Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil || resp.Content == "" {
		t.Fatalf("stub failed: %v %v", resp, err)
	}
	if _, err := r.Get("missing"); err == nil {
		t.Fatal("should error on missing")
	}
}

func TestStubEmbedder(t *testing.T) {
	e := NewStubEmbedder(5)
	resp, err := e.Embed(context.Background(), EmbedRequest{Input: []string{"hello", "world"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Embeddings) != 2 || len(resp.Embeddings[0]) != 5 {
		t.Fatalf("embedding dims wrong: %v", resp.Embeddings)
	}
}

func TestOpenRouter_RetryOn429(t *testing.T) {
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if count < 3 {
			w.WriteHeader(429)
			w.Write([]byte(`{"error":{"message":"rate limited","code":429}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"status\":\"completed\",\"response\":\"done\"}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()
	provider := NewOpenRouter("test-key", "minimax/minimax-m3:free")
	provider.baseURL = server.URL
	provider.client = &http.Client{Timeout: 5 * time.Second}
	resp, err := provider.Complete(context.Background(), CompletionRequest{Model: "minimax/minimax-m3:free", Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("expected retry success, got %v", err)
	}
	if resp.Content == "" {
		t.Fatal("empty response")
	}
	if count != 3 {
		t.Fatalf("expected 3 attempts, got %d", count)
	}
}
