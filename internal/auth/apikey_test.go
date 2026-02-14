package auth

import (
	"strings"
	"testing"
)

func TestGenerateAPIKey(t *testing.T) {
	plaintext, hash, prefix, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey failed: %v", err)
	}

	// Plaintext should start with areg_
	if !strings.HasPrefix(plaintext, "areg_") {
		t.Fatalf("expected key to start with areg_, got %s", plaintext[:8])
	}

	// After prefix, should be 32 hex chars
	rawKey := strings.TrimPrefix(plaintext, "areg_")
	if len(rawKey) != 32 {
		t.Fatalf("expected 32 hex chars after prefix, got %d", len(rawKey))
	}

	// Validate hex chars
	for _, c := range rawKey {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("invalid hex character in key: %c", c)
		}
	}

	// Hash should be non-empty and 64 hex chars (SHA-256)
	if len(hash) != 64 {
		t.Fatalf("expected 64 hex char hash, got %d", len(hash))
	}

	// Prefix should be first 12 chars of plaintext
	if prefix != plaintext[:12] {
		t.Fatalf("expected prefix %s, got %s", plaintext[:12], prefix)
	}
}

func TestGenerateAPIKeyUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 50; i++ {
		plaintext, _, _, err := GenerateAPIKey()
		if err != nil {
			t.Fatalf("GenerateAPIKey failed on iteration %d: %v", i, err)
		}
		if seen[plaintext] {
			t.Fatalf("duplicate API key generated: %s", plaintext)
		}
		seen[plaintext] = true
	}
}

func TestHashAPIKey(t *testing.T) {
	key := "areg_abcdef1234567890abcdef1234567890"

	hash1 := HashAPIKey(key)
	hash2 := HashAPIKey(key)

	// Same input should produce same hash
	if hash1 != hash2 {
		t.Fatal("SHA-256 hash should be deterministic")
	}

	// Hash should be 64 hex chars
	if len(hash1) != 64 {
		t.Fatalf("expected 64 hex char hash, got %d", len(hash1))
	}

	// Different key should produce different hash
	otherHash := HashAPIKey("areg_00000000000000000000000000000000")
	if hash1 == otherHash {
		t.Fatal("different keys should produce different hashes")
	}
}

func TestHashAPIKeyConsistencyWithGenerate(t *testing.T) {
	plaintext, hash, _, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey failed: %v", err)
	}

	// Hashing the plaintext should produce the same hash
	computed := HashAPIKey(plaintext)
	if computed != hash {
		t.Fatalf("HashAPIKey(%s) = %s, want %s", plaintext, computed, hash)
	}
}

func TestValidateAPIKeyFormat(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		valid bool
	}{
		{
			name:  "valid key",
			key:   "areg_abcdef1234567890abcdef1234567890",
			valid: true,
		},
		{
			name:  "valid key with all digits",
			key:   "areg_12345678901234567890123456789012",
			valid: true,
		},
		{
			name:  "wrong prefix",
			key:   "xreg_abcdef1234567890abcdef1234567890",
			valid: false,
		},
		{
			name:  "no prefix",
			key:   "abcdef1234567890abcdef1234567890abcdef12",
			valid: false,
		},
		{
			name:  "too short",
			key:   "areg_abcdef",
			valid: false,
		},
		{
			name:  "too long",
			key:   "areg_abcdef1234567890abcdef1234567890extra",
			valid: false,
		},
		{
			name:  "invalid hex chars",
			key:   "areg_ABCDEF1234567890abcdef1234567890",
			valid: false,
		},
		{
			name:  "empty",
			key:   "",
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateAPIKeyFormat(tt.key); got != tt.valid {
				t.Fatalf("ValidateAPIKeyFormat(%q) = %v, want %v", tt.key, got, tt.valid)
			}
		})
	}
}
