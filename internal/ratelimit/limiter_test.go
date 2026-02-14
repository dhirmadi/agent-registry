package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestAllowWithinLimit(t *testing.T) {
	rl := NewRateLimiter()

	for i := 0; i < 5; i++ {
		allowed, remaining, _ := rl.Allow("test-key", 5, time.Minute)
		if !allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
		expectedRemaining := 4 - i
		if remaining != expectedRemaining {
			t.Fatalf("request %d: expected remaining=%d, got %d", i+1, expectedRemaining, remaining)
		}
	}
}

func TestBlockOverLimit(t *testing.T) {
	rl := NewRateLimiter()

	// Use up the limit
	for i := 0; i < 5; i++ {
		allowed, _, _ := rl.Allow("test-key", 5, time.Minute)
		if !allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	// Next request should be blocked
	allowed, remaining, resetAt := rl.Allow("test-key", 5, time.Minute)
	if allowed {
		t.Fatal("request over limit should be blocked")
	}
	if remaining != 0 {
		t.Fatalf("expected remaining=0, got %d", remaining)
	}
	if resetAt.Before(time.Now()) {
		t.Fatal("resetAt should be in the future")
	}
}

func TestDifferentKeysIndependent(t *testing.T) {
	rl := NewRateLimiter()

	// Exhaust key1
	for i := 0; i < 3; i++ {
		rl.Allow("key1", 3, time.Minute)
	}
	allowed, _, _ := rl.Allow("key1", 3, time.Minute)
	if allowed {
		t.Fatal("key1 should be blocked")
	}

	// key2 should still work
	allowed, _, _ = rl.Allow("key2", 3, time.Minute)
	if !allowed {
		t.Fatal("key2 should be allowed (independent of key1)")
	}
}

func TestWindowReset(t *testing.T) {
	rl := NewRateLimiter()

	// Use a very short window
	window := 50 * time.Millisecond

	// Exhaust the limit
	for i := 0; i < 3; i++ {
		rl.Allow("reset-key", 3, window)
	}
	allowed, _, _ := rl.Allow("reset-key", 3, window)
	if allowed {
		t.Fatal("should be blocked after exhausting limit")
	}

	// Wait for window to expire
	time.Sleep(60 * time.Millisecond)

	// Should be allowed again
	allowed, remaining, _ := rl.Allow("reset-key", 3, window)
	if !allowed {
		t.Fatal("should be allowed after window reset")
	}
	if remaining != 2 {
		t.Fatalf("expected remaining=2 after reset, got %d", remaining)
	}
}

func TestConcurrentAccess(t *testing.T) {
	rl := NewRateLimiter()
	limit := 100

	var wg sync.WaitGroup
	allowedCount := 0
	var mu sync.Mutex

	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed, _, _ := rl.Allow("concurrent-key", limit, time.Minute)
			if allowed {
				mu.Lock()
				allowedCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if allowedCount != limit {
		t.Fatalf("expected exactly %d allowed requests, got %d", limit, allowedCount)
	}
}

func TestMiddleware(t *testing.T) {
	rl := NewRateLimiter()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	keyFunc := func(r *http.Request) string {
		return r.RemoteAddr
	}

	mw := rl.Middleware(3, time.Minute, keyFunc)
	wrapped := mw(handler)

	// First 3 requests should succeed
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:1234"
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}

		// Check rate limit headers
		if w.Header().Get("X-RateLimit-Limit") != "3" {
			t.Fatalf("request %d: expected X-RateLimit-Limit=3, got %s", i+1, w.Header().Get("X-RateLimit-Limit"))
		}
	}

	// 4th request should be rate limited
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}

	// Should have Retry-After header
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header on 429 response")
	}
}

func TestMiddlewareHeaders(t *testing.T) {
	rl := NewRateLimiter()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	keyFunc := func(r *http.Request) string { return "test" }
	mw := rl.Middleware(10, time.Minute, keyFunc)
	wrapped := mw(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Header().Get("X-RateLimit-Limit") == "" {
		t.Fatal("missing X-RateLimit-Limit header")
	}
	if w.Header().Get("X-RateLimit-Remaining") == "" {
		t.Fatal("missing X-RateLimit-Remaining header")
	}
	if w.Header().Get("X-RateLimit-Reset") == "" {
		t.Fatal("missing X-RateLimit-Reset header")
	}
}
