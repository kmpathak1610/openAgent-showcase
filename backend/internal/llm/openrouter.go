package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

// OpenRouterProvider implements Provider via OpenRouter's OpenAI-compatible API
// It is provider-agnostic and can route to OpenAI, Anthropic, Google, etc. via model prefix
type OpenRouterProvider struct {
	apiKey string
	model  string
	baseURL string
	client *http.Client
}

func NewOpenRouter(apiKey, model string) *OpenRouterProvider {
	if model == "" { model = "anthropic/claude-3.5-sonnet" }
	return &OpenRouterProvider{
		apiKey: apiKey,
		model: model,
		baseURL: "https://openrouter.ai/api/v1",
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (o *OpenRouterProvider) Name() string { return "openrouter" }
func (o *OpenRouterProvider) Model() string { return o.model }

func (o *OpenRouterProvider) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		resp, err := o.doComplete(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "rate_limit") || strings.Contains(err.Error(), "rate-limit") {
			// Check context before backoff
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			backoff := time.Duration(1<<attempt)*time.Second + time.Duration(rand.Intn(500))*time.Millisecond
			if backoff > 10*time.Second {
				backoff = 10 * time.Second
			}
			select {
			case <-time.After(backoff):
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		break
	}
	return nil, lastErr
}

func (o *OpenRouterProvider) doComplete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	model := req.Model
	if model == "" {
		model = o.model
	}
	payload := map[string]any{
		"model":    model,
		"messages": req.Messages,
	}
	if req.MaxTokens > 0 {
		payload["max_tokens"] = req.MaxTokens
	}
	if req.Temperature != nil {
		payload["temperature"] = *req.Temperature
	}
	if len(req.Tools) > 0 {
		payload["tools"] = req.Tools
	}
	body, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	httpReq.Header.Set("HTTP-Referer", "https://openagent.local")
	httpReq.Header.Set("X-Title", "OpenAgent")
	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("openrouter %d: %s", resp.StatusCode, string(respBody))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("no choices")
	}
	return &CompletionResponse{
		Content:      parsed.Choices[0].Message.Content,
		FinishReason: parsed.Choices[0].FinishReason,
		Usage:        Usage{PromptTokens: parsed.Usage.PromptTokens, CompletionTokens: parsed.Usage.CompletionTokens, TotalTokens: parsed.Usage.TotalTokens},
	}, nil
}

func (o *OpenRouterProvider) Embed(ctx context.Context, req EmbedRequest) (*EmbedResponse, error) {
	// OpenRouter embeddings via same endpoint (use OpenAI embeddings model)
	model := req.Model
	if model == "" { model = "openai/text-embedding-3-small" }
	payload := map[string]any{"model": model, "input": req.Input}
	body, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil { return nil, err }
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	resp, err := o.client.Do(httpReq)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("openrouter embed %d: %s", resp.StatusCode, string(respBody))
	}
	var parsed struct{
		Data []struct{ Embedding []float32 `json:"embedding"` } `json:"data"`
		Usage Usage `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil { return nil, err }
	embs := make([][]float32, len(parsed.Data))
	for i, d := range parsed.Data { embs[i]=d.Embedding }
	return &EmbedResponse{Embeddings: embs, Usage: parsed.Usage}, nil
}
