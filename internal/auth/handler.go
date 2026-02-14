package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	apierrors "github.com/agent-smit/agentic-registry/internal/errors"
)

// lockoutDurations defines the escalating lockout durations.
// 15min, 30min, 60min, max 24h (doubles each time, capped at 24h).
var lockoutDurations = []time.Duration{
	15 * time.Minute,
	30 * time.Minute,
	60 * time.Minute,
	24 * time.Hour,
}

// UserForAuth is the interface the auth handler needs from the user store.
type UserForAuth interface {
	GetByUsername(ctx context.Context, username string) (*UserRecord, error)
	GetByID(ctx context.Context, id uuid.UUID) (*UserRecord, error)
	GetByEmail(ctx context.Context, email string) (*UserRecord, error)
	Create(ctx context.Context, user *UserRecord) error
	IncrementFailedLogins(ctx context.Context, id uuid.UUID) error
	ResetFailedLogins(ctx context.Context, id uuid.UUID) error
	LockAccount(ctx context.Context, id uuid.UUID, until time.Time) error
	UpdatePassword(ctx context.Context, id uuid.UUID, hash string) error
	UpdateAuthMethod(ctx context.Context, id uuid.UUID, method string, clearPassword bool) error
}

// UserRecord is the user data the auth handler operates on.
type UserRecord struct {
	ID             uuid.UUID
	Username       string
	Email          string
	DisplayName    string
	PasswordHash   string
	Role           string
	AuthMethod     string
	IsActive       bool
	MustChangePass bool
	FailedLogins   int
	LockedUntil    *time.Time
	LastLoginAt    *time.Time
}

// SessionForAuth is the interface the auth handler needs from the session store.
type SessionForAuth interface {
	Create(ctx context.Context, sess *SessionRecord) error
	GetByID(ctx context.Context, id string) (*SessionRecord, error)
	Delete(ctx context.Context, id string) error
	DeleteByUserID(ctx context.Context, userID uuid.UUID) error
	DeleteOthersByUserID(ctx context.Context, userID uuid.UUID, keepSessionID string) error
}

// SessionRecord is the session data used by the auth handler.
type SessionRecord struct {
	ID        string
	UserID    uuid.UUID
	CSRFToken string
	IPAddress string
	UserAgent string
	ExpiresAt time.Time
}

// AuditForAuth is the interface the auth handler needs for audit logging.
type AuditForAuth interface {
	Insert(ctx context.Context, entry *AuditRecord) error
}

// AuditRecord is the audit entry data used by the auth handler.
type AuditRecord struct {
	Actor        string
	ActorID      *uuid.UUID
	Action       string
	ResourceType string
	ResourceID   string
	Details      json.RawMessage
	IPAddress    string
}

// OAuthForAuth is the interface for the OAuth provider.
type OAuthForAuth interface {
	IsEnabled() bool
	GenerateAuthURL() (authURL, state, codeVerifier string, err error)
	ExchangeCode(ctx context.Context, code, codeVerifier string) (*GoogleClaims, error)
}

// OAuthConnectionForAuth is the interface the auth handler needs from the OAuth connection store.
type OAuthConnectionForAuth interface {
	GetByProviderUID(ctx context.Context, provider, providerUID string) (*OAuthConnectionRecord, error)
	Create(ctx context.Context, conn *OAuthConnectionRecord) error
	DeleteByUserID(ctx context.Context, userID uuid.UUID) error
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]OAuthConnectionRecord, error)
}

// OAuthConnectionRecord is the OAuth connection data the auth handler operates on.
type OAuthConnectionRecord struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	Provider    string
	ProviderUID string
	Email       string
	DisplayName string
}

// Handler provides HTTP handlers for authentication endpoints.
type Handler struct {
	users      UserForAuth
	sessions   SessionForAuth
	audit      AuditForAuth
	oauth      OAuthForAuth
	oauthConns OAuthConnectionForAuth
	encKey     []byte
}

// NewHandler creates a new auth Handler.
func NewHandler(users UserForAuth, sessions SessionForAuth, audit AuditForAuth) *Handler {
	return &Handler{
		users:    users,
		sessions: sessions,
		audit:    audit,
	}
}

// NewHandlerWithOAuth creates a new auth Handler with OAuth support.
func NewHandlerWithOAuth(users UserForAuth, sessions SessionForAuth, audit AuditForAuth, oauth OAuthForAuth, oauthConns OAuthConnectionForAuth, encKey []byte) *Handler {
	return &Handler{
		users:      users,
		sessions:   sessions,
		audit:      audit,
		oauth:      oauth,
		oauthConns: oauthConns,
		encKey:     encKey,
	}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	User            userResponse `json:"user"`
	MustChangePwd   bool         `json:"must_change_password"`
}

type userResponse struct {
	ID             uuid.UUID  `json:"id"`
	Username       string     `json:"username"`
	Email          string     `json:"email"`
	DisplayName    string     `json:"display_name"`
	Role           string     `json:"role"`
	AuthMethod     string     `json:"auth_method"`
	IsActive       bool       `json:"is_active"`
	MustChangePass bool       `json:"must_change_password"`
	LastLoginAt    *time.Time `json:"last_login_at"`
}

// HandleLogin handles POST /auth/login.
func (h *Handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	if req.Username == "" || req.Password == "" {
		respondError(w, r, apierrors.Validation("username and password are required"))
		return
	}

	// Look up user
	user, err := h.users.GetByUsername(r.Context(), req.Username)
	if err != nil {
		// User not found — return generic invalid credentials
		respondError(w, r, apierrors.Unauthorized("invalid credentials"))
		return
	}

	// Check if account is locked
	if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
		// Return same message as invalid credentials (no info leakage)
		respondError(w, r, apierrors.Unauthorized("invalid credentials"))
		return
	}

	// Check if user is active
	if !user.IsActive {
		respondError(w, r, apierrors.Unauthorized("invalid credentials"))
		return
	}

	// Check auth method supports password
	if user.AuthMethod != "password" && user.AuthMethod != "both" {
		respondError(w, r, apierrors.Unauthorized("invalid credentials"))
		return
	}

	// Verify password
	if err := VerifyPassword(user.PasswordHash, req.Password); err != nil {
		// Increment failed logins
		h.users.IncrementFailedLogins(r.Context(), user.ID)

		// Check if we should lock
		newFailCount := user.FailedLogins + 1
		if newFailCount >= 5 {
			duration := lockoutDuration(newFailCount)
			h.users.LockAccount(r.Context(), user.ID, time.Now().Add(duration))
		}

		h.auditLog(r, user.Username, &user.ID, "login_failed", "session", "", nil)
		respondError(w, r, apierrors.Unauthorized("invalid credentials"))
		return
	}

	// Success: reset failed logins
	h.users.ResetFailedLogins(r.Context(), user.ID)

	// Create session
	sessionID, err := GenerateSessionID()
	if err != nil {
		respondError(w, r, apierrors.Internal("failed to generate session"))
		return
	}

	csrfToken, err := GenerateCSRFToken()
	if err != nil {
		respondError(w, r, apierrors.Internal("failed to generate CSRF token"))
		return
	}

	sess := &SessionRecord{
		ID:        sessionID,
		UserID:    user.ID,
		CSRFToken: csrfToken,
		IPAddress: clientIP(r),
		UserAgent: r.UserAgent(),
		ExpiresAt: time.Now().Add(SessionTTL),
	}

	if err := h.sessions.Create(r.Context(), sess); err != nil {
		respondError(w, r, apierrors.Internal("failed to create session"))
		return
	}

	// Clear any stale cookies from prior sessions / mode switches, then set fresh ones.
	ClearCSRFCookie(w)
	setSessionCookie(w, sessionID)
	SetCSRFCookie(w, csrfToken)

	h.auditLog(r, user.Username, &user.ID, "login", "session", sessionID, nil)

	respondJSON(w, r, http.StatusOK, loginResponse{
		User: userResponse{
			ID:          user.ID,
			Username:    user.Username,
			Email:       user.Email,
			DisplayName: user.DisplayName,
			Role:        user.Role,
			AuthMethod:  user.AuthMethod,
			IsActive:    user.IsActive,
			LastLoginAt: user.LastLoginAt,
		},
		MustChangePwd: user.MustChangePass,
	})
}

// HandleLogout handles POST /auth/logout.
func (h *Handler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(SessionCookieName())
	if err != nil {
		respondError(w, r, apierrors.Unauthorized("not authenticated"))
		return
	}

	h.sessions.Delete(r.Context(), cookie.Value)

	// Clear cookies
	clearSessionCookie(w)
	ClearCSRFCookie(w)

	h.auditLog(r, "", nil, "logout", "session", cookie.Value, nil)

	respondJSON(w, r, http.StatusOK, map[string]string{"message": "logged out"})
}

// HandleMe handles GET /auth/me.
func (h *Handler) HandleMe(w http.ResponseWriter, r *http.Request) {
	// The authenticated user should be in context (set by middleware)
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		respondError(w, r, apierrors.Unauthorized("not authenticated"))
		return
	}

	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		respondError(w, r, apierrors.Internal("failed to retrieve user"))
		return
	}

	respondJSON(w, r, http.StatusOK, userResponse{
		ID:             user.ID,
		Username:       user.Username,
		Email:          user.Email,
		DisplayName:    user.DisplayName,
		Role:           user.Role,
		AuthMethod:     user.AuthMethod,
		IsActive:       user.IsActive,
		MustChangePass: user.MustChangePass,
		LastLoginAt:    user.LastLoginAt,
	})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// HandleChangePassword handles POST /auth/change-password.
func (h *Handler) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		respondError(w, r, apierrors.Unauthorized("not authenticated"))
		return
	}

	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		respondError(w, r, apierrors.Validation("current_password and new_password are required"))
		return
	}

	// Validate new password policy
	if err := ValidatePasswordPolicy(req.NewPassword); err != nil {
		respondError(w, r, apierrors.Validation(err.Error()))
		return
	}

	// Get user to verify current password
	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		respondError(w, r, apierrors.Internal("failed to retrieve user"))
		return
	}

	// Verify current password
	if err := VerifyPassword(user.PasswordHash, req.CurrentPassword); err != nil {
		respondError(w, r, apierrors.Unauthorized("current password is incorrect"))
		return
	}

	// Hash new password
	hash, err := HashPassword(req.NewPassword)
	if err != nil {
		respondError(w, r, apierrors.Internal("failed to hash password"))
		return
	}

	// Update password
	if err := h.users.UpdatePassword(r.Context(), userID, hash); err != nil {
		respondError(w, r, apierrors.Internal("failed to update password"))
		return
	}

	// Invalidate all OTHER sessions for this user, keeping the current one alive.
	// Without this, the user's active session is destroyed and they get
	// "session expired or invalid" immediately after the password change succeeds.
	currentSessionID := ""
	if cookie, err := r.Cookie(SessionCookieName()); err == nil {
		currentSessionID = cookie.Value
	}
	if currentSessionID != "" {
		h.sessions.DeleteOthersByUserID(r.Context(), userID, currentSessionID)
	} else {
		// Fallback: no session cookie found (e.g., API key auth) — delete all sessions
		h.sessions.DeleteByUserID(r.Context(), userID)
	}

	h.auditLog(r, user.Username, &user.ID, "password_change", "user", user.ID.String(), nil)

	respondJSON(w, r, http.StatusOK, map[string]string{"message": "password changed"})
}

// lockoutDuration returns the lockout duration based on the number of failed attempts.
func lockoutDuration(failedLogins int) time.Duration {
	// How many times we've exceeded the threshold of 5
	idx := (failedLogins - 5) / 5
	if idx < 0 {
		idx = 0
	}
	if idx >= len(lockoutDurations) {
		return lockoutDurations[len(lockoutDurations)-1]
	}
	return lockoutDurations[idx]
}

func setSessionCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName(),
		Value:    sessionID,
		Path:     "/",
		MaxAge:   SessionCookieMaxAge,
		Secure:   secureCookies,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName(),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   secureCookies,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) auditLog(r *http.Request, actor string, actorID *uuid.UUID, action, resourceType, resourceID string, details json.RawMessage) {
	if h.audit == nil {
		return
	}
	h.audit.Insert(r.Context(), &AuditRecord{
		Actor:        actor,
		ActorID:      actorID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      details,
		IPAddress:    clientIP(r),
	})
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Strip port from RemoteAddr
	addr := r.RemoteAddr
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
}

// HandleGoogleStart handles GET /auth/google/start.
// Generates PKCE parameters, sets an encrypted state cookie, and redirects to Google.
func (h *Handler) HandleGoogleStart(w http.ResponseWriter, r *http.Request) {
	if h.oauth == nil || !h.oauth.IsEnabled() {
		respondError(w, r, apierrors.NotFound("oauth", "google"))
		return
	}

	authURL, state, codeVerifier, err := h.oauth.GenerateAuthURL()
	if err != nil {
		respondError(w, r, apierrors.Internal("failed to generate OAuth URL"))
		return
	}

	if err := SetOAuthStateCookie(w, state, codeVerifier, h.encKey); err != nil {
		respondError(w, r, apierrors.Internal("failed to set state cookie"))
		return
	}

	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleGoogleCallback handles GET /auth/google/callback.
// Validates state, exchanges code via PKCE, performs account linking, creates session.
func (h *Handler) HandleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	if h.oauth == nil || !h.oauth.IsEnabled() {
		respondError(w, r, apierrors.NotFound("oauth", "google"))
		return
	}

	// Check for error from Google
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		http.Redirect(w, r, "/?error=oauth_denied", http.StatusFound)
		return
	}

	// Read and validate state cookie
	stateCookie, err := GetOAuthStateCookie(r, h.encKey)
	if err != nil {
		respondError(w, r, apierrors.Validation("invalid or expired OAuth state"))
		return
	}

	// Clear the state cookie immediately
	ClearOAuthStateCookie(w)

	// Validate state matches
	queryState := r.URL.Query().Get("state")
	if queryState == "" || queryState != stateCookie.State {
		respondError(w, r, apierrors.Validation("OAuth state mismatch"))
		return
	}

	// Exchange authorization code for tokens
	code := r.URL.Query().Get("code")
	if code == "" {
		respondError(w, r, apierrors.Validation("missing authorization code"))
		return
	}

	claims, err := h.oauth.ExchangeCode(r.Context(), code, stateCookie.CodeVerifier)
	if err != nil {
		respondError(w, r, apierrors.Internal("failed to exchange authorization code"))
		return
	}

	// Account linking logic (spec Section 3.7)
	user, err := h.linkAccount(r.Context(), claims)
	if err != nil {
		respondError(w, r, apierrors.Internal("failed to link account"))
		return
	}

	// Check if user is active
	if !user.IsActive {
		respondError(w, r, apierrors.Unauthorized("account is disabled"))
		return
	}

	// Reset failed logins on successful OAuth login
	h.users.ResetFailedLogins(r.Context(), user.ID)

	// Create session
	sessionID, err := GenerateSessionID()
	if err != nil {
		respondError(w, r, apierrors.Internal("failed to generate session"))
		return
	}

	csrfToken, err := GenerateCSRFToken()
	if err != nil {
		respondError(w, r, apierrors.Internal("failed to generate CSRF token"))
		return
	}

	sess := &SessionRecord{
		ID:        sessionID,
		UserID:    user.ID,
		CSRFToken: csrfToken,
		IPAddress: clientIP(r),
		UserAgent: r.UserAgent(),
		ExpiresAt: time.Now().Add(SessionTTL),
	}

	if err := h.sessions.Create(r.Context(), sess); err != nil {
		respondError(w, r, apierrors.Internal("failed to create session"))
		return
	}

	// Clear any stale cookies from prior sessions / mode switches, then set fresh ones.
	ClearCSRFCookie(w)
	setSessionCookie(w, sessionID)
	SetCSRFCookie(w, csrfToken)

	h.auditLog(r, user.Username, &user.ID, "oauth_login", "session", sessionID, nil)

	// Redirect to dashboard
	http.Redirect(w, r, "/", http.StatusFound)
}

// linkAccount performs the account linking logic per spec Section 3.7.
func (h *Handler) linkAccount(ctx context.Context, claims *GoogleClaims) (*UserRecord, error) {
	// 1. Check if provider_uid matches an existing oauth_connection
	conn, err := h.oauthConns.GetByProviderUID(ctx, "google", claims.Sub)
	if err == nil {
		// Found existing connection — return that user
		user, err := h.users.GetByID(ctx, conn.UserID)
		if err != nil {
			return nil, err
		}
		return user, nil
	}

	// 2. Check if Google email matches an existing user
	user, err := h.users.GetByEmail(ctx, claims.Email)
	if err == nil {
		// Link Google to existing user
		if err := h.oauthConns.Create(ctx, &OAuthConnectionRecord{
			UserID:      user.ID,
			Provider:    "google",
			ProviderUID: claims.Sub,
			Email:       claims.Email,
			DisplayName: claims.Name,
		}); err != nil {
			return nil, err
		}
		// Set auth_method to "google" and clear password
		if err := h.users.UpdateAuthMethod(ctx, user.ID, "google", true); err != nil {
			return nil, err
		}
		user.AuthMethod = "google"
		user.PasswordHash = ""
		return user, nil
	}

	// 3. No match — create new user
	newUser := &UserRecord{
		ID:          uuid.New(),
		Username:    claims.Email, // Use email as username for OAuth users
		Email:       claims.Email,
		DisplayName: claims.Name,
		AuthMethod:  "google",
		Role:        "viewer",
		IsActive:    true,
	}
	if err := h.users.Create(ctx, newUser); err != nil {
		return nil, err
	}

	// Create OAuth connection
	if err := h.oauthConns.Create(ctx, &OAuthConnectionRecord{
		UserID:      newUser.ID,
		Provider:    "google",
		ProviderUID: claims.Sub,
		Email:       claims.Email,
		DisplayName: claims.Name,
	}); err != nil {
		return nil, err
	}

	return newUser, nil
}

// HandleUnlinkGoogle handles POST /auth/unlink-google.
// Removes the Google OAuth connection for the authenticated user.
// Requires the user to have a password set (auth_method must be "both").
func (h *Handler) HandleUnlinkGoogle(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		respondError(w, r, apierrors.Unauthorized("not authenticated"))
		return
	}

	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		respondError(w, r, apierrors.Internal("failed to retrieve user"))
		return
	}

	// User must have a password to unlink Google (can't leave with no auth method)
	if user.AuthMethod != "both" {
		respondError(w, r, apierrors.Validation("cannot unlink Google: no password set. Set a password first"))
		return
	}

	// Delete OAuth connections
	if err := h.oauthConns.DeleteByUserID(r.Context(), userID); err != nil {
		respondError(w, r, apierrors.Internal("failed to unlink Google account"))
		return
	}

	// Update auth method to password-only
	if err := h.users.UpdateAuthMethod(r.Context(), userID, "password", false); err != nil {
		respondError(w, r, apierrors.Internal("failed to update auth method"))
		return
	}

	h.auditLog(r, user.Username, &user.ID, "google_unlink", "user", user.ID.String(), nil)

	respondJSON(w, r, http.StatusOK, map[string]string{"message": "Google account unlinked"})
}

// Context key types for auth context values.
type contextKey string

const (
	contextKeyUserID   contextKey = "user_id"
	contextKeyUserRole contextKey = "user_role"
	contextKeyAuthType contextKey = "auth_type" // "session" or "apikey"
)

// ContextWithUser adds user information to the context.
func ContextWithUser(ctx context.Context, userID uuid.UUID, role string, authType string) context.Context {
	ctx = context.WithValue(ctx, contextKeyUserID, userID)
	ctx = context.WithValue(ctx, contextKeyUserRole, role)
	ctx = context.WithValue(ctx, contextKeyAuthType, authType)
	return ctx
}

// UserIDFromContext extracts the authenticated user ID from the context.
func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(contextKeyUserID).(uuid.UUID)
	return id, ok
}

// UserRoleFromContext extracts the authenticated user's role from the context.
func UserRoleFromContext(ctx context.Context) (string, bool) {
	role, ok := ctx.Value(contextKeyUserRole).(string)
	return role, ok
}

// AuthTypeFromContext extracts the authentication type from the context.
func AuthTypeFromContext(ctx context.Context) (string, bool) {
	at, ok := ctx.Value(contextKeyAuthType).(string)
	return at, ok
}

