package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
)

// --- Mock stores for testing ---

type mockUserStore struct {
	users           map[string]*UserRecord
	failedIncrement bool
}

func newMockUserStore() *mockUserStore {
	return &mockUserStore{users: make(map[string]*UserRecord)}
}

func (m *mockUserStore) GetByUsername(_ context.Context, username string) (*UserRecord, error) {
	u, ok := m.users[username]
	if !ok {
		return nil, &notFoundErr{username}
	}
	return u, nil
}

func (m *mockUserStore) GetByID(_ context.Context, id uuid.UUID) (*UserRecord, error) {
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, &notFoundErr{id.String()}
}

func (m *mockUserStore) GetByEmail(_ context.Context, email string) (*UserRecord, error) {
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, &notFoundErr{email}
}

func (m *mockUserStore) Create(_ context.Context, user *UserRecord) error {
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	m.users[user.Username] = user
	return nil
}

func (m *mockUserStore) IncrementFailedLogins(_ context.Context, id uuid.UUID) error {
	for _, u := range m.users {
		if u.ID == id {
			u.FailedLogins++
			return nil
		}
	}
	return nil
}

func (m *mockUserStore) ResetFailedLogins(_ context.Context, id uuid.UUID) error {
	for _, u := range m.users {
		if u.ID == id {
			u.FailedLogins = 0
			u.LockedUntil = nil
			now := time.Now()
			u.LastLoginAt = &now
			return nil
		}
	}
	return nil
}

func (m *mockUserStore) LockAccount(_ context.Context, id uuid.UUID, until time.Time) error {
	for _, u := range m.users {
		if u.ID == id {
			u.LockedUntil = &until
			return nil
		}
	}
	return nil
}

func (m *mockUserStore) UpdatePassword(_ context.Context, id uuid.UUID, hash string) error {
	for _, u := range m.users {
		if u.ID == id {
			u.PasswordHash = hash
			u.MustChangePass = false
			return nil
		}
	}
	return nil
}

func (m *mockUserStore) UpdateAuthMethod(_ context.Context, id uuid.UUID, method string, clearPassword bool) error {
	for _, u := range m.users {
		if u.ID == id {
			u.AuthMethod = method
			if clearPassword {
				u.PasswordHash = ""
			}
			return nil
		}
	}
	return nil
}

type notFoundErr struct {
	id string
}

func (e *notFoundErr) Error() string {
	return "not found: " + e.id
}

type mockSessionStore struct {
	sessions map[string]*SessionRecord
}

func newMockSessionStore() *mockSessionStore {
	return &mockSessionStore{sessions: make(map[string]*SessionRecord)}
}

func (m *mockSessionStore) Create(_ context.Context, sess *SessionRecord) error {
	m.sessions[sess.ID] = sess
	return nil
}

func (m *mockSessionStore) GetByID(_ context.Context, id string) (*SessionRecord, error) {
	s, ok := m.sessions[id]
	if !ok {
		return nil, &notFoundErr{id}
	}
	return s, nil
}

func (m *mockSessionStore) Delete(_ context.Context, id string) error {
	delete(m.sessions, id)
	return nil
}

func (m *mockSessionStore) DeleteByUserID(_ context.Context, userID uuid.UUID) error {
	for k, s := range m.sessions {
		if s.UserID == userID {
			delete(m.sessions, k)
		}
	}
	return nil
}

type mockAuditStore struct {
	entries []*AuditRecord
}

func (m *mockAuditStore) Insert(_ context.Context, entry *AuditRecord) error {
	m.entries = append(m.entries, entry)
	return nil
}

// --- Helpers ---

func createTestUser(store *mockUserStore, username, password, role, authMethod string) *UserRecord {
	hash, _ := HashPassword(password)
	id := uuid.New()
	user := &UserRecord{
		ID:           id,
		Username:     username,
		Email:        username + "@test.com",
		DisplayName:  username,
		PasswordHash: hash,
		Role:         role,
		AuthMethod:   authMethod,
		IsActive:     true,
	}
	store.users[username] = user
	return user
}

func loginPayload(username, password string) *bytes.Buffer {
	body, _ := json.Marshal(loginRequest{Username: username, Password: password})
	return bytes.NewBuffer(body)
}

func parseEnvelope(t *testing.T, body []byte) map[string]interface{} {
	t.Helper()
	var env map[string]interface{}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	return env
}

// --- Tests ---

func TestHandleLogin(t *testing.T) {
	userStore := newMockUserStore()
	sessionStore := newMockSessionStore()
	auditStore := &mockAuditStore{}
	h := NewHandler(userStore, sessionStore, auditStore)

	createTestUser(userStore, "admin", "SecurePass123!", "admin", "password")

	tests := []struct {
		name       string
		username   string
		password   string
		wantStatus int
		wantSuccess bool
	}{
		{
			name:        "valid login",
			username:    "admin",
			password:    "SecurePass123!",
			wantStatus:  http.StatusOK,
			wantSuccess: true,
		},
		{
			name:        "wrong password",
			username:    "admin",
			password:    "WrongPassword1!",
			wantStatus:  http.StatusUnauthorized,
			wantSuccess: false,
		},
		{
			name:        "unknown user",
			username:    "nobody",
			password:    "SecurePass123!",
			wantStatus:  http.StatusUnauthorized,
			wantSuccess: false,
		},
		{
			name:        "empty username",
			username:    "",
			password:    "SecurePass123!",
			wantStatus:  http.StatusBadRequest,
			wantSuccess: false,
		},
		{
			name:        "empty password",
			username:    "admin",
			password:    "",
			wantStatus:  http.StatusBadRequest,
			wantSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/auth/login", loginPayload(tt.username, tt.password))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			h.HandleLogin(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tt.wantStatus, w.Code, w.Body.String())
			}

			env := parseEnvelope(t, w.Body.Bytes())
			if env["success"] != tt.wantSuccess {
				t.Fatalf("expected success=%v, got %v", tt.wantSuccess, env["success"])
			}
		})
	}
}

func TestHandleLoginSetsCookies(t *testing.T) {
	userStore := newMockUserStore()
	sessionStore := newMockSessionStore()
	h := NewHandler(userStore, sessionStore, &mockAuditStore{})

	createTestUser(userStore, "admin", "SecurePass123!", "admin", "password")

	req := httptest.NewRequest(http.MethodPost, "/auth/login", loginPayload("admin", "SecurePass123!"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleLogin(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	cookies := w.Result().Cookies()
	var sessionCookie, csrfCookie *http.Cookie
	for _, c := range cookies {
		switch c.Name {
		case SessionCookieName:
			sessionCookie = c
		case CSRFCookieName:
			csrfCookie = c
		}
	}

	if sessionCookie == nil {
		t.Fatal("session cookie not set")
	}
	if !sessionCookie.HttpOnly {
		t.Fatal("session cookie should be HttpOnly")
	}
	if sessionCookie.SameSite != http.SameSiteLaxMode {
		t.Fatal("session cookie should be SameSite=Lax")
	}

	if csrfCookie == nil {
		t.Fatal("CSRF cookie not set")
	}
	if csrfCookie.HttpOnly {
		t.Fatal("CSRF cookie should NOT be HttpOnly")
	}
}

func TestHandleLoginLockedAccount(t *testing.T) {
	userStore := newMockUserStore()
	sessionStore := newMockSessionStore()
	h := NewHandler(userStore, sessionStore, &mockAuditStore{})

	user := createTestUser(userStore, "locked", "SecurePass123!", "admin", "password")
	future := time.Now().Add(15 * time.Minute)
	user.LockedUntil = &future

	req := httptest.NewRequest(http.MethodPost, "/auth/login", loginPayload("locked", "SecurePass123!"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleLogin(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for locked account, got %d", w.Code)
	}
}

func TestHandleLoginInactiveUser(t *testing.T) {
	userStore := newMockUserStore()
	sessionStore := newMockSessionStore()
	h := NewHandler(userStore, sessionStore, &mockAuditStore{})

	user := createTestUser(userStore, "inactive", "SecurePass123!", "admin", "password")
	user.IsActive = false

	req := httptest.NewRequest(http.MethodPost, "/auth/login", loginPayload("inactive", "SecurePass123!"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleLogin(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for inactive user, got %d", w.Code)
	}
}

func TestHandleLoginGoogleOnlyUser(t *testing.T) {
	userStore := newMockUserStore()
	sessionStore := newMockSessionStore()
	h := NewHandler(userStore, sessionStore, &mockAuditStore{})

	user := createTestUser(userStore, "googleuser", "SecurePass123!", "editor", "google")
	_ = user

	req := httptest.NewRequest(http.MethodPost, "/auth/login", loginPayload("googleuser", "SecurePass123!"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleLogin(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for google-only user, got %d", w.Code)
	}
}

func TestHandleLoginBruteForceProtection(t *testing.T) {
	userStore := newMockUserStore()
	sessionStore := newMockSessionStore()
	h := NewHandler(userStore, sessionStore, &mockAuditStore{})

	createTestUser(userStore, "bruteforce", "SecurePass123!", "admin", "password")

	// Attempt 5 wrong passwords
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", loginPayload("bruteforce", "WrongPass12345!"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.HandleLogin(w, req)
	}

	// User should now be locked
	user := userStore.users["bruteforce"]
	if user.LockedUntil == nil {
		t.Fatal("expected account to be locked after 5 failed attempts")
	}
	if user.LockedUntil.Before(time.Now()) {
		t.Fatal("lock should be in the future")
	}
}

func TestHandleLogout(t *testing.T) {
	userStore := newMockUserStore()
	sessionStore := newMockSessionStore()
	h := NewHandler(userStore, sessionStore, &mockAuditStore{})

	// Create a session
	sessionStore.sessions["test-session-id"] = &SessionRecord{
		ID:     "test-session-id",
		UserID: uuid.New(),
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "test-session-id"})
	w := httptest.NewRecorder()

	h.HandleLogout(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Session should be deleted
	if _, ok := sessionStore.sessions["test-session-id"]; ok {
		t.Fatal("session should have been deleted")
	}
}

func TestHandleLogoutNoSession(t *testing.T) {
	h := NewHandler(newMockUserStore(), newMockSessionStore(), &mockAuditStore{})

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	w := httptest.NewRecorder()

	h.HandleLogout(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestHandleMe(t *testing.T) {
	userStore := newMockUserStore()
	h := NewHandler(userStore, newMockSessionStore(), &mockAuditStore{})

	user := createTestUser(userStore, "testuser", "SecurePass123!", "editor", "password")

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	ctx := ContextWithUser(req.Context(), user.ID, user.Role, "session")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.HandleMe(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	env := parseEnvelope(t, w.Body.Bytes())
	data, ok := env["data"].(map[string]interface{})
	if !ok {
		t.Fatal("expected data to be an object")
	}
	if data["username"] != "testuser" {
		t.Fatalf("expected username=testuser, got %s", data["username"])
	}
	// password_hash should never appear in JSON
	if _, exists := data["password_hash"]; exists {
		t.Fatal("password_hash should never be in response")
	}
}

func TestHandleMeUnauthenticated(t *testing.T) {
	h := NewHandler(newMockUserStore(), newMockSessionStore(), &mockAuditStore{})

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	w := httptest.NewRecorder()

	h.HandleMe(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestHandleChangePassword(t *testing.T) {
	userStore := newMockUserStore()
	sessionStore := newMockSessionStore()
	h := NewHandler(userStore, sessionStore, &mockAuditStore{})

	user := createTestUser(userStore, "changeme", "OldPassword123!", "admin", "password")

	body, _ := json.Marshal(changePasswordRequest{
		CurrentPassword: "OldPassword123!",
		NewPassword:     "NewPassword456@",
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/change-password", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := ContextWithUser(req.Context(), user.ID, user.Role, "session")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.HandleChangePassword(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify the password was actually changed
	if err := VerifyPassword(userStore.users["changeme"].PasswordHash, "NewPassword456@"); err != nil {
		t.Fatal("new password should verify after change")
	}
}

func TestHandleChangePasswordWrongCurrent(t *testing.T) {
	userStore := newMockUserStore()
	h := NewHandler(userStore, newMockSessionStore(), &mockAuditStore{})

	user := createTestUser(userStore, "wrongpwd", "OldPassword123!", "admin", "password")

	body, _ := json.Marshal(changePasswordRequest{
		CurrentPassword: "WrongPassword1!",
		NewPassword:     "NewPassword456@",
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/change-password", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := ContextWithUser(req.Context(), user.ID, user.Role, "session")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.HandleChangePassword(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestHandleChangePasswordWeakNew(t *testing.T) {
	userStore := newMockUserStore()
	h := NewHandler(userStore, newMockSessionStore(), &mockAuditStore{})

	user := createTestUser(userStore, "weaknew", "OldPassword123!", "admin", "password")

	body, _ := json.Marshal(changePasswordRequest{
		CurrentPassword: "OldPassword123!",
		NewPassword:     "weak",
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/change-password", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := ContextWithUser(req.Context(), user.ID, user.Role, "session")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.HandleChangePassword(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for weak password, got %d", w.Code)
	}
}

func TestLockoutDuration(t *testing.T) {
	tests := []struct {
		failedLogins int
		want         time.Duration
	}{
		{5, 15 * time.Minute},
		{6, 15 * time.Minute},
		{9, 15 * time.Minute},
		{10, 30 * time.Minute},
		{14, 30 * time.Minute},
		{15, 60 * time.Minute},
		{20, 24 * time.Hour},
		{100, 24 * time.Hour},
	}

	for _, tt := range tests {
		got := lockoutDuration(tt.failedLogins)
		if got != tt.want {
			t.Errorf("lockoutDuration(%d) = %v, want %v", tt.failedLogins, got, tt.want)
		}
	}
}

// --- OAuth mock stores ---

type mockOAuthProvider struct {
	enabled     bool
	authURL     string
	state       string
	verifier    string
	claims      *GoogleClaims
	exchangeErr error
}

func (m *mockOAuthProvider) IsEnabled() bool {
	return m.enabled
}

func (m *mockOAuthProvider) GenerateAuthURL() (string, string, string, error) {
	return m.authURL, m.state, m.verifier, nil
}

func (m *mockOAuthProvider) ExchangeCode(_ context.Context, _, _ string) (*GoogleClaims, error) {
	if m.exchangeErr != nil {
		return nil, m.exchangeErr
	}
	return m.claims, nil
}

type mockOAuthConnStore struct {
	conns map[string]*OAuthConnectionRecord // keyed by "provider:provider_uid"
}

func newMockOAuthConnStore() *mockOAuthConnStore {
	return &mockOAuthConnStore{conns: make(map[string]*OAuthConnectionRecord)}
}

func (m *mockOAuthConnStore) GetByProviderUID(_ context.Context, provider, providerUID string) (*OAuthConnectionRecord, error) {
	key := provider + ":" + providerUID
	c, ok := m.conns[key]
	if !ok {
		return nil, &notFoundErr{key}
	}
	return c, nil
}

func (m *mockOAuthConnStore) Create(_ context.Context, conn *OAuthConnectionRecord) error {
	if conn.ID == uuid.Nil {
		conn.ID = uuid.New()
	}
	key := conn.Provider + ":" + conn.ProviderUID
	m.conns[key] = conn
	return nil
}

func (m *mockOAuthConnStore) DeleteByUserID(_ context.Context, userID uuid.UUID) error {
	for key, c := range m.conns {
		if c.UserID == userID {
			delete(m.conns, key)
		}
	}
	return nil
}

func (m *mockOAuthConnStore) GetByUserID(_ context.Context, userID uuid.UUID) ([]OAuthConnectionRecord, error) {
	var result []OAuthConnectionRecord
	for _, c := range m.conns {
		if c.UserID == userID {
			result = append(result, *c)
		}
	}
	return result, nil
}

func newTestEncKey() []byte {
	key := make([]byte, 32)
	rand.Read(key)
	return key
}

// --- OAuth handler tests ---

func TestHandleGoogleStartRedirects(t *testing.T) {
	encKey := newTestEncKey()
	oauth := &mockOAuthProvider{
		enabled:  true,
		authURL:  "https://accounts.google.com/o/oauth2/auth?client_id=test",
		state:    "test-state",
		verifier: "test-verifier",
	}
	h := NewHandlerWithOAuth(newMockUserStore(), newMockSessionStore(), &mockAuditStore{}, oauth, newMockOAuthConnStore(), encKey)

	req := httptest.NewRequest(http.MethodGet, "/auth/google/start", nil)
	w := httptest.NewRecorder()

	h.HandleGoogleStart(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", w.Code, w.Body.String())
	}

	location := w.Header().Get("Location")
	if location != oauth.authURL {
		t.Fatalf("expected redirect to %s, got %s", oauth.authURL, location)
	}

	// Check state cookie was set
	cookies := w.Result().Cookies()
	var stateCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == OAuthStateCookieName {
			stateCookie = c
		}
	}
	if stateCookie == nil {
		t.Fatal("OAuth state cookie should be set")
	}
	if !stateCookie.HttpOnly {
		t.Fatal("state cookie should be HttpOnly")
	}
	if stateCookie.MaxAge != 300 {
		t.Fatalf("state cookie MaxAge should be 300, got %d", stateCookie.MaxAge)
	}
}

func TestHandleGoogleStartDisabled(t *testing.T) {
	oauth := &mockOAuthProvider{enabled: false}
	h := NewHandlerWithOAuth(newMockUserStore(), newMockSessionStore(), &mockAuditStore{}, oauth, newMockOAuthConnStore(), newTestEncKey())

	req := httptest.NewRequest(http.MethodGet, "/auth/google/start", nil)
	w := httptest.NewRecorder()

	h.HandleGoogleStart(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when OAuth disabled, got %d", w.Code)
	}
}

func TestHandleGoogleStartNilOAuth(t *testing.T) {
	h := NewHandler(newMockUserStore(), newMockSessionStore(), &mockAuditStore{})

	req := httptest.NewRequest(http.MethodGet, "/auth/google/start", nil)
	w := httptest.NewRecorder()

	h.HandleGoogleStart(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 with nil OAuth, got %d", w.Code)
	}
}

func TestHandleGoogleCallbackExistingOAuthConnection(t *testing.T) {
	encKey := newTestEncKey()
	userStore := newMockUserStore()
	sessionStore := newMockSessionStore()
	oauthConnStore := newMockOAuthConnStore()

	// Create existing user with OAuth connection
	user := createTestUser(userStore, "oauthuser", "SecurePass123!", "editor", "google")

	oauthConnStore.conns["google:google-uid-123"] = &OAuthConnectionRecord{
		ID:          uuid.New(),
		UserID:      user.ID,
		Provider:    "google",
		ProviderUID: "google-uid-123",
		Email:       "oauthuser@test.com",
	}

	oauth := &mockOAuthProvider{
		enabled: true,
		claims: &GoogleClaims{
			Sub:   "google-uid-123",
			Email: "oauthuser@test.com",
			Name:  "OAuth User",
		},
	}

	h := NewHandlerWithOAuth(userStore, sessionStore, &mockAuditStore{}, oauth, oauthConnStore, encKey)

	// Set up the state cookie
	w := httptest.NewRecorder()
	SetOAuthStateCookie(w, "test-state", "test-verifier", encKey)
	cookies := w.Result().Cookies()

	// Create callback request with state cookie
	callbackURL := "/auth/google/callback?code=authcode123&state=test-state"
	req := httptest.NewRequest(http.MethodGet, callbackURL, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w = httptest.NewRecorder()

	h.HandleGoogleCallback(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect, got %d: %s", w.Code, w.Body.String())
	}

	if w.Header().Get("Location") != "/" {
		t.Fatalf("expected redirect to /, got %s", w.Header().Get("Location"))
	}

	// Session should have been created
	if len(sessionStore.sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessionStore.sessions))
	}

	// Check session cookie was set
	respCookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range respCookies {
		if c.Name == SessionCookieName {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("session cookie should be set after OAuth callback")
	}
}

func TestHandleGoogleCallbackEmailMatchLinksAccount(t *testing.T) {
	encKey := newTestEncKey()
	userStore := newMockUserStore()
	sessionStore := newMockSessionStore()
	oauthConnStore := newMockOAuthConnStore()

	// Create existing user with password auth (same email as Google account)
	user := createTestUser(userStore, "existing", "SecurePass123!", "editor", "password")
	user.Email = "existing@google.com"

	oauth := &mockOAuthProvider{
		enabled: true,
		claims: &GoogleClaims{
			Sub:   "new-google-uid",
			Email: "existing@google.com",
			Name:  "Existing User",
		},
	}

	h := NewHandlerWithOAuth(userStore, sessionStore, &mockAuditStore{}, oauth, oauthConnStore, encKey)

	// Set up the state cookie
	w := httptest.NewRecorder()
	SetOAuthStateCookie(w, "test-state", "test-verifier", encKey)
	cookies := w.Result().Cookies()

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=authcode&state=test-state", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w = httptest.NewRecorder()

	h.HandleGoogleCallback(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", w.Code, w.Body.String())
	}

	// User's auth_method should now be "google"
	if user.AuthMethod != "google" {
		t.Fatalf("expected auth_method=google, got %s", user.AuthMethod)
	}

	// Password should be cleared
	if user.PasswordHash != "" {
		t.Fatal("password_hash should be cleared after Google linking")
	}

	// OAuth connection should exist
	if len(oauthConnStore.conns) != 1 {
		t.Fatalf("expected 1 OAuth connection, got %d", len(oauthConnStore.conns))
	}
}

func TestHandleGoogleCallbackNoMatchCreatesUser(t *testing.T) {
	encKey := newTestEncKey()
	userStore := newMockUserStore()
	sessionStore := newMockSessionStore()
	oauthConnStore := newMockOAuthConnStore()

	oauth := &mockOAuthProvider{
		enabled: true,
		claims: &GoogleClaims{
			Sub:   "brand-new-uid",
			Email: "brandnew@google.com",
			Name:  "Brand New User",
		},
	}

	h := NewHandlerWithOAuth(userStore, sessionStore, &mockAuditStore{}, oauth, oauthConnStore, encKey)

	// Set up the state cookie
	w := httptest.NewRecorder()
	SetOAuthStateCookie(w, "test-state", "test-verifier", encKey)
	cookies := w.Result().Cookies()

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=authcode&state=test-state", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w = httptest.NewRecorder()

	h.HandleGoogleCallback(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", w.Code, w.Body.String())
	}

	// A new user should have been created
	newUser, ok := userStore.users["brandnew@google.com"]
	if !ok {
		t.Fatal("new user should have been created with email as username")
	}
	if newUser.AuthMethod != "google" {
		t.Fatalf("expected auth_method=google, got %s", newUser.AuthMethod)
	}
	if newUser.Role != "viewer" {
		t.Fatalf("expected role=viewer, got %s", newUser.Role)
	}
	if newUser.Email != "brandnew@google.com" {
		t.Fatalf("expected email=brandnew@google.com, got %s", newUser.Email)
	}
	if newUser.DisplayName != "Brand New User" {
		t.Fatalf("expected display_name=Brand New User, got %s", newUser.DisplayName)
	}

	// OAuth connection should exist
	if len(oauthConnStore.conns) != 1 {
		t.Fatalf("expected 1 OAuth connection, got %d", len(oauthConnStore.conns))
	}
}

func TestHandleGoogleCallbackStateMismatch(t *testing.T) {
	encKey := newTestEncKey()
	oauth := &mockOAuthProvider{enabled: true}
	h := NewHandlerWithOAuth(newMockUserStore(), newMockSessionStore(), &mockAuditStore{}, oauth, newMockOAuthConnStore(), encKey)

	// Set state cookie with one state value
	w := httptest.NewRecorder()
	SetOAuthStateCookie(w, "correct-state", "test-verifier", encKey)
	cookies := w.Result().Cookies()

	// But send a different state in the query
	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=authcode&state=wrong-state", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w = httptest.NewRecorder()

	h.HandleGoogleCallback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for state mismatch, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGoogleCallbackMissingCode(t *testing.T) {
	encKey := newTestEncKey()
	oauth := &mockOAuthProvider{enabled: true}
	h := NewHandlerWithOAuth(newMockUserStore(), newMockSessionStore(), &mockAuditStore{}, oauth, newMockOAuthConnStore(), encKey)

	w := httptest.NewRecorder()
	SetOAuthStateCookie(w, "test-state", "test-verifier", encKey)
	cookies := w.Result().Cookies()

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?state=test-state", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w = httptest.NewRecorder()

	h.HandleGoogleCallback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing code, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGoogleCallbackGoogleError(t *testing.T) {
	encKey := newTestEncKey()
	oauth := &mockOAuthProvider{enabled: true}
	h := NewHandlerWithOAuth(newMockUserStore(), newMockSessionStore(), &mockAuditStore{}, oauth, newMockOAuthConnStore(), encKey)

	callbackURL := "/auth/google/callback?" + url.Values{"error": {"access_denied"}}.Encode()
	req := httptest.NewRequest(http.MethodGet, callbackURL, nil)
	w := httptest.NewRecorder()

	h.HandleGoogleCallback(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect on Google error, got %d", w.Code)
	}
	location := w.Header().Get("Location")
	if location != "/?error=oauth_denied" {
		t.Fatalf("expected redirect to /?error=oauth_denied, got %s", location)
	}
}

func TestHandleUnlinkGoogleSuccess(t *testing.T) {
	userStore := newMockUserStore()
	oauthConnStore := newMockOAuthConnStore()

	user := createTestUser(userStore, "bothuser", "SecurePass123!", "editor", "both")
	oauthConnStore.conns["google:uid-123"] = &OAuthConnectionRecord{
		ID:       uuid.New(),
		UserID:   user.ID,
		Provider: "google",
		ProviderUID: "uid-123",
		Email:    "bothuser@test.com",
	}

	h := NewHandlerWithOAuth(userStore, newMockSessionStore(), &mockAuditStore{}, &mockOAuthProvider{enabled: true}, oauthConnStore, newTestEncKey())

	req := httptest.NewRequest(http.MethodPost, "/auth/unlink-google", nil)
	ctx := ContextWithUser(req.Context(), user.ID, user.Role, "session")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.HandleUnlinkGoogle(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Auth method should be password
	if user.AuthMethod != "password" {
		t.Fatalf("expected auth_method=password, got %s", user.AuthMethod)
	}

	// OAuth connections should be deleted
	if len(oauthConnStore.conns) != 0 {
		t.Fatalf("expected 0 OAuth connections, got %d", len(oauthConnStore.conns))
	}
}

func TestHandleUnlinkGoogleNoPassword(t *testing.T) {
	userStore := newMockUserStore()

	// User with Google-only auth (no password)
	user := createTestUser(userStore, "googleonly", "SecurePass123!", "editor", "google")
	user.PasswordHash = "" // No password set

	h := NewHandlerWithOAuth(userStore, newMockSessionStore(), &mockAuditStore{}, &mockOAuthProvider{enabled: true}, newMockOAuthConnStore(), newTestEncKey())

	req := httptest.NewRequest(http.MethodPost, "/auth/unlink-google", nil)
	ctx := ContextWithUser(req.Context(), user.ID, user.Role, "session")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.HandleUnlinkGoogle(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when no password set, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleUnlinkGoogleUnauthenticated(t *testing.T) {
	h := NewHandlerWithOAuth(newMockUserStore(), newMockSessionStore(), &mockAuditStore{}, &mockOAuthProvider{enabled: true}, newMockOAuthConnStore(), newTestEncKey())

	req := httptest.NewRequest(http.MethodPost, "/auth/unlink-google", nil)
	w := httptest.NewRecorder()

	h.HandleUnlinkGoogle(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}
