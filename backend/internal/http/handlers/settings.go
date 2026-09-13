package handlers

import "net/http"

type SettingsHandler struct {
	openRouterModel string
	openRouterKeySet bool
	llmProvider string
	llmModel string
}

func NewSettingsHandler(openRouterModel string, openRouterKeySet bool, llmProvider, llmModel string) *SettingsHandler {
	return &SettingsHandler{openRouterModel: openRouterModel, openRouterKeySet: openRouterKeySet, llmProvider: llmProvider, llmModel: llmModel}
}

func (h *SettingsHandler) GetLLM(w http.ResponseWriter, r *http.Request) {
	def := h.openRouterModel
	if def == "" {
		def = "openai/gpt-4o-mini"
	}
	if h.openRouterModel == "" && h.llmModel != "" {
		def = h.llmModel
	}
	models := []string{def, "openai/gpt-4o-mini", "openai/gpt-4o", "anthropic/claude-3.5-sonnet", "google/gemini-1.5-flash", "meta-llama/llama-3.1-70b"}
	seen := map[string]bool{}
	out := []string{}
	for _, m := range models {
		if m == "" || seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	providers := []map[string]any{
		{"name": "openrouter", "real": h.openRouterKeySet},
		{"name": "openai", "real": false},
		{"name": "anthropic", "real": false},
		{"name": "google", "real": false},
		{"name": "stub", "real": true},
	}
	writeData(w, http.StatusOK, map[string]any{
		"defaultModel": def,
		"availableModels": out,
		"providers": providers,
	}, nil)
}
