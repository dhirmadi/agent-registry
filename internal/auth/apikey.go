package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
)

const (
	apiKeyPrefix    = "areg_"
	apiKeyPrefixLen = 12 // first 12 chars for display
	apiKeyRawLen    = 16 // 16 random bytes = 32 hex chars
)

// apiKeyPattern matches areg_ followed by exactly 32 lowercase hex chars.
var apiKeyPattern = regexp.MustCompile(`^areg_[0-9a-f]{32}$`)

// GenerateAPIKey generates a new API key.
// Returns the plaintext key, its SHA-256 hash, and a prefix for display.
func GenerateAPIKey() (plaintext string, hash string, prefix string, err error) {
	b := make([]byte, apiKeyRawLen)
	if _, err := rand.Read(b); err != nil {
		return "", "", "", fmt.Errorf("generating API key: %w", err)
	}

	plaintext = apiKeyPrefix + hex.EncodeToString(b)
	hash = HashAPIKey(plaintext)
	prefix = plaintext[:apiKeyPrefixLen]

	return plaintext, hash, prefix, nil
}

// HashAPIKey computes the SHA-256 hex digest of an API key.
func HashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

// ValidateAPIKeyFormat checks that a key matches the expected format:
// areg_ followed by exactly 32 lowercase hex characters.
func ValidateAPIKeyFormat(key string) bool {
	return apiKeyPattern.MatchString(key)
}
