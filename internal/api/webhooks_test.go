package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	apierrors "github.com/agent-smit/agentic-registry/internal/errors"
	"github.com/agent-smit/agentic-registry/internal/store"
)

// --- Mock webhook store ---

type mockWebhookStore struct {
	subs      map[uuid.UUID]*store.WebhookSubscription
	createErr error
}

func newMockWebhookStore() *mockWebhookStore {
	return &mockWebhookStore{
		subs: make(map[uuid.UUID]*store.WebhookSubscription),
	}
}

func (m *mockWebhookStore) Create(_ context.Context, sub *store.WebhookSubscription) error {
	if m.createErr != nil {
		return m.createErr
	}
	sub.ID = uuid.New()
	sub.CreatedAt = time.Now()
	sub.UpdatedAt = time.Now()
	m.subs[sub.ID] = sub
	return nil
}

func (m *mockWebhookStore) List(_ context.Context) ([]store.WebhookSubscription, error) {
	var all []store.WebhookSubscription
	for _, s := range m.subs {
		all = append(all, *s)
	}
	return all, nil
}

func (m *mockWebhookStore) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := m.subs[id]; !ok {
		return apierrors.NotFound("webhook_subscription", id.String())
	}
	delete(m.subs, id)
	return nil
}

// --- Webhook handler tests ---

func TestWebhooksHandler_ListEmpty(t *testing.T) {
	whStore := newMockWebhookStore()
	audit := &mockAuditStoreForAPI{}
	h := NewWebhooksHandler(whStore, audit)

	req := adminRequest(http.MethodGet, "/api/v1/webhooks", nil)
	w := httptest.NewRecorder()

	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	env := parseEnvelope(t, w)
	if !env.Success {
		t.Fatal("expected success=true")
	}

	data := env.Data.(map[string]interface{})
	subs := data["subscriptions"].([]interface{})
	if len(subs) != 0 {
		t.Fatalf("expected 0 subscriptions, got %d", len(subs))
	}

	total := data["total"].(float64)
	if total != 0 {
		t.Fatalf("expected total=0, got %v", total)
	}
}

func TestWebhooksHandler_ListWithSubscriptions(t *testing.T) {
	whStore := newMockWebhookStore()
	audit := &mockAuditStoreForAPI{}
	h := NewWebhooksHandler(whStore, audit)

	// Seed subscriptions
	id1 := uuid.New()
	id2 := uuid.New()
	whStore.subs[id1] = &store.WebhookSubscription{
		ID:        id1,
		URL:       "https://example.com/hook1",
		Secret:    "super-secret-1",
		Events:    json.RawMessage(`["agent.created"]`),
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	whStore.subs[id2] = &store.WebhookSubscription{
		ID:        id2,
		URL:       "https://example.com/hook2",
		Secret:    "super-secret-2",
		Events:    json.RawMessage(`["agent.updated","agent.deleted"]`),
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	req := adminRequest(http.MethodGet, "/api/v1/webhooks", nil)
	w := httptest.NewRecorder()

	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	env := parseEnvelope(t, w)
	if !env.Success {
		t.Fatal("expected success=true")
	}

	data := env.Data.(map[string]interface{})
	subs := data["subscriptions"].([]interface{})
	if len(subs) != 2 {
		t.Fatalf("expected 2 subscriptions, got %d", len(subs))
	}

	// Verify secret is NOT in JSON response (due to json:"-" tag)
	rawBody := w.Body.String()
	if contains(rawBody, "super-secret-1") || contains(rawBody, "super-secret-2") {
		t.Fatal("response must NOT contain secret values")
	}
}

func TestWebhooksHandler_CreateValid(t *testing.T) {
	whStore := newMockWebhookStore()
	audit := &mockAuditStoreForAPI{}
	h := NewWebhooksHandler(whStore, audit)

	req := adminRequest(http.MethodPost, "/api/v1/webhooks", map[string]interface{}{
		"url":    "https://example.com/webhook",
		"secret": "my-secret",
		"events": []string{"agent.created", "agent.updated"},
	})
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	env := parseEnvelope(t, w)
	if !env.Success {
		t.Fatal("expected success=true")
	}

	data := env.Data.(map[string]interface{})
	if data["url"] != "https://example.com/webhook" {
		t.Fatalf("expected url in response, got %v", data["url"])
	}

	// Verify secret is NOT in response (json:"-")
	rawBody := w.Body.String()
	if contains(rawBody, "my-secret") {
		t.Fatal("response must NOT contain secret")
	}

	// Verify audit log
	if len(audit.entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(audit.entries))
	}
	if audit.entries[0].Action != "webhook_create" {
		t.Fatalf("expected action webhook_create, got %s", audit.entries[0].Action)
	}
}

func TestWebhooksHandler_CreateMissingURL(t *testing.T) {
	whStore := newMockWebhookStore()
	audit := &mockAuditStoreForAPI{}
	h := NewWebhooksHandler(whStore, audit)

	req := adminRequest(http.MethodPost, "/api/v1/webhooks", map[string]interface{}{
		"secret": "my-secret",
		"events": []string{"agent.created"},
	})
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestWebhooksHandler_CreateMissingEvents(t *testing.T) {
	whStore := newMockWebhookStore()
	audit := &mockAuditStoreForAPI{}
	h := NewWebhooksHandler(whStore, audit)

	req := adminRequest(http.MethodPost, "/api/v1/webhooks", map[string]interface{}{
		"url":    "https://example.com/webhook",
		"secret": "my-secret",
	})
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestWebhooksHandler_CreateEmptyEvents(t *testing.T) {
	whStore := newMockWebhookStore()
	audit := &mockAuditStoreForAPI{}
	h := NewWebhooksHandler(whStore, audit)

	req := adminRequest(http.MethodPost, "/api/v1/webhooks", map[string]interface{}{
		"url":    "https://example.com/webhook",
		"events": []string{},
	})
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty events, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestWebhooksHandler_CreateInvalidBody(t *testing.T) {
	whStore := newMockWebhookStore()
	audit := &mockAuditStoreForAPI{}
	h := NewWebhooksHandler(whStore, audit)

	req := adminRequest(http.MethodPost, "/api/v1/webhooks", nil)
	// Override body with invalid JSON
	req.Body = http.NoBody
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid body, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestWebhooksHandler_DeleteExisting(t *testing.T) {
	whStore := newMockWebhookStore()
	audit := &mockAuditStoreForAPI{}
	h := NewWebhooksHandler(whStore, audit)

	subID := uuid.New()
	whStore.subs[subID] = &store.WebhookSubscription{
		ID:        subID,
		URL:       "https://example.com/hook",
		Secret:    "secret",
		Events:    json.RawMessage(`["agent.created"]`),
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	req := adminRequest(http.MethodDelete, "/api/v1/webhooks/"+subID.String(), nil)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("webhookId", subID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	h.Delete(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body: %s", w.Code, w.Body.String())
	}

	// Verify it was actually deleted
	if _, ok := whStore.subs[subID]; ok {
		t.Fatal("subscription should have been deleted")
	}

	// Verify audit log
	if len(audit.entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(audit.entries))
	}
	if audit.entries[0].Action != "webhook_delete" {
		t.Fatalf("expected action webhook_delete, got %s", audit.entries[0].Action)
	}
}

func TestWebhooksHandler_DeleteNotFound(t *testing.T) {
	whStore := newMockWebhookStore()
	audit := &mockAuditStoreForAPI{}
	h := NewWebhooksHandler(whStore, audit)

	missingID := uuid.New()
	req := adminRequest(http.MethodDelete, "/api/v1/webhooks/"+missingID.String(), nil)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("webhookId", missingID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	h.Delete(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestWebhooksHandler_CreateSSRFBlocked(t *testing.T) {
	whStore := newMockWebhookStore()
	audit := &mockAuditStoreForAPI{}
	h := NewWebhooksHandler(whStore, audit)

	privateURLs := []string{
		"http://localhost/hook",
		"http://127.0.0.1/hook",
		"http://10.0.0.1/hook",
		"http://172.16.0.1/hook",
		"http://192.168.1.1/hook",
		"http://169.254.169.254/latest/meta-data/",
	}

	for _, u := range privateURLs {
		t.Run(u, func(t *testing.T) {
			req := adminRequest(http.MethodPost, "/api/v1/webhooks", map[string]interface{}{
				"url":    u,
				"secret": "s",
				"events": []string{"agent.created"},
			})
			w := httptest.NewRecorder()

			h.Create(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for SSRF URL %s, got %d; body: %s", u, w.Code, w.Body.String())
			}

			env := parseEnvelope(t, w)
			if env.Success {
				t.Fatal("expected success=false")
			}
			errMap, ok := env.Error.(map[string]interface{})
			if !ok {
				t.Fatal("expected error to be a map")
			}
			msg, _ := errMap["message"].(string)
			if !contains(msg, "private") {
				t.Fatalf("expected error about private address, got %q", msg)
			}
		})
	}
}

func TestWebhooksHandler_CreatePublicURLAllowed(t *testing.T) {
	whStore := newMockWebhookStore()
	audit := &mockAuditStoreForAPI{}
	h := NewWebhooksHandler(whStore, audit)

	req := adminRequest(http.MethodPost, "/api/v1/webhooks", map[string]interface{}{
		"url":    "https://hooks.example.com/webhook",
		"secret": "my-secret",
		"events": []string{"agent.created"},
	})
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 for public URL, got %d; body: %s", w.Code, w.Body.String())
	}
}
