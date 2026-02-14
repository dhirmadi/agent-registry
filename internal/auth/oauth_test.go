package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestOAuthProviderIsEnabled(t *testing.T) {
	tests := []struct {
		name    string
		config  OAuthConfig
		want    bool
	}{
		{
			name:   "enabled with all fields",
			config: OAuthConfig{ClientID: "id", ClientSecret: "secret", RedirectURL: "http://localhost/callback"},
			want:   true,
		},
		{
			name:   "disabled with empty client ID",
			config: OAuthConfig{ClientID: "", ClientSecret: "secret", RedirectURL: "http://localhost/callback"},
			want:   false,
		},
		{
			name:   "disabled with zero config",
			config: OAuthConfig{},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewOAuthProvider(tt.config)
			if got := p.IsEnabled(); got != tt.want {
				t.Fatalf("IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOAuthProviderGenerateAuthURL(t *testing.T) {
	provider := NewOAuthProvider(OAuthConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "http://localhost:8080/auth/google/callback",
	})

	authURL, state, codeVerifier, err := provider.GenerateAuthURL()
	if err != nil {
		t.Fatalf("GenerateAuthURL failed: %v", err)
	}

	// State should be non-empty
	if state == "" {
		t.Fatal("state should not be empty")
	}

	// Code verifier should be non-empty and base64url-encoded (no padding)
	if codeVerifier == "" {
		t.Fatal("code_verifier should not be empty")
	}
	if strings.ContainsAny(codeVerifier, "+/=") {
		t.Fatal("code_verifier should use base64url encoding without padding")
	}

	// Parse the auth URL
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("failed to parse auth URL: %v", err)
	}

	// Should be Google's auth endpoint
	if parsed.Host != "accounts.google.com" {
		t.Fatalf("expected accounts.google.com, got %s", parsed.Host)
	}

	params := parsed.Query()

	// Check required OAuth params
	if params.Get("client_id") != "test-client-id" {
		t.Fatalf("expected client_id=test-client-id, got %s", params.Get("client_id"))
	}

	if params.Get("response_type") != "code" {
		t.Fatalf("expected response_type=code, got %s", params.Get("response_type"))
	}

	// Check PKCE params
	if params.Get("code_challenge_method") != "S256" {
		t.Fatalf("expected code_challenge_method=S256, got %s", params.Get("code_challenge_method"))
	}

	codeChallenge := params.Get("code_challenge")
	if codeChallenge == "" {
		t.Fatal("code_challenge should be present")
	}

	// Verify code_challenge = base64url(SHA-256(code_verifier))
	h := sha256.Sum256([]byte(codeVerifier))
	expectedChallenge := base64.RawURLEncoding.EncodeToString(h[:])
	if codeChallenge != expectedChallenge {
		t.Fatalf("code_challenge mismatch: got %s, want %s", codeChallenge, expectedChallenge)
	}

	// Check scopes include openid, email, profile
	scopes := params.Get("scope")
	for _, required := range []string{"openid", "email", "profile"} {
		if !strings.Contains(scopes, required) {
			t.Fatalf("scope should contain %q, got %q", required, scopes)
		}
	}

	// Check state matches
	if params.Get("state") != state {
		t.Fatalf("URL state should match returned state")
	}

	// Check redirect URI
	if params.Get("redirect_uri") != "http://localhost:8080/auth/google/callback" {
		t.Fatalf("unexpected redirect_uri: %s", params.Get("redirect_uri"))
	}
}

func TestOAuthProviderGenerateAuthURLDisabled(t *testing.T) {
	provider := NewOAuthProvider(OAuthConfig{})

	_, _, _, err := provider.GenerateAuthURL()
	if err == nil {
		t.Fatal("GenerateAuthURL should fail when OAuth is disabled")
	}
}

func TestOAuthProviderGenerateAuthURLUniqueness(t *testing.T) {
	provider := NewOAuthProvider(OAuthConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "http://localhost:8080/auth/google/callback",
	})

	_, state1, cv1, _ := provider.GenerateAuthURL()
	_, state2, cv2, _ := provider.GenerateAuthURL()

	if state1 == state2 {
		t.Fatal("consecutive states should be different")
	}
	if cv1 == cv2 {
		t.Fatal("consecutive code verifiers should be different")
	}
}

func TestOAuthStateCookieRoundTrip(t *testing.T) {
	encKey := make([]byte, 32)
	rand.Read(encKey)

	state := "random-state-value"
	codeVerifier := "random-code-verifier"

	// Set cookie
	w := httptest.NewRecorder()
	if err := SetOAuthStateCookie(w, state, codeVerifier, encKey); err != nil {
		t.Fatalf("SetOAuthStateCookie failed: %v", err)
	}

	// Read the cookie from the response
	resp := w.Result()
	cookies := resp.Cookies()

	var oauthCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == OAuthStateCookieName {
			oauthCookie = c
		}
	}
	if oauthCookie == nil {
		t.Fatal("oauth state cookie not set")
	}

	// Verify cookie properties
	if !oauthCookie.HttpOnly {
		t.Fatal("cookie should be HttpOnly")
	}
	if !oauthCookie.Secure {
		t.Fatal("cookie should be Secure")
	}
	if oauthCookie.SameSite != http.SameSiteLaxMode {
		t.Fatal("cookie should be SameSite=Lax")
	}
	if oauthCookie.MaxAge != 300 {
		t.Fatalf("cookie MaxAge should be 300, got %d", oauthCookie.MaxAge)
	}

	// Decrypt cookie
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(oauthCookie)

	got, err := GetOAuthStateCookie(req, encKey)
	if err != nil {
		t.Fatalf("GetOAuthStateCookie failed: %v", err)
	}

	if got.State != state {
		t.Fatalf("state mismatch: got %q, want %q", got.State, state)
	}
	if got.CodeVerifier != codeVerifier {
		t.Fatalf("code_verifier mismatch: got %q, want %q", got.CodeVerifier, codeVerifier)
	}
}

func TestOAuthStateCookieWrongKey(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	rand.Read(key1)
	rand.Read(key2)

	w := httptest.NewRecorder()
	SetOAuthStateCookie(w, "state", "verifier", key1)

	cookies := w.Result().Cookies()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}

	_, err := GetOAuthStateCookie(req, key2)
	if err == nil {
		t.Fatal("GetOAuthStateCookie should fail with wrong key")
	}
}

func TestOAuthStateCookieMissing(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := GetOAuthStateCookie(req, key)
	if err == nil {
		t.Fatal("GetOAuthStateCookie should fail when cookie is missing")
	}
}

func TestClearOAuthStateCookie(t *testing.T) {
	w := httptest.NewRecorder()
	ClearOAuthStateCookie(w)

	cookies := w.Result().Cookies()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == OAuthStateCookieName {
			found = c
		}
	}
	if found == nil {
		t.Fatal("clear cookie should still set the cookie")
	}
	if found.MaxAge != -1 {
		t.Fatalf("clear cookie should have MaxAge=-1, got %d", found.MaxAge)
	}
}

func TestOAuthStateCookieValueIsEncrypted(t *testing.T) {
	encKey := make([]byte, 32)
	rand.Read(encKey)

	w := httptest.NewRecorder()
	SetOAuthStateCookie(w, "my-state", "my-verifier", encKey)

	cookies := w.Result().Cookies()
	for _, c := range cookies {
		if c.Name == OAuthStateCookieName {
			// The cookie value should not contain plaintext state or verifier
			var parsed OAuthStateCookie
			err := json.Unmarshal([]byte(c.Value), &parsed)
			// If it parses as valid JSON with our fields, the encryption isn't working
			if err == nil && (parsed.State == "my-state" || parsed.CodeVerifier == "my-verifier") {
				t.Fatal("cookie value should be encrypted, not plaintext JSON")
			}
		}
	}
}
