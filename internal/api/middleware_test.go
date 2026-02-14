package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/auth"
)

// --- Mock implementations ---

type mockSessionLookup struct {
	sessions map[string]sessionInfo
}

type sessionInfo struct {
	userID    uuid.UUID
	role      string
	csrfToken string
}

func (m *mockSessionLookup) GetSessionUser(_ context.Context, sessionID string) (uuid.UUID, string, string, error) {
	s, ok := m.sessions[sessionID]
	if !ok {
		return uuid.Nil, "", "", fmt.Errorf("session not found")
	}
	return s.userID, s.role, s.csrfToken, nil
}

func (m *mockSessionLookup) TouchSession(_ context.Context, _ string) error {
	return nil
}

type mockAPIKeyLookup struct {
	keys map[string]keyInfo
}

type keyInfo struct {
	userID uuid.UUID
	role   string
}

func (m *mockAPIKeyLookup) ValidateAPIKey(_ context.Context, key string) (uuid.UUID, string, error) {
	k, ok := m.keys[key]
	if !ok {
		return uuid.Nil, "", fmt.Errorf("invalid key")
	}
	return k.userID, k.role, nil
}

// --- Tests ---

func TestAuthMiddlewareAPIKey(t *testing.T) {
	userID := uuid.New()
	apiKeys := &mockAPIKeyLookup{
		keys: map[string]keyInfo{
			"areg_abcdef1234567890abcdef1234567890": {userID: userID, role: "admin"},
		},
	}
	sessions := &mockSessionLookup{sessions: make(map[string]sessionInfo)}

	var capturedUserID uuid.UUID
	var capturedAuthType string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserID, _ = auth.UserIDFromContext(r.Context())
		capturedAuthType, _ = auth.AuthTypeFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	mw := AuthMiddleware(sessions, apiKeys)
	wrapped := mw(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer areg_abcdef1234567890abcdef1234567890")
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if capturedUserID != userID {
		t.Fatalf("expected userID %s, got %s", userID, capturedUserID)
	}
	if capturedAuthType != "apikey" {
		t.Fatalf("expected auth type 'apikey', got %s", capturedAuthType)
	}
}

func TestAuthMiddlewareInvalidAPIKey(t *testing.T) {
	apiKeys := &mockAPIKeyLookup{keys: make(map[string]keyInfo)}
	sessions := &mockSessionLookup{sessions: make(map[string]sessionInfo)}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := AuthMiddleware(sessions, apiKeys)
	wrapped := mw(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer areg_invalid00000000000000000000000")
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddlewareSessionCookie(t *testing.T) {
	userID := uuid.New()
	csrfToken := "csrf_token_12345678901234567890123456789012345678901234567890"
	sessions := &mockSessionLookup{
		sessions: map[string]sessionInfo{
			"session123": {userID: userID, role: "editor", csrfToken: csrfToken},
		},
	}
	apiKeys := &mockAPIKeyLookup{keys: make(map[string]keyInfo)}

	var capturedRole string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedRole, _ = auth.UserRoleFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	mw := AuthMiddleware(sessions, apiKeys)
	wrapped := mw(handler)

	// GET request (no CSRF needed)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "session123"})
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if capturedRole != "editor" {
		t.Fatalf("expected role 'editor', got %s", capturedRole)
	}
}

func TestAuthMiddlewareNoAuth(t *testing.T) {
	sessions := &mockSessionLookup{sessions: make(map[string]sessionInfo)}
	apiKeys := &mockAPIKeyLookup{keys: make(map[string]keyInfo)}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := AuthMiddleware(sessions, apiKeys)
	wrapped := mw(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddlewareExpiredSession(t *testing.T) {
	sessions := &mockSessionLookup{sessions: make(map[string]sessionInfo)}
	apiKeys := &mockAPIKeyLookup{keys: make(map[string]keyInfo)}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := AuthMiddleware(sessions, apiKeys)
	wrapped := mw(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "expired-session"})
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for expired session, got %d", w.Code)
	}
}

func TestRequireRole(t *testing.T) {
	tests := []struct {
		name       string
		userRole   string
		required   []string
		wantStatus int
	}{
		{
			name:       "admin allowed for admin role",
			userRole:   "admin",
			required:   []string{"admin"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "editor allowed for editor+admin",
			userRole:   "editor",
			required:   []string{"editor", "admin"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "viewer denied for admin-only",
			userRole:   "viewer",
			required:   []string{"admin"},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "viewer denied for editor+admin",
			userRole:   "viewer",
			required:   []string{"editor", "admin"},
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			mw := RequireRole(tt.required...)
			wrapped := mw(handler)

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			ctx := auth.ContextWithUser(req.Context(), uuid.New(), tt.userRole, "session")
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()

			wrapped.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d", tt.wantStatus, w.Code)
			}
		})
	}
}

func TestRequireRoleNoAuth(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := RequireRole("admin")
	wrapped := mw(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := RequestIDMiddleware(handler)

	t.Run("generates ID when missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()

		wrapped.ServeHTTP(w, req)

		id := w.Header().Get("X-Request-Id")
		if id == "" {
			t.Fatal("expected X-Request-Id to be set")
		}
		// Should be a valid UUID
		if _, err := uuid.Parse(id); err != nil {
			t.Fatalf("expected valid UUID, got %s", id)
		}
	})

	t.Run("preserves existing ID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Request-Id", "custom-id-123")
		w := httptest.NewRecorder()

		wrapped.ServeHTTP(w, req)

		if w.Header().Get("X-Request-Id") != "custom-id-123" {
			t.Fatalf("expected preserved request ID, got %s", w.Header().Get("X-Request-Id"))
		}
	})
}

func TestSecurityHeadersMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := SecurityHeadersMiddleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	expectedHeaders := map[string]string{
		"Strict-Transport-Security": "max-age=63072000; includeSubDomains",
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"Referrer-Policy":           "strict-origin-when-cross-origin",
		"Permissions-Policy":        "camera=(), microphone=(), geolocation=()",
	}

	for header, expected := range expectedHeaders {
		got := w.Header().Get(header)
		if got != expected {
			t.Errorf("%s = %q, want %q", header, got, expected)
		}
	}

	csp := w.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Error("Content-Security-Policy header not set")
	}
}

// --- MustChangePass middleware tests ---
//
// These tests prove that the middleware does NOT enforce must_change_pass.
// A user with must_change_pass=true should be blocked from all API endpoints
// except /auth/change-password, /auth/logout, and /auth/me.

// mockUserLookup provides a mock for looking up a user's must_change_pass status.
type mockUserLookup struct {
	users map[uuid.UUID]bool // userID -> mustChangePass
}

func (m *mockUserLookup) GetMustChangePass(_ context.Context, userID uuid.UUID) (bool, error) {
	val, ok := m.users[userID]
	if !ok {
		return false, fmt.Errorf("user not found")
	}
	return val, nil
}

// buildMustChangePassRouter creates a chi router with AuthMiddleware and
// MustChangePassMiddleware applied, with test routes for API and auth paths.
func buildMustChangePassRouter(sessions SessionLookup, apiKeys APIKeyLookup, users UserLookup) *chi.Mux {
	r := chi.NewRouter()
	r.Use(AuthMiddleware(sessions, apiKeys))
	r.Use(MustChangePassMiddleware(users))

	// API endpoints that should be blocked
	r.Get("/api/v1/agents", func(w http.ResponseWriter, r *http.Request) {
		RespondJSON(w, r, http.StatusOK, map[string]string{"message": "ok"})
	})
	r.Post("/api/v1/agents", func(w http.ResponseWriter, r *http.Request) {
		RespondJSON(w, r, http.StatusOK, map[string]string{"message": "ok"})
	})
	r.Get("/api/v1/mcp-servers", func(w http.ResponseWriter, r *http.Request) {
		RespondJSON(w, r, http.StatusOK, map[string]string{"message": "ok"})
	})

	// Auth endpoints that should be allowed
	r.Post("/auth/change-password", func(w http.ResponseWriter, r *http.Request) {
		RespondJSON(w, r, http.StatusOK, map[string]string{"message": "password changed"})
	})
	r.Post("/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		RespondJSON(w, r, http.StatusOK, map[string]string{"message": "logged out"})
	})
	r.Get("/auth/me", func(w http.ResponseWriter, r *http.Request) {
		RespondJSON(w, r, http.StatusOK, map[string]string{"message": "me"})
	})

	return r
}

func TestMustChangePassBlocksAPIAccess(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	sessions := &mockSessionLookup{
		sessions: map[string]sessionInfo{
			"sess-must-change": {userID: userID, role: "admin", csrfToken: "csrf123"},
		},
	}
	apiKeys := &mockAPIKeyLookup{keys: make(map[string]keyInfo)}
	users := &mockUserLookup{
		users: map[uuid.UUID]bool{
			userID: true, // must_change_pass = true
		},
	}

	router := buildMustChangePassRouter(sessions, apiKeys, users)

	// Test multiple API paths that should all be blocked
	paths := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/agents"},
		{http.MethodGet, "/api/v1/mcp-servers"},
	}

	for _, p := range paths {
		t.Run(p.method+" "+p.path, func(t *testing.T) {
			req := httptest.NewRequest(p.method, p.path, nil)
			req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "sess-must-change"})
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != http.StatusForbidden {
				t.Fatalf("expected 403 for must_change_pass user on %s %s, got %d; body: %s",
					p.method, p.path, w.Code, w.Body.String())
			}

			// Verify the error code is PASSWORD_CHANGE_REQUIRED
			var env Envelope
			if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if env.Success {
				t.Fatal("expected success=false")
			}
			errMap, ok := env.Error.(map[string]interface{})
			if !ok {
				t.Fatal("expected error to be a map")
			}
			if errMap["code"] != "PASSWORD_CHANGE_REQUIRED" {
				t.Fatalf("expected error code PASSWORD_CHANGE_REQUIRED, got %v", errMap["code"])
			}
		})
	}
}

func TestMustChangePassAllowsChangePassword(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	sessions := &mockSessionLookup{
		sessions: map[string]sessionInfo{
			"sess-must-change": {userID: userID, role: "admin", csrfToken: "csrf123"},
		},
	}
	apiKeys := &mockAPIKeyLookup{keys: make(map[string]keyInfo)}
	users := &mockUserLookup{
		users: map[uuid.UUID]bool{
			userID: true, // must_change_pass = true
		},
	}

	router := buildMustChangePassRouter(sessions, apiKeys, users)

	req := httptest.NewRequest(http.MethodPost, "/auth/change-password", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "sess-must-change"})
	// Add CSRF cookie to match stored token
	req.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: "csrf123"})
	req.Header.Set("X-CSRF-Token", "csrf123")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for must_change_pass user on POST /auth/change-password, got %d; body: %s",
			w.Code, w.Body.String())
	}
}

func TestMustChangePassAllowsLogout(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	sessions := &mockSessionLookup{
		sessions: map[string]sessionInfo{
			"sess-must-change": {userID: userID, role: "admin", csrfToken: "csrf123"},
		},
	}
	apiKeys := &mockAPIKeyLookup{keys: make(map[string]keyInfo)}
	users := &mockUserLookup{
		users: map[uuid.UUID]bool{
			userID: true,
		},
	}

	router := buildMustChangePassRouter(sessions, apiKeys, users)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "sess-must-change"})
	req.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: "csrf123"})
	req.Header.Set("X-CSRF-Token", "csrf123")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for must_change_pass user on POST /auth/logout, got %d; body: %s",
			w.Code, w.Body.String())
	}
}

func TestMustChangePassAllowsMe(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	sessions := &mockSessionLookup{
		sessions: map[string]sessionInfo{
			"sess-must-change": {userID: userID, role: "admin", csrfToken: "csrf123"},
		},
	}
	apiKeys := &mockAPIKeyLookup{keys: make(map[string]keyInfo)}
	users := &mockUserLookup{
		users: map[uuid.UUID]bool{
			userID: true,
		},
	}

	router := buildMustChangePassRouter(sessions, apiKeys, users)

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "sess-must-change"})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for must_change_pass user on GET /auth/me, got %d; body: %s",
			w.Code, w.Body.String())
	}
}

func TestMustChangePassAPIKeyBypass(t *testing.T) {
	t.Parallel()

	// An API key user whose associated account has must_change_pass=true
	// should also be blocked from API endpoints. API keys should not bypass
	// the must_change_pass enforcement.
	userID := uuid.New()
	sessions := &mockSessionLookup{sessions: make(map[string]sessionInfo)}
	apiKeys := &mockAPIKeyLookup{
		keys: map[string]keyInfo{
			"areg_mustchangepass1234567890abcdef": {userID: userID, role: "admin"},
		},
	}
	users := &mockUserLookup{
		users: map[uuid.UUID]bool{
			userID: true, // must_change_pass = true
		},
	}

	router := buildMustChangePassRouter(sessions, apiKeys, users)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer areg_mustchangepass1234567890abcdef")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for API key user with must_change_pass=true, got %d; body: %s",
			w.Code, w.Body.String())
	}

	var env Envelope
	if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if env.Success {
		t.Fatal("expected success=false")
	}
	errMap, ok := env.Error.(map[string]interface{})
	if !ok {
		t.Fatal("expected error to be a map")
	}
	if errMap["code"] != "PASSWORD_CHANGE_REQUIRED" {
		t.Fatalf("expected error code PASSWORD_CHANGE_REQUIRED, got %v", errMap["code"])
	}
}

func TestMustChangePassDoesNotBlockNormalUser(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	sessions := &mockSessionLookup{
		sessions: map[string]sessionInfo{
			"sess-normal": {userID: userID, role: "admin", csrfToken: "csrf123"},
		},
	}
	apiKeys := &mockAPIKeyLookup{keys: make(map[string]keyInfo)}
	users := &mockUserLookup{
		users: map[uuid.UUID]bool{
			userID: false, // must_change_pass = false
		},
	}

	router := buildMustChangePassRouter(sessions, apiKeys, users)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "sess-normal"})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for normal user on GET /api/v1/agents, got %d; body: %s",
			w.Code, w.Body.String())
	}
}
