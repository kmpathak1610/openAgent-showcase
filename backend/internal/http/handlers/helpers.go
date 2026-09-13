package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
)

type envelope struct {
	Data  any `json:"data,omitempty"`
	Meta  *meta `json:"meta,omitempty"`
	Error *apiError `json:"error,omitempty"`
}

type meta struct {
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
	Total    int `json:"total,omitempty"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeData(w http.ResponseWriter, status int, data any, m *meta) {
	writeJSON(w, status, envelope{Data: data, Meta: m})
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, envelope{Error: &apiError{Code: code, Message: msg}})
}

func pagination(r *http.Request) (page, pageSize int) {
	page = 1
	pageSize = 20
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := r.URL.Query().Get("pageSize"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			pageSize = n
		}
	}
	return
}
