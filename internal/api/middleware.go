package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/auth"
	apierrors "github.com/agent-smit/agentic-registry/internal/errors"
)

// SessionLookup looks up a session by ID and returns user info.
type SessionLookup interface {
	GetSessionUser(ctx context.Context, sessionID string) (userID uuid.UUID, role string, csrfToken string, err error)
	TouchSession(ctx context.Context, sessionID string) error
}

// APIKeyLookup validates an API key and returns user info.
type APIKeyLookup interface {
	ValidateAPIKey(ctx context.Context, key string) (userID uuid.UUID, role string, err error)
}

// AuthMiddleware returns middleware that authenticates requests via API key or session.
// Priority: 1. API key, 2. Session cookie, 3. 401.
func AuthMiddleware(sessions SessionLookup, apiKeys APIKeyLookup) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1. Check for API key in Authorization header
			if authHeader := r.Header.Get("Authorization"); authHeader != "" {
				if strings.HasPrefix(authHeader, "Bearer areg_") {
					key := strings.TrimPrefix(authHeader, "Bearer ")
					userID, role, err := apiKeys.ValidateAPIKey(r.Context(), key)
					if err != nil {
						RespondError(w, r, apierrors.Unauthorized("invalid API key"))
						return
					}
					ctx := auth.ContextWithUser(r.Context(), userID, role, "apikey")
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// 2. Check for session cookie
			cookie, err := r.Cookie(auth.SessionCookieName())
			if err == nil && cookie.Value != "" {
				userID, role, _, err := sessions.GetSessionUser(r.Context(), cookie.Value)
				if err != nil {
					// Invalid or expired session
					RespondError(w, r, apierrors.Unauthorized("session expired or invalid"))
					return
				}

			// Validate CSRF for mutation requests
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
				if err := auth.ValidateCSRF(r); err != nil {
					RespondError(w, r, apierrors.Forbidden("CSRF validation failed"))
					return
				}
			}

				// Touch session (update last_seen)
				sessions.TouchSession(r.Context(), cookie.Value)

				ctx := auth.ContextWithUser(r.Context(), userID, role, "session")
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// 3. No authentication
			RespondError(w, r, apierrors.Unauthorized("authentication required"))
		})
	}
}

// RequireRole returns middleware that checks the authenticated user has one of the specified roles.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	roleSet := make(map[string]bool, len(roles))
	for _, r := range roles {
		roleSet[r] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, ok := auth.UserRoleFromContext(r.Context())
			if !ok {
				RespondError(w, r, apierrors.Unauthorized("authentication required"))
				return
			}
			if !roleSet[role] {
				RespondError(w, r, apierrors.Forbidden("insufficient permissions"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequestIDMiddleware adds a unique request ID to each request.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			id = uuid.New().String()
			r.Header.Set("X-Request-Id", id)
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r)
	})
}

// UserLookup checks whether a user must change their password.
type UserLookup interface {
	GetMustChangePass(ctx context.Context, userID uuid.UUID) (bool, error)
}

// MustChangePassMiddleware returns middleware that blocks API access for users
// with must_change_pass=true, allowing only /auth/change-password, /auth/logout,
// and /auth/me.
func MustChangePassMiddleware(users UserLookup) func(http.Handler) http.Handler {
	// Paths that must_change_pass users are allowed to access.
	allowedPaths := map[string]bool{
		"/auth/change-password": true,
		"/auth/logout":          true,
		"/auth/me":              true,
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := auth.UserIDFromContext(r.Context())
			if !ok {
				// Not authenticated — let downstream middleware handle it.
				next.ServeHTTP(w, r)
				return
			}

			mustChange, err := users.GetMustChangePass(r.Context(), userID)
			if err != nil {
				// User not found — let downstream handle it.
				next.ServeHTTP(w, r)
				return
			}

			if mustChange && !allowedPaths[r.URL.Path] {
				RespondError(w, r, apierrors.PasswordChangeRequired())
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// MaxBodySize limits the size of request bodies.
func MaxBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CORSMiddleware handles CORS for same-origin requests.
// It rejects cross-origin requests by not setting any Access-Control-Allow-Origin header,
// while handling preflight OPTIONS requests for the embedded GUI.
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// For same-origin requests, Origin is either empty or matches the host
		if origin != "" {
			// Only allow if origin matches the request host (same-origin)
			// The embedded GUI is served from the same origin
			host := r.Host
			if origin == "http://"+host || origin == "https://"+host {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-CSRF-Token, If-Match")
				w.Header().Set("Access-Control-Max-Age", "3600")
				w.Header().Set("Vary", "Origin")
			}
			// Cross-origin: no CORS headers = browser blocks the request
		}

		// Handle preflight
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
