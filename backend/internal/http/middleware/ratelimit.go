package middleware

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type RateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*limiterEntry
	r        rate.Limit
	burst    int
}

func NewRateLimiter(r rate.Limit, burst int) *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*limiterEntry),
		r:        r,
		burst:    burst,
	}
}

func (rl *RateLimiter) getLimiter(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	// cleanup old entries periodically
	if len(rl.limiters) > 1000 {
		for k, v := range rl.limiters {
			if time.Since(v.lastSeen) > 10*time.Minute {
				delete(rl.limiters, k)
			}
		}
	}
	entry, ok := rl.limiters[key]
	if !ok {
		entry = &limiterEntry{limiter: rate.NewLimiter(rl.r, rl.burst)}
		rl.limiters[key] = entry
	}
	entry.lastSeen = time.Now()
	return entry.limiter
}

func RateLimit(rl *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// key by IP + org (if authenticated, use org, else IP)
			key := r.RemoteAddr
			if org := r.Header.Get("X-Organization-ID"); org != "" {
				key = org
			} else if claims, ok := GetClaims(r.Context()); ok {
				key = claims.OrganizationID.String()
			}
			// also per-path to avoid one endpoint starving others
			key = key + ":" + r.URL.Path
			if !rl.getLimiter(key).Allow() {
				http.Error(w, `{"error":{"code":"RATE_LIMITED","message":"too many requests"}}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
