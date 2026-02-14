package auth

import (
	"errors"
	"fmt"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

// ValidatePasswordPolicy checks that the password meets the required policy:
// minimum 12 characters, at least 1 uppercase, 1 lowercase, 1 digit, 1 special character.
func ValidatePasswordPolicy(password string) error {
	if len(password) < 12 {
		return fmt.Errorf("password must be at least 12 characters long")
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return errors.New("password must contain at least one uppercase letter")
	}
	if !hasLower {
		return errors.New("password must contain at least one lowercase letter")
	}
	if !hasDigit {
		return errors.New("password must contain at least one digit")
	}
	if !hasSpecial {
		return errors.New("password must contain at least one special character")
	}

	return nil
}

// HashPassword hashes a plaintext password using bcrypt with cost 12.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword compares a bcrypt hash with a plaintext password.
// Uses constant-time comparison internally via bcrypt.
func VerifyPassword(hash, password string) error {
	if hash == "" {
		return errors.New("empty password hash")
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
