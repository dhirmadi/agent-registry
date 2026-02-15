package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// mockAgentCardProvider implements AgentCardProvider for testing.
type mockAgentCardProvider struct {
	card map[string]interface{}
	err  error
}

func (m *mockAgentCardProvider) GetAgentCard(ctx context.Context, agentID string) (map[string]interface{}, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.card, nil
}

func TestA2APublisher_Publish_Upsert(t *testing.T) {
	received := make(chan *http.Request, 1)
	bodyReceived := make(chan []byte, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodyReceived <- body
		received <- r
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	provider := &mockAgentCardProvider{
		card: map[string]interface{}{
			"name":        "Test Agent",
			"description": "A test agent",
			"url":         "https://example.com/api/v1/agents/test_agent",
		},
	}

	pub := NewA2APublisher(srv.URL, "https://registry.example.com", provider)

	err := pub.Publish(context.Background(), "test_agent", "upsert")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case req := <-received:
		if req.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", req.Method)
		}
		if req.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json, got %s", req.Header.Get("Content-Type"))
		}
		if req.URL.Path != "/agents/test_agent" {
			t.Errorf("expected /agents/test_agent, got %s", req.URL.Path)
		}

		body := <-bodyReceived
		var card map[string]interface{}
		if err := json.Unmarshal(body, &card); err != nil {
			t.Fatalf("failed to unmarshal body: %v", err)
		}
		if card["name"] != "Test Agent" {
			t.Errorf("expected name 'Test Agent', got %v", card["name"])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for request")
	}
}

func TestA2APublisher_Publish_Delete(t *testing.T) {
	received := make(chan *http.Request, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	provider := &mockAgentCardProvider{}
	pub := NewA2APublisher(srv.URL, "https://registry.example.com", provider)

	err := pub.Publish(context.Background(), "test_agent", "delete")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case req := <-received:
		if req.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", req.Method)
		}
		if req.URL.Path != "/agents/test_agent" {
			t.Errorf("expected /agents/test_agent, got %s", req.URL.Path)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for request")
	}
}

func TestA2APublisher_RetryOnFailure(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	provider := &mockAgentCardProvider{
		card: map[string]interface{}{"name": "Test Agent"},
	}
	pub := NewA2APublisher(srv.URL, "https://registry.example.com", provider)

	err := pub.Publish(context.Background(), "test_agent", "upsert")
	if err != nil {
		t.Fatalf("unexpected error after retries: %v", err)
	}

	count := attempts.Load()
	if count != 3 {
		t.Errorf("expected 3 attempts (2 failures + 1 success), got %d", count)
	}
}

func TestA2APublisher_ExhaustsRetries(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	provider := &mockAgentCardProvider{
		card: map[string]interface{}{"name": "Test Agent"},
	}
	pub := NewA2APublisher(srv.URL, "https://registry.example.com", provider)

	err := pub.Publish(context.Background(), "test_agent", "upsert")
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}

	count := attempts.Load()
	// 1 initial + 3 retries = 4
	if count != 4 {
		t.Errorf("expected 4 attempts, got %d", count)
	}
}

func TestA2APublisher_InvalidAction(t *testing.T) {
	provider := &mockAgentCardProvider{}
	pub := NewA2APublisher("https://example.com", "https://registry.example.com", provider)

	err := pub.Publish(context.Background(), "test_agent", "invalid")
	if err == nil {
		t.Fatal("expected error for invalid action")
	}
}

func TestA2APublisher_SSRFProtection(t *testing.T) {
	tests := []struct {
		name        string
		registryURL string
	}{
		{"localhost", "http://localhost:8080"},
		{"127.0.0.1", "http://127.0.0.1:8080"},
		{"private 10.x", "http://10.0.0.1:8080"},
		{"private 172.16.x", "http://172.16.0.1:8080"},
		{"private 192.168.x", "http://192.168.1.1:8080"},
		{"link-local 169.254.x", "http://169.254.169.254/latest/meta-data"},
		{"file scheme", "file:///etc/passwd"},
		{"ftp scheme", "ftp://example.com/agents"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateA2ARegistryURL(tt.registryURL)
			if err == nil {
				t.Errorf("expected SSRF rejection for %s", tt.registryURL)
			}
		})
	}
}

func TestA2APublisher_ValidURLs(t *testing.T) {
	tests := []struct {
		name        string
		registryURL string
	}{
		{"https public", "https://registry.example.com"},
		{"http public", "http://registry.example.com"},
		{"with path", "https://registry.example.com/v1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateA2ARegistryURL(tt.registryURL)
			if err != nil {
				t.Errorf("expected valid URL %s, got error: %v", tt.registryURL, err)
			}
		})
	}
}

func TestA2APublisher_CardProviderError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	provider := &mockAgentCardProvider{
		err: context.DeadlineExceeded,
	}
	pub := NewA2APublisher(srv.URL, "https://registry.example.com", provider)

	err := pub.Publish(context.Background(), "test_agent", "upsert")
	if err == nil {
		t.Fatal("expected error when card provider fails")
	}
}

func TestA2APublisher_NilPublisherNoOp(t *testing.T) {
	var pub *A2APublisher
	err := pub.Publish(context.Background(), "test_agent", "upsert")
	if err != nil {
		t.Fatalf("nil publisher should be a no-op, got error: %v", err)
	}
}
