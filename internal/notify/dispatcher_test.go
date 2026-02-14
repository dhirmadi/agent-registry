package notify

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

// mockLoader implements SubscriptionLoader for testing.
type mockLoader struct {
	subs []Subscription
}

func (m *mockLoader) ListActive(_ context.Context) ([]Subscription, error) {
	return m.subs, nil
}

func TestDispatchDeliversToMatchingSub(t *testing.T) {
	received := make(chan *http.Request, 1)
	bodyReceived := make(chan []byte, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodyReceived <- body
		received <- r
		w.WriteHeader(200)
	}))
	defer srv.Close()

	loader := &mockLoader{
		subs: []Subscription{{
			ID:     uuid.New(),
			URL:    srv.URL,
			Secret: "test-secret",
			Events: []string{"agent.created"},
		}},
	}

	d := NewDispatcher(loader, Config{Workers: 1, MaxRetries: 3, Timeout: 5 * time.Second})
	d.Start()
	defer d.Stop()

	event := Event{
		Type:         "agent.created",
		ResourceType: "agent",
		ResourceID:   "test-id",
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Actor:        "user1",
	}
	d.Dispatch(event)

	select {
	case req := <-received:
		if req.Method != "POST" {
			t.Errorf("expected POST, got %s", req.Method)
		}
		if req.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json content type, got %s", req.Header.Get("Content-Type"))
		}
		if req.Header.Get("X-Webhook-Event") != "agent.created" {
			t.Errorf("expected X-Webhook-Event=agent.created, got %s", req.Header.Get("X-Webhook-Event"))
		}
		body := <-bodyReceived
		var received Event
		if err := json.Unmarshal(body, &received); err != nil {
			t.Fatalf("failed to unmarshal body: %v", err)
		}
		if received.Type != event.Type {
			t.Errorf("expected event type %s, got %s", event.Type, received.Type)
		}
		if received.ResourceID != event.ResourceID {
			t.Errorf("expected resource ID %s, got %s", event.ResourceID, received.ResourceID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for webhook delivery")
	}
}

func TestHMACSignature(t *testing.T) {
	bodyReceived := make(chan []byte, 1)
	sigReceived := make(chan string, 1)

	secret := "my-webhook-secret"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodyReceived <- body
		sigReceived <- r.Header.Get("X-Webhook-Signature")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	loader := &mockLoader{
		subs: []Subscription{{
			ID:     uuid.New(),
			URL:    srv.URL,
			Secret: secret,
			Events: []string{"agent.updated"},
		}},
	}

	d := NewDispatcher(loader, Config{Workers: 1, MaxRetries: 0, Timeout: 5 * time.Second})
	d.Start()
	defer d.Stop()

	d.Dispatch(Event{
		Type:         "agent.updated",
		ResourceType: "agent",
		ResourceID:   "abc",
		Timestamp:    "2025-01-01T00:00:00Z",
		Actor:        "admin",
	})

	select {
	case body := <-bodyReceived:
		sig := <-sigReceived

		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

		if sig != expected {
			t.Errorf("HMAC mismatch:\n  got:  %s\n  want: %s", sig, expected)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for webhook delivery")
	}
}

func TestDispatchSkipsNonMatchingEvents(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	loader := &mockLoader{
		subs: []Subscription{{
			ID:     uuid.New(),
			URL:    srv.URL,
			Secret: "",
			Events: []string{"agent.created"},
		}},
	}

	d := NewDispatcher(loader, Config{Workers: 1, MaxRetries: 0, Timeout: 5 * time.Second})
	d.Start()
	defer d.Stop()

	d.Dispatch(Event{
		Type:         "agent.deleted",
		ResourceType: "agent",
		ResourceID:   "xyz",
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Actor:        "user2",
	})

	// Give the worker time to process
	time.Sleep(500 * time.Millisecond)

	if count := callCount.Load(); count != 0 {
		t.Errorf("expected no HTTP calls for non-matching event, got %d", count)
	}
}

func TestRetryOnFailure(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 2 {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	loader := &mockLoader{
		subs: []Subscription{{
			ID:     uuid.New(),
			URL:    srv.URL,
			Secret: "",
			Events: []string{"agent.created"},
		}},
	}

	d := NewDispatcher(loader, Config{Workers: 1, MaxRetries: 3, Timeout: 5 * time.Second})
	d.Start()
	defer d.Stop()

	d.Dispatch(Event{
		Type:         "agent.created",
		ResourceType: "agent",
		ResourceID:   "retry-test",
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Actor:        "user3",
	})

	// Wait for retries (1s + 2s backoff + margin)
	time.Sleep(5 * time.Second)

	if count := attempts.Load(); count != 3 {
		t.Errorf("expected 3 attempts (1 initial + 2 retries then success on 3rd), got %d", count)
	}
}

func TestDeliveryHeaders(t *testing.T) {
	headers := make(chan http.Header, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers <- r.Header
		w.WriteHeader(200)
	}))
	defer srv.Close()

	loader := &mockLoader{
		subs: []Subscription{{
			ID:     uuid.New(),
			URL:    srv.URL,
			Secret: "s3cret",
			Events: []string{"prompt.created"},
		}},
	}

	d := NewDispatcher(loader, Config{Workers: 1, MaxRetries: 0, Timeout: 5 * time.Second})
	d.Start()
	defer d.Stop()

	d.Dispatch(Event{
		Type:         "prompt.created",
		ResourceType: "prompt",
		ResourceID:   "p1",
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Actor:        "admin",
	})

	select {
	case h := <-headers:
		if h.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", h.Get("Content-Type"))
		}
		if h.Get("X-Webhook-Event") != "prompt.created" {
			t.Errorf("expected X-Webhook-Event prompt.created, got %s", h.Get("X-Webhook-Event"))
		}
		if h.Get("X-Registry-Delivery") == "" {
			t.Error("expected X-Registry-Delivery header to be set")
		}
		if h.Get("X-Webhook-Signature") == "" {
			t.Error("expected X-Webhook-Signature header to be set (secret provided)")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for webhook delivery")
	}
}

func TestDispatchWithNoSubscriptions(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	loader := &mockLoader{
		subs: []Subscription{},
	}

	d := NewDispatcher(loader, Config{Workers: 1, MaxRetries: 0, Timeout: 5 * time.Second})
	d.Start()
	defer d.Stop()

	d.Dispatch(Event{
		Type:         "agent.created",
		ResourceType: "agent",
		ResourceID:   "no-sub-test",
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Actor:        "user4",
	})

	time.Sleep(500 * time.Millisecond)

	if count := callCount.Load(); count != 0 {
		t.Errorf("expected no HTTP calls with no subscriptions, got %d", count)
	}
}

func TestComputeHMAC(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		body   []byte
		want   string
	}{
		{
			name:   "known value",
			secret: "secret-key",
			body:   []byte(`{"event":"agent.created"}`),
			want: func() string {
				mac := hmac.New(sha256.New, []byte("secret-key"))
				mac.Write([]byte(`{"event":"agent.created"}`))
				return hex.EncodeToString(mac.Sum(nil))
			}(),
		},
		{
			name:   "empty body",
			secret: "key",
			body:   []byte{},
			want: func() string {
				mac := hmac.New(sha256.New, []byte("key"))
				mac.Write([]byte{})
				return hex.EncodeToString(mac.Sum(nil))
			}(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := computeHMAC(tc.secret, tc.body)
			if got != tc.want {
				t.Errorf("computeHMAC(%q, %q) = %s, want %s", tc.secret, tc.body, got, tc.want)
			}
		})
	}
}
