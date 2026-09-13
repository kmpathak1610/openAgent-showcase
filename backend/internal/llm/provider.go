package llm

import (
	"context"
	"errors"
)

// Provider-agnostic abstraction — domain code depends only on this package.

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
	Name    *string `json:"name,omitempty"`
}

type CompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"maxTokens,omitempty"`
	Temperature *float64  `json:"temperature,omitempty"`
	Tools       []ToolDef `json:"tools,omitempty"`
}

type ToolDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Schema      map[string]any `json:"schema"`
}

type CompletionResponse struct {
	Content      string     `json:"content"`
	FinishReason string     `json:"finishReason"`
	ToolCalls    []ToolCall `json:"toolCalls,omitempty"`
	Usage        Usage      `json:"usage"`
}

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Usage struct {
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens      int `json:"totalTokens"`
}

// Embedder for RAG
type EmbedRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type EmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
	Usage      Usage       `json:"usage"`
}

type Provider interface {
	Name() string
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
}

type Embedder interface {
	Embed(ctx context.Context, req EmbedRequest) (*EmbedResponse, error)
}

// Registry holds named providers; domain resolves by agent.provider field.
type Registry struct {
	providers map[string]Provider
	embedders map[string]Embedder
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
		embedders: make(map[string]Embedder),
	}
}

func (r *Registry) Register(p Provider) {
	r.providers[p.Name()] = p
}

func (r *Registry) RegisterEmbedder(name string, e Embedder) {
	r.embedders[name] = e
}

func (r *Registry) Get(name string) (Provider, error) {
	p, ok := r.providers[name]
	if !ok {
		return nil, errors.New("llm provider not found: " + name)
	}
	return p, nil
}

func (r *Registry) GetEmbedder(name string) (Embedder, error) {
	e, ok := r.embedders[name]
	if !ok {
		return nil, errors.New("embedder not found: " + name)
	}
	return e, nil
}

// Stub provider for development / tests — no external calls.
type StubProvider struct {
	name string
}

func NewStub(name string) *StubProvider { return &StubProvider{name: name} }
func (s *StubProvider) Name() string { return s.name }
func (s *StubProvider) Complete(_ context.Context, req CompletionRequest) (*CompletionResponse, error) {
	return &CompletionResponse{
		Content:      "[stub] response for model " + req.Model,
		FinishReason: "stop",
		Usage:        Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}, nil
}

type StubEmbedder struct{ dims int }

func NewStubEmbedder(dims int) *StubEmbedder { return &StubEmbedder{dims: dims} }
func (s *StubEmbedder) Embed(_ context.Context, req EmbedRequest) (*EmbedResponse, error) {
	embs := make([][]float32, len(req.Input))
	for i := range embs {
		v := make([]float32, s.dims)
		for j := range v {
			v[j] = 0.01
		}
		embs[i] = v
	}
	return &EmbedResponse{Embeddings: embs}, nil
}
