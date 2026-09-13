package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"openagent/internal/auth"
)

func TestAuthMiddlewareRejectsMissing(t *testing.T) {
	svc := auth.New("test-secret-32-chars-long-enough-12345")
	h := Auth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("expected 401 got %d", rr.Code)
	}
}

func TestAuthMiddlewareAcceptsValid(t *testing.T) {
	svc := auth.New("test-secret-32-chars-long-enough-12345")
	uid := uuid.New()
	oid := uuid.New()
	tok, _ := svc.CreateToken(uid, oid, "member", "a@b.com", time.Hour)
	h := Auth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r.Context())
		if !ok || claims.UserID != uid {
			t.Error("claims not propagated")
		}
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("expected 200 got %d body %s", rr.Code, rr.Body.String())
	}
}
