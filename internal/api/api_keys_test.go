package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/store"
)

// --- Mock stores for api keys tests ---

type mockAPIKeyStore struct {
	keys      map[uuid.UUID]*store.APIKey
	createErr error
}

func newMockAPIKeyStore() *mockAPIKeyStore {
	return &mockAPIKeyStore{
		keys: make(map[uuid.UUID]*store.APIKey),
	}
}

func (m *mockAPIKeyStore) Create(_ context.Context, key *store.APIKey) error {
	if m.createErr != nil {
		return m.createErr
	}
	if key.ID == uuid.Nil {
		key.ID = uuid.New()
	}
	key.CreatedAt = time.Now()
	m.keys[key.ID] = key
	return nil
}

func (m *mockAPIKeyStore) List(_ context.Context, userID *uuid.UUID) ([]store.APIKey, error) {
	var result []store.APIKey
	for _, k := range m.keys {
		if userID != nil && (k.UserID == nil || *k.UserID != *userID) {
			continue
		}
		result = append(result, *k)
	}
	return result, nil
}

func (m *mockAPIKeyStore) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := m.keys[id]; !ok {
		return fmt.Errorf("api key not found")
	}
	delete(m.keys, id)
	return nil
}

func (m *mockAPIKeyStore) GetByID(_ context.Context, id uuid.UUID) (*store.APIKey, error) {
	k, ok := m.keys[id]
	if !ok {
		return nil, fmt.Errorf("api key not found")
	}
	return k, nil
}

// --- API Keys handler tests ---

func TestAPIKeysHandler_Create(t *testing.T) {
	keyStore := newMockAPIKeyStore()
	audit := &mockAuditStoreForAPI{}
	h := NewAPIKeysHandler(keyStore, audit)

	userID := uuid.New()
	req := authedRequest(http.MethodPost, "/api/v1/api-keys", map[string]interface{}{
		"name":   "My BFF Key",
		"scopes": []string{"read", "write"},
	}, userID, "admin")
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	env := parseEnvelope(t, w)
	if !env.Success {
		t.Fatal("expected success=true")
	}

	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatal("expected data to be a map")
	}

	// Verify the plaintext key is returned
	key, ok := data["key"].(string)
	if !ok || key == "" {
		t.Fatal("expected key to be returned in response")
	}
	if len(key) < 10 {
		t.Fatalf("key seems too short: %s", key)
	}

	// Verify key_prefix is returned
	prefix, ok := data["key_prefix"].(string)
	if !ok || prefix == "" {
		t.Fatal("expected key_prefix to be returned")
	}

	// Verify key_hash is NOT in the response
	rawData, _ := json.Marshal(data)
	if json.Valid(rawData) {
		var dataMap map[string]interface{}
		json.Unmarshal(rawData, &dataMap)
		if _, exists := dataMap["key_hash"]; exists {
			t.Fatal("response must NOT contain key_hash")
		}
	}
}

func TestAPIKeysHandler_Create_MissingName(t *testing.T) {
	keyStore := newMockAPIKeyStore()
	audit := &mockAuditStoreForAPI{}
	h := NewAPIKeysHandler(keyStore, audit)

	userID := uuid.New()
	req := authedRequest(http.MethodPost, "/api/v1/api-keys", map[string]interface{}{
		"scopes": []string{"read"},
	}, userID, "admin")
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestAPIKeysHandler_List_AdminSeesAll(t *testing.T) {
	keyStore := newMockAPIKeyStore()
	audit := &mockAuditStoreForAPI{}
	h := NewAPIKeysHandler(keyStore, audit)

	user1 := uuid.New()
	user2 := uuid.New()

	// Add keys for different users
	keyStore.keys[uuid.New()] = &store.APIKey{
		ID:        uuid.New(),
		UserID:    &user1,
		Name:      "Key 1",
		KeyPrefix: "areg_abc1",
		KeyHash:   "hash1",
		Scopes:    []string{"read"},
		IsActive:  true,
		CreatedAt: time.Now(),
	}
	keyStore.keys[uuid.New()] = &store.APIKey{
		ID:        uuid.New(),
		UserID:    &user2,
		Name:      "Key 2",
		KeyPrefix: "areg_def2",
		KeyHash:   "hash2",
		Scopes:    []string{"read", "write"},
		IsActive:  true,
		CreatedAt: time.Now(),
	}

	// Admin should see all keys
	req := authedRequest(http.MethodGet, "/api/v1/api-keys", nil, uuid.New(), "admin")
	w := httptest.NewRecorder()

	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	env := parseEnvelope(t, w)
	data, _ := env.Data.(map[string]interface{})
	keys, _ := data["keys"].([]interface{})

	if len(keys) != 2 {
		t.Fatalf("admin should see 2 keys, got %d", len(keys))
	}

	// Verify key_hash is not in the response
	rawData, _ := json.Marshal(keys)
	if json.Valid(rawData) {
		rawStr := string(rawData)
		if contains(rawStr, "key_hash") {
			t.Fatal("response must NOT contain key_hash")
		}
	}
}

func TestAPIKeysHandler_List_NonAdminSeesOwnOnly(t *testing.T) {
	keyStore := newMockAPIKeyStore()
	audit := &mockAuditStoreForAPI{}
	h := NewAPIKeysHandler(keyStore, audit)

	myID := uuid.New()
	otherID := uuid.New()

	myKeyID := uuid.New()
	otherKeyID := uuid.New()

	keyStore.keys[myKeyID] = &store.APIKey{
		ID:        myKeyID,
		UserID:    &myID,
		Name:      "My Key",
		KeyPrefix: "areg_mine",
		KeyHash:   "hash1",
		Scopes:    []string{"read"},
		IsActive:  true,
		CreatedAt: time.Now(),
	}
	keyStore.keys[otherKeyID] = &store.APIKey{
		ID:        otherKeyID,
		UserID:    &otherID,
		Name:      "Other Key",
		KeyPrefix: "areg_othr",
		KeyHash:   "hash2",
		Scopes:    []string{"read"},
		IsActive:  true,
		CreatedAt: time.Now(),
	}

	// Non-admin should only see own keys
	req := authedRequest(http.MethodGet, "/api/v1/api-keys", nil, myID, "viewer")
	w := httptest.NewRecorder()

	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	env := parseEnvelope(t, w)
	data, _ := env.Data.(map[string]interface{})
	keys, _ := data["keys"].([]interface{})

	if len(keys) != 1 {
		t.Fatalf("non-admin should see 1 key, got %d", len(keys))
	}
}

func TestAPIKeysHandler_Revoke_OwnKey(t *testing.T) {
	keyStore := newMockAPIKeyStore()
	audit := &mockAuditStoreForAPI{}
	h := NewAPIKeysHandler(keyStore, audit)

	myID := uuid.New()
	keyID := uuid.New()

	keyStore.keys[keyID] = &store.APIKey{
		ID:        keyID,
		UserID:    &myID,
		Name:      "My Key",
		KeyPrefix: "areg_mine",
		KeyHash:   "hash1",
		Scopes:    []string{"read"},
		IsActive:  true,
		CreatedAt: time.Now(),
	}

	req := authedRequest(http.MethodDelete, "/api/v1/api-keys/"+keyID.String(), nil, myID, "viewer")
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("keyId", keyID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	h.Revoke(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestAPIKeysHandler_Revoke_AdminCanRevokeAny(t *testing.T) {
	keyStore := newMockAPIKeyStore()
	audit := &mockAuditStoreForAPI{}
	h := NewAPIKeysHandler(keyStore, audit)

	otherID := uuid.New()
	keyID := uuid.New()

	keyStore.keys[keyID] = &store.APIKey{
		ID:        keyID,
		UserID:    &otherID,
		Name:      "Other Key",
		KeyPrefix: "areg_othr",
		KeyHash:   "hash1",
		Scopes:    []string{"read"},
		IsActive:  true,
		CreatedAt: time.Now(),
	}

	adminID := uuid.New()
	req := authedRequest(http.MethodDelete, "/api/v1/api-keys/"+keyID.String(), nil, adminID, "admin")
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("keyId", keyID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	h.Revoke(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestAPIKeysHandler_Revoke_NonAdminCannotRevokeOthers(t *testing.T) {
	keyStore := newMockAPIKeyStore()
	audit := &mockAuditStoreForAPI{}
	h := NewAPIKeysHandler(keyStore, audit)

	otherID := uuid.New()
	keyID := uuid.New()

	keyStore.keys[keyID] = &store.APIKey{
		ID:        keyID,
		UserID:    &otherID,
		Name:      "Other Key",
		KeyPrefix: "areg_othr",
		KeyHash:   "hash1",
		Scopes:    []string{"read"},
		IsActive:  true,
		CreatedAt: time.Now(),
	}

	myID := uuid.New()
	req := authedRequest(http.MethodDelete, "/api/v1/api-keys/"+keyID.String(), nil, myID, "viewer")
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("keyId", keyID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	h.Revoke(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", w.Code, w.Body.String())
	}
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
