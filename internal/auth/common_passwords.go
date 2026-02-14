package auth

import (
	_ "embed"
	"strings"
)

//go:embed common_passwords.txt
var commonPasswordsRaw string

// commonPasswords contains the top common passwords parsed from the embedded text file.
// Passwords are stored lowercased for case-insensitive lookup.
// Source: SecLists / NCSC breached password lists.
var commonPasswords map[string]struct{}

func init() {
	lines := strings.Split(commonPasswordsRaw, "\n")
	commonPasswords = make(map[string]struct{}, len(lines))
	for _, line := range lines {
		pw := strings.TrimSpace(line)
		if pw == "" {
			continue
		}
		commonPasswords[strings.ToLower(pw)] = struct{}{}
	}
}

// isCommonPassword checks whether the given password appears in the
// embedded common-passwords list (case-insensitive).
func isCommonPassword(password string) bool {
	_, exists := commonPasswords[strings.ToLower(password)]
	return exists
}
