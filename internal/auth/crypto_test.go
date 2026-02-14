package auth

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, 32) // AES-256 requires 32-byte key
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	tests := []struct {
		name      string
		plaintext []byte
	}{
		{"empty", []byte{}},
		{"short string", []byte("hello world")},
		{"json payload", []byte(`{"state":"abc123","code_verifier":"def456"}`)},
		{"binary data", func() []byte {
			b := make([]byte, 256)
			rand.Read(b)
			return b
		}()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ciphertext, err := Encrypt(tt.plaintext, key)
			if err != nil {
				t.Fatalf("Encrypt failed: %v", err)
			}

			// Ciphertext should differ from plaintext
			if len(tt.plaintext) > 0 && bytes.Equal(ciphertext, tt.plaintext) {
				t.Fatal("ciphertext should not equal plaintext")
			}

			// Ciphertext should be longer (nonce + tag overhead)
			if len(ciphertext) <= len(tt.plaintext) {
				t.Fatal("ciphertext should be longer than plaintext due to nonce + tag")
			}

			decrypted, err := Decrypt(ciphertext, key)
			if err != nil {
				t.Fatalf("Decrypt failed: %v", err)
			}

			if !bytes.Equal(decrypted, tt.plaintext) {
				t.Fatalf("decrypted text does not match original: got %q, want %q", decrypted, tt.plaintext)
			}
		})
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	plaintext := []byte("same input twice")

	ct1, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("first Encrypt failed: %v", err)
	}

	ct2, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("second Encrypt failed: %v", err)
	}

	// Each encryption should produce a different ciphertext due to random nonce
	if bytes.Equal(ct1, ct2) {
		t.Fatal("two encryptions of the same plaintext should produce different ciphertexts")
	}
}

func TestDecryptWithWrongKey(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	rand.Read(key1)
	rand.Read(key2)

	plaintext := []byte("secret data")

	ciphertext, err := Encrypt(plaintext, key1)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	_, err = Decrypt(ciphertext, key2)
	if err == nil {
		t.Fatal("Decrypt with wrong key should fail")
	}
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	ciphertext, err := Encrypt([]byte("important data"), key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Flip a byte in the ciphertext
	tampered := make([]byte, len(ciphertext))
	copy(tampered, ciphertext)
	tampered[len(tampered)-1] ^= 0xFF

	_, err = Decrypt(tampered, key)
	if err == nil {
		t.Fatal("Decrypt of tampered ciphertext should fail")
	}
}

func TestEncryptInvalidKeyLength(t *testing.T) {
	tests := []struct {
		name    string
		keyLen  int
	}{
		{"too short 16", 16},
		{"too short 10", 10},
		{"too long 48", 48},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := make([]byte, tt.keyLen)
			rand.Read(key)

			_, err := Encrypt([]byte("test"), key)
			if err == nil {
				t.Fatalf("Encrypt with %d-byte key should fail", tt.keyLen)
			}
		})
	}
}

func TestDecryptTooShortCiphertext(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	// AES-GCM nonce is 12 bytes, so anything shorter should fail
	_, err := Decrypt([]byte("short"), key)
	if err == nil {
		t.Fatal("Decrypt with too-short ciphertext should fail")
	}
}
