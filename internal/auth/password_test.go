package auth

import (
	"strings"
	"testing"
)

func TestValidatePasswordPolicy(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "valid password",
			password: "SecurePass123!",
			wantErr:  false,
		},
		{
			name:     "valid password with symbols",
			password: "MyP@ssw0rd!!xy",
			wantErr:  false,
		},
		{
			name:     "too short",
			password: "Short1!aA",
			wantErr:  true,
			errMsg:   "at least 12 characters",
		},
		{
			name:     "no uppercase",
			password: "securepass123!",
			wantErr:  true,
			errMsg:   "uppercase",
		},
		{
			name:     "no lowercase",
			password: "SECUREPASS123!",
			wantErr:  true,
			errMsg:   "lowercase",
		},
		{
			name:     "no digit",
			password: "SecurePassword!",
			wantErr:  true,
			errMsg:   "digit",
		},
		{
			name:     "no special char",
			password: "SecurePass1234",
			wantErr:  true,
			errMsg:   "special character",
		},
		{
			name:     "empty password",
			password: "",
			wantErr:  true,
			errMsg:   "at least 12 characters",
		},
		{
			name:     "exactly 12 chars valid",
			password: "Abcdefgh12!@",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePasswordPolicy(tt.password)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errMsg)
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Fatalf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			}
		})
	}
}

func TestHashPassword(t *testing.T) {
	password := "SecurePass123!"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if hash == "" {
		t.Fatal("hash should not be empty")
	}

	if hash == password {
		t.Fatal("hash should not equal plaintext password")
	}

	// bcrypt hashes start with $2a$ or $2b$
	if !strings.HasPrefix(hash, "$2a$") && !strings.HasPrefix(hash, "$2b$") {
		t.Fatalf("hash should be a bcrypt hash, got %q", hash[:10])
	}

	// Verify cost 12 is encoded in the hash
	if !strings.Contains(hash, "$12$") {
		t.Fatalf("hash should use cost 12, got %q", hash)
	}
}

func TestVerifyPassword(t *testing.T) {
	password := "SecurePass123!"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	tests := []struct {
		name     string
		hash     string
		password string
		wantErr  bool
	}{
		{
			name:     "correct password",
			hash:     hash,
			password: password,
			wantErr:  false,
		},
		{
			name:     "wrong password",
			hash:     hash,
			password: "WrongPassword123!",
			wantErr:  true,
		},
		{
			name:     "empty password",
			hash:     hash,
			password: "",
			wantErr:  true,
		},
		{
			name:     "empty hash",
			hash:     "",
			password: password,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyPassword(tt.hash, tt.password)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestHashPasswordDifferentHashes(t *testing.T) {
	password := "SecurePass123!"

	hash1, err := HashPassword(password)
	if err != nil {
		t.Fatalf("first HashPassword failed: %v", err)
	}

	hash2, err := HashPassword(password)
	if err != nil {
		t.Fatalf("second HashPassword failed: %v", err)
	}

	if hash1 == hash2 {
		t.Fatal("two hashes of the same password should be different (different salts)")
	}

	// Both should verify correctly
	if err := VerifyPassword(hash1, password); err != nil {
		t.Fatalf("verify with hash1 failed: %v", err)
	}
	if err := VerifyPassword(hash2, password); err != nil {
		t.Fatalf("verify with hash2 failed: %v", err)
	}
}
