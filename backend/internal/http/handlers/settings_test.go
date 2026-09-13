package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSettings_GetLLM_OpenRouterReal(t *testing.T) {
	h := NewSettingsHandler("minimax/minimax-m3:free", true, "stub", "openai/gpt-4o-mini")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings/llm", nil)
	rec := httptest.NewRecorder()
	h.GetLLM(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d body %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data struct {
			DefaultModel    string `json:"defaultModel"`
			AvailableModels []string `json:"availableModels"`
			Providers []struct {
				Name string `json:"name"`
				Real bool `json:"real"`
			} `json:"providers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v body %s", err, rec.Body.String())
	}
	if env.Data.DefaultModel != "minimax/minimax-m3:free" {
		t.Fatalf("expected minimax default got %q", env.Data.DefaultModel)
	}
	found := false
	for _, p := range env.Data.Providers {
		if p.Name == "openrouter" && p.Real {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected openrouter real, got %+v", env.Data.Providers)
	}
	foundModel := false
	for _, m := range env.Data.AvailableModels {
		if m == "minimax/minimax-m3:free" {
			foundModel = true
		}
	}
	if !foundModel {
		t.Fatalf("expected minimax in available, got %v", env.Data.AvailableModels)
	}
}

func TestSettings_GetLLM_StubFallback(t *testing.T) {
	h := NewSettingsHandler("", false, "stub", "openai/gpt-4o-mini")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings/llm", nil)
	rec := httptest.NewRecorder()
	h.GetLLM(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rec.Code)
	}
	var env struct {
		Data struct {
			Providers []struct {
				Name string `json:"name"`
				Real bool `json:"real"`
			} `json:"providers"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	for _, p := range env.Data.Providers {
		if p.Name == "openrouter" && p.Real {
			t.Fatal("openrouter must not be real without key")
		}
	}
}
