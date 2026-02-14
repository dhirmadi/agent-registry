package auth

import (
	"testing"
)

func TestGenerateSessionID(t *testing.T) {
	id, err := GenerateSessionID()
	if err != nil {
		t.Fatalf("GenerateSessionID failed: %v", err)
	}

	// Session ID should be 64 hex chars (32 bytes hex-encoded)
	if len(id) != 64 {
		t.Fatalf("expected 64 hex chars, got %d", len(id))
	}

	// Should be valid hex
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("invalid hex character: %c", c)
		}
	}
}

func TestGenerateSessionIDUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id, err := GenerateSessionID()
		if err != nil {
			t.Fatalf("GenerateSessionID failed on iteration %d: %v", i, err)
		}
		if seen[id] {
			t.Fatalf("duplicate session ID generated: %s", id)
		}
		seen[id] = true
	}
}

func TestGenerateCSRFToken(t *testing.T) {
	token, err := GenerateCSRFToken()
	if err != nil {
		t.Fatalf("GenerateCSRFToken failed: %v", err)
	}

	// CSRF token should be 64 hex chars (32 bytes hex-encoded)
	if len(token) != 64 {
		t.Fatalf("expected 64 hex chars, got %d", len(token))
	}

	// Should be valid hex
	for _, c := range token {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("invalid hex character: %c", c)
		}
	}
}

func TestGenerateCSRFTokenUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		token, err := GenerateCSRFToken()
		if err != nil {
			t.Fatalf("GenerateCSRFToken failed on iteration %d: %v", i, err)
		}
		if seen[token] {
			t.Fatalf("duplicate CSRF token generated: %s", token)
		}
		seen[token] = true
	}
}

func TestSessionConstants(t *testing.T) {
	if SessionTTL.Hours() != 8 {
		t.Fatalf("expected session TTL of 8 hours, got %v", SessionTTL)
	}
	if SessionIdleTimeout.Minutes() != 30 {
		t.Fatalf("expected idle timeout of 30 minutes, got %v", SessionIdleTimeout)
	}
	if SessionCookieName != "__Host-session" {
		t.Fatalf("expected cookie name __Host-session, got %s", SessionCookieName)
	}
	if CSRFCookieName != "__Host-csrf" {
		t.Fatalf("expected CSRF cookie name __Host-csrf, got %s", CSRFCookieName)
	}
}
