package ratelimit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type bucket struct {
	count    int
	windowStart time.Time
}

// RateLimiter provides in-memory per-key rate limiting with sliding windows.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
}

// NewRateLimiter creates a new in-memory rate limiter.
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*bucket),
	}
}

// Allow checks whether a request with the given key is allowed.
// Returns whether the request is allowed, the remaining requests in the window,
// and the time when the window resets.
func (rl *RateLimiter) Allow(key string, limit int, window time.Duration) (allowed bool, remaining int, resetAt time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, exists := rl.buckets[key]

	if !exists || now.Sub(b.windowStart) >= window {
		// New window
		b = &bucket{
			count:       1,
			windowStart: now,
		}
		rl.buckets[key] = b
		return true, limit - 1, now.Add(window)
	}

	resetAt = b.windowStart.Add(window)

	if b.count >= limit {
		return false, 0, resetAt
	}

	b.count++
	return true, limit - b.count, resetAt
}

// Middleware returns an HTTP middleware that enforces rate limits.
// keyFunc extracts the rate limit key from the request (e.g., IP address, user ID).
func (rl *RateLimiter) Middleware(limit int, window time.Duration, keyFunc func(r *http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFunc(r)
			allowed, remaining, resetAt := rl.Allow(key, limit, window)

			// Always set rate limit headers
			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
			w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetAt.Unix()))

			if !allowed {
				retryAfter := int(time.Until(resetAt).Seconds())
				if retryAfter < 1 {
					retryAfter = 1
				}
				w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"success": false,
					"error": map[string]string{
						"code":    "RATE_LIMITED",
						"message": "Too many requests. Please try again later.",
					},
					"data": nil,
					"meta": map[string]interface{}{
						"timestamp":  time.Now().UTC().Format(time.RFC3339),
						"request_id": r.Header.Get("X-Request-ID"),
					},
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
