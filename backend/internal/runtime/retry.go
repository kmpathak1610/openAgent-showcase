package runtime

import (
	"context"
	"fmt"
	"openagent/internal/llm"
)

func (r *Runtime) retryDecision(ctx context.Context, provider llm.Provider, model string, hardenedSystem string, rawOutput string, firstErr error, runIDStr string) (*AgentDecision, error) {
	decision, err := ParseAgentDecision(rawOutput)
	if err == nil {
		return decision, nil
	}
	for retry := 1; retry <= MaxDecisionRetries; retry++ {
		repairPrompt := fmt.Sprintf("Validation failed: %v. Provide ONLY valid JSON for AgentDecision.", err)
		msgs := []llm.Message{{Role: llm.RoleSystem, Content: hardenedSystem}, {Role: llm.RoleUser, Content: repairPrompt}}
		resp, repErr := provider.Complete(ctx, llm.CompletionRequest{Model: model, Messages: msgs})
		if repErr != nil {
			return nil, repErr
		}
		decision, err = ParseAgentDecision(resp.Content)
		if err == nil {
			return decision, nil
		}
	}
	return nil, fmt.Errorf("invalid decision after %d retries: %w", MaxDecisionRetries, err)
}
