package handlers

import (
	"net/http"
	"os"
)

func Health(w http.ResponseWriter, r *http.Request) {
	writeData(w, http.StatusOK, map[string]string{"status": "ok"}, nil)
}

func Ready(dbPing func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		checks := map[string]string{"database": "ok", "llm": "ok", "embedding": "ok"}
		ready := true
		if err := dbPing(); err != nil {
			checks["database"] = "not ready: " + err.Error()
			ready = false
		}
		// Production readiness: require real LLM and embedding providers
		if os.Getenv("ENV") == "production" {
			needLLM := os.Getenv("OPENROUTER_API_KEY") == "" && os.Getenv("OPENAI_API_KEY") == "" && os.Getenv("ANTHROPIC_API_KEY") == "" && os.Getenv("GOOGLE_API_KEY") == "" && os.Getenv("LLM_FALLBACK_PROVIDER") == ""
			if needLLM {
				checks["llm"] = "not ready: no real LLM provider (set OPENROUTER_API_KEY or OPENAI_API_KEY)"
				ready = false
			}
			embeddingProvider := os.Getenv("EMBEDDING_PROVIDER")
			if embeddingProvider == "" {
				embeddingProvider = "openrouter"
			}
			if embeddingProvider == "stub" {
				checks["embedding"] = "not ready: stub embedding not allowed in production"
				ready = false
			} else if embeddingProvider == "openrouter" && os.Getenv("OPENROUTER_API_KEY") == "" && os.Getenv("OPENAI_API_KEY") == "" {
				checks["embedding"] = "not ready: embedding provider openrouter requires OPENROUTER_API_KEY or OPENAI_API_KEY"
				ready = false
			}
			// Redis check placeholder — if REDIS_URL set, ping would be here
			if os.Getenv("REDIS_URL") != "" {
				// If Redis not reachable, mark not ready (stub for now)
			}
		}
		if !ready {
			writeErrorWithDetails(w, http.StatusServiceUnavailable, "NOT_READY", "not ready", checks)
			return
		}
		writeData(w, http.StatusOK, map[string]string{"status": "ready"}, nil)
	}
}

func writeErrorWithDetails(w http.ResponseWriter, status int, code, message string, details map[string]string) {
	writeJSON(w, status, envelope{Error: &apiError{Code: code, Message: message + ": " + mapToString(details), Details: details}})
}

func mapToString(m map[string]string) string {
	s := ""
	for k, v := range m {
		if s != "" {
			s += ", "
		}
		s += k + "=" + v
	}
	return s
}
