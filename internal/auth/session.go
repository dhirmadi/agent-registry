package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	SessionTTL         = 8 * time.Hour
	SessionIdleTimeout = 30 * time.Minute
	SessionCookieName  = "__Host-session"
	CSRFCookieName     = "__Host-csrf"
	SessionCookieMaxAge = 28800 // 8 hours in seconds
)

// Session represents a server-side session stored in PostgreSQL.
type Session struct {
	ID        string    `json:"id" db:"id"`
	UserID    uuid.UUID `json:"user_id" db:"user_id"`
	CSRFToken string    `json:"-" db:"csrf_token"`
	IPAddress string    `json:"ip_address" db:"ip_address"`
	UserAgent string    `json:"user_agent" db:"user_agent"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	LastSeen  time.Time `json:"last_seen" db:"last_seen"`
	ExpiresAt time.Time `json:"expires_at" db:"expires_at"`
}

// GenerateSessionID generates a cryptographically random session ID.
// Returns 32 random bytes, hex-encoded (64 hex characters).
func GenerateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating session ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// GenerateCSRFToken generates a cryptographically random CSRF token.
// Returns 32 random bytes, hex-encoded (64 hex characters).
func GenerateCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating CSRF token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
