package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/auth"
	"github.com/agent-smit/agentic-registry/internal/store"
)

// --- Mock stores for users tests ---

type mockUserStore struct {
	users       map[uuid.UUID]*store.User
	adminCount  int
	createErr   error
	updateErr   error
	listErr     error
}

func newMockUserStore() *mockUserStore {
	return &mockUserStore{
		users: make(map[uuid.UUID]*store.User),
	}
}

func (m *mockUserStore) List(_ context.Context, offset, limit int) ([]store.User, int, error) {
	if m.listErr != nil {
		return nil, 0, m.listErr
	}
	var all []store.User
	for _, u := range m.users {
		all = append(all, *u)
	}
	total := len(all)
	if offset >= len(all) {
		return nil, total, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], total, nil
}

func (m *mockUserStore) Create(_ context.Context, user *store.User) error {
	if m.createErr != nil {
		return m.createErr
	}
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()
	m.users[user.ID] = user
	return nil
}

func (m *mockUserStore) GetByID(_ context.Context, id uuid.UUID) (*store.User, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, fmt.Errorf("user not found")
	}
	return u, nil
}

func (m *mockUserStore) Update(_ context.Context, user *store.User) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	existing, ok := m.users[user.ID]
	if !ok {
		return fmt.Errorf("user not found")
	}
	if !existing.UpdatedAt.Equal(user.UpdatedAt) {
		return fmt.Errorf("conflict")
	}
	user.UpdatedAt = time.Now()
	m.users[user.ID] = user
	return nil
}

func (m *mockUserStore) CountAdmins(_ context.Context) (int, error) {
	if m.adminCount > 0 {
		return m.adminCount, nil
	}
	count := 0
	for _, u := range m.users {
		if u.Role == "admin" && u.IsActive {
			count++
		}
	}
	return count, nil
}

type mockOAuthConnStore struct {
	deleted map[uuid.UUID]bool
}

func newMockOAuthConnStore() *mockOAuthConnStore {
	return &mockOAuthConnStore{deleted: make(map[uuid.UUID]bool)}
}

func (m *mockOAuthConnStore) DeleteByUserID(_ context.Context, userID uuid.UUID) error {
	m.deleted[userID] = true
	return nil
}

type mockAuditStoreForAPI struct {
	entries []*store.AuditEntry
}

func (m *mockAuditStoreForAPI) Insert(_ context.Context, entry *store.AuditEntry) error {
	m.entries = append(m.entries, entry)
	return nil
}

// --- Helper to create an authenticated request with admin role ---

func adminRequest(method, url string, body interface{}) *http.Request {
	return authedRequest(method, url, body, uuid.New(), "admin")
}

func authedRequest(method, url string, body interface{}, userID uuid.UUID, role string) *http.Request {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, url, &buf)
	req.Header.Set("Content-Type", "application/json")
	ctx := auth.ContextWithUser(req.Context(), userID, role, "session")
	return req.WithContext(ctx)
}

func parseEnvelope(t *testing.T, w *httptest.ResponseRecorder) Envelope {
	t.Helper()
	var env Envelope
	if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	return env
}

// --- Users handler tests ---

func TestUsersHandler_List(t *testing.T) {
	userStore := newMockUserStore()
	audit := &mockAuditStoreForAPI{}
	h := NewUsersHandler(userStore, newMockOAuthConnStore(), audit)

	// Seed some users
	for i := 0; i < 3; i++ {
		u := &store.User{
			ID:          uuid.New(),
			Username:    fmt.Sprintf("user%d", i),
			Email:       fmt.Sprintf("user%d@example.com", i),
			DisplayName: fmt.Sprintf("User %d", i),
			Role:        "viewer",
			AuthMethod:  "password",
			IsActive:    true,
		}
		userStore.users[u.ID] = u
	}

	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantCount  int
	}{
		{
			name:       "list all users default pagination",
			query:      "",
			wantStatus: http.StatusOK,
			wantCount:  3,
		},
		{
			name:       "list with limit=1",
			query:      "?limit=1",
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name:       "list with offset=10 returns empty",
			query:      "?offset=10",
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := adminRequest(http.MethodGet, "/api/v1/users"+tt.query, nil)
			w := httptest.NewRecorder()

			h.List(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, w.Code)
			}

			env := parseEnvelope(t, w)
			if !env.Success {
				t.Fatal("expected success=true")
			}

			data, ok := env.Data.(map[string]interface{})
			if !ok {
				t.Fatal("expected data to be a map")
			}

			users, ok := data["users"].([]interface{})
			if !ok {
				t.Fatal("expected data.users to be an array")
			}
			if len(users) != tt.wantCount {
				t.Fatalf("expected %d users, got %d", tt.wantCount, len(users))
			}
		})
	}
}

func TestUsersHandler_Create(t *testing.T) {
	tests := []struct {
		name       string
		body       map[string]interface{}
		wantStatus int
		wantErr    bool
	}{
		{
			name: "valid user creation",
			body: map[string]interface{}{
				"username":     "newuser",
				"email":        "new@example.com",
				"display_name": "New User",
				"password":     "StrongPass123!",
				"role":         "viewer",
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "weak password rejected",
			body: map[string]interface{}{
				"username":     "weakuser",
				"email":        "weak@example.com",
				"display_name": "Weak User",
				"password":     "short",
				"role":         "viewer",
			},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name: "missing username rejected",
			body: map[string]interface{}{
				"email":        "no-user@example.com",
				"display_name": "No Username",
				"password":     "StrongPass123!",
				"role":         "viewer",
			},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name: "missing email rejected",
			body: map[string]interface{}{
				"username":     "noemail",
				"display_name": "No Email",
				"password":     "StrongPass123!",
				"role":         "viewer",
			},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name: "invalid role rejected",
			body: map[string]interface{}{
				"username":     "badrole",
				"email":        "role@example.com",
				"display_name": "Bad Role",
				"password":     "StrongPass123!",
				"role":         "superadmin",
			},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userStore := newMockUserStore()
			audit := &mockAuditStoreForAPI{}
			h := NewUsersHandler(userStore, newMockOAuthConnStore(), audit)

			req := adminRequest(http.MethodPost, "/api/v1/users", tt.body)
			w := httptest.NewRecorder()

			h.Create(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}

			env := parseEnvelope(t, w)
			if tt.wantErr && env.Success {
				t.Fatal("expected success=false")
			}
			if !tt.wantErr {
				if !env.Success {
					t.Fatal("expected success=true")
				}
				// Verify password_hash is NOT in the response
				data, _ := json.Marshal(env.Data)
				if bytes.Contains(data, []byte("password_hash")) {
					t.Fatal("response must NOT contain password_hash")
				}
			}
		})
	}
}

func TestUsersHandler_Get(t *testing.T) {
	userStore := newMockUserStore()
	audit := &mockAuditStoreForAPI{}
	h := NewUsersHandler(userStore, newMockOAuthConnStore(), audit)

	testUser := &store.User{
		ID:          uuid.New(),
		Username:    "testuser",
		Email:       "test@example.com",
		DisplayName: "Test User",
		Role:        "viewer",
		AuthMethod:  "password",
		IsActive:    true,
	}
	userStore.users[testUser.ID] = testUser

	tests := []struct {
		name       string
		userID     string
		wantStatus int
	}{
		{
			name:       "existing user",
			userID:     testUser.ID.String(),
			wantStatus: http.StatusOK,
		},
		{
			name:       "non-existent user",
			userID:     uuid.New().String(),
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "invalid UUID",
			userID:     "not-a-uuid",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := adminRequest(http.MethodGet, "/api/v1/users/"+tt.userID, nil)
			w := httptest.NewRecorder()

			// Set chi URL param
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("userId", tt.userID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			h.Get(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestUsersHandler_Update(t *testing.T) {
	userStore := newMockUserStore()
	audit := &mockAuditStoreForAPI{}
	h := NewUsersHandler(userStore, newMockOAuthConnStore(), audit)

	testUser := &store.User{
		ID:          uuid.New(),
		Username:    "testuser",
		Email:       "test@example.com",
		DisplayName: "Test User",
		Role:        "viewer",
		AuthMethod:  "password",
		IsActive:    true,
		UpdatedAt:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	userStore.users[testUser.ID] = testUser

	tests := []struct {
		name       string
		userID     string
		ifMatch    string
		body       map[string]interface{}
		wantStatus int
	}{
		{
			name:    "valid update with matching etag",
			userID:  testUser.ID.String(),
			ifMatch: testUser.UpdatedAt.UTC().Format(time.RFC3339Nano),
			body: map[string]interface{}{
				"display_name": "Updated Name",
				"role":         "editor",
				"is_active":    true,
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing If-Match header",
			userID:     testUser.ID.String(),
			ifMatch:    "",
			body:       map[string]interface{}{"display_name": "Updated"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "non-existent user",
			userID:     uuid.New().String(),
			ifMatch:    time.Now().UTC().Format(time.RFC3339Nano),
			body:       map[string]interface{}{"display_name": "Updated"},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := adminRequest(http.MethodPut, "/api/v1/users/"+tt.userID, tt.body)
			if tt.ifMatch != "" {
				req.Header.Set("If-Match", tt.ifMatch)
			}
			w := httptest.NewRecorder()

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("userId", tt.userID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			h.Update(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestUsersHandler_Delete_CannotDeleteSelf(t *testing.T) {
	userStore := newMockUserStore()
	audit := &mockAuditStoreForAPI{}
	h := NewUsersHandler(userStore, newMockOAuthConnStore(), audit)

	adminID := uuid.New()
	adminUser := &store.User{
		ID:          adminID,
		Username:    "admin",
		Email:       "admin@example.com",
		DisplayName: "Admin",
		Role:        "admin",
		AuthMethod:  "password",
		IsActive:    true,
		UpdatedAt:   time.Now(),
	}
	userStore.users[adminID] = adminUser

	// Try to deactivate yourself
	req := authedRequest(http.MethodDelete, "/api/v1/users/"+adminID.String(), nil, adminID, "admin")
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("userId", adminID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	h.Delete(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestUsersHandler_Delete_DeactivatesOther(t *testing.T) {
	userStore := newMockUserStore()
	audit := &mockAuditStoreForAPI{}
	h := NewUsersHandler(userStore, newMockOAuthConnStore(), audit)

	adminID := uuid.New()
	targetID := uuid.New()

	targetUser := &store.User{
		ID:          targetID,
		Username:    "target",
		Email:       "target@example.com",
		DisplayName: "Target User",
		Role:        "viewer",
		AuthMethod:  "password",
		IsActive:    true,
		UpdatedAt:   time.Now(),
	}
	userStore.users[targetID] = targetUser

	req := authedRequest(http.MethodDelete, "/api/v1/users/"+targetID.String(), nil, adminID, "admin")
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("userId", targetID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	h.Delete(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body: %s", w.Code, w.Body.String())
	}

	// Verify user is deactivated
	if userStore.users[targetID].IsActive {
		t.Fatal("expected user to be deactivated")
	}
}

func TestUsersHandler_ResetAuth(t *testing.T) {
	userStore := newMockUserStore()
	oauthConns := newMockOAuthConnStore()
	audit := &mockAuditStoreForAPI{}
	h := NewUsersHandler(userStore, oauthConns, audit)

	adminID := uuid.New()
	targetID := uuid.New()

	targetUser := &store.User{
		ID:          targetID,
		Username:    "oauthuser",
		Email:       "oauth@example.com",
		DisplayName: "OAuth User",
		Role:        "viewer",
		AuthMethod:  "google",
		IsActive:    true,
		UpdatedAt:   time.Now(),
	}
	userStore.users[targetID] = targetUser

	tests := []struct {
		name       string
		targetID   string
		callerID   uuid.UUID
		body       map[string]interface{}
		wantStatus int
	}{
		{
			name:     "valid reset-auth",
			targetID: targetID.String(),
			callerID: adminID,
			body: map[string]interface{}{
				"new_password": "TempPassword123!",
				"force_change": true,
			},
			wantStatus: http.StatusOK,
		},
		{
			name:     "cannot reset yourself",
			targetID: adminID.String(),
			callerID: adminID,
			body: map[string]interface{}{
				"new_password": "TempPassword123!",
				"force_change": true,
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:     "weak password rejected",
			targetID: targetID.String(),
			callerID: adminID,
			body: map[string]interface{}{
				"new_password": "weak",
				"force_change": true,
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Re-add admin user for the "cannot reset yourself" case
			adminUser := &store.User{
				ID:          adminID,
				Username:    "admin",
				Email:       "admin@example.com",
				DisplayName: "Admin",
				Role:        "admin",
				AuthMethod:  "password",
				IsActive:    true,
				UpdatedAt:   time.Now(),
			}
			userStore.users[adminID] = adminUser

			req := authedRequest(http.MethodPost, "/api/v1/users/"+tt.targetID+"/reset-auth", tt.body, tt.callerID, "admin")
			w := httptest.NewRecorder()

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("userId", tt.targetID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			h.ResetAuth(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}

			if tt.wantStatus == http.StatusOK {
				// Verify auth was reset
				updated := userStore.users[targetID]
				if updated.AuthMethod != "password" {
					t.Fatalf("expected auth_method=password, got %s", updated.AuthMethod)
				}
				if !updated.MustChangePass {
					t.Fatal("expected must_change_pass=true")
				}
				// Verify oauth connections were deleted
				if !oauthConns.deleted[targetID] {
					t.Fatal("expected oauth connections to be deleted")
				}
			}
		})
	}
}

func TestUsersHandler_RequiresAdminRole(t *testing.T) {
	// This tests that when mounted behind RequireRole("admin"),
	// non-admin roles get 403. We test this via the middleware directly.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := RequireRole("admin")
	wrapped := mw(handler)

	roles := []struct {
		role       string
		wantStatus int
	}{
		{"admin", http.StatusOK},
		{"editor", http.StatusForbidden},
		{"viewer", http.StatusForbidden},
	}

	for _, tt := range roles {
		t.Run(tt.role+"_access", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
			ctx := auth.ContextWithUser(req.Context(), uuid.New(), tt.role, "session")
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()

			wrapped.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected %d for role %s, got %d", tt.wantStatus, tt.role, w.Code)
			}
		})
	}
}
