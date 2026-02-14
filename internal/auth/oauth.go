package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// OAuthConfig holds the Google OAuth configuration.
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string // {EXTERNAL_URL}/auth/google/callback
}

// OAuthProvider manages Google OAuth 2.0 with PKCE.
type OAuthProvider struct {
	config  OAuthConfig
	enabled bool
	oauth2  *oauth2.Config
}

// GoogleClaims represents the claims extracted from Google's ID token.
type GoogleClaims struct {
	Sub     string `json:"sub"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

// OAuthStateCookie holds the state and code verifier for the PKCE flow.
type OAuthStateCookie struct {
	State        string `json:"state"`
	CodeVerifier string `json:"code_verifier"`
}

const (
	// oauthStateCookieMaxAge is 5 minutes in seconds.
	oauthStateCookieMaxAge = 300
)

// NewOAuthProvider creates a new OAuthProvider.
// If ClientID is empty, the provider is disabled.
func NewOAuthProvider(cfg OAuthConfig) *OAuthProvider {
	enabled := cfg.ClientID != ""
	var oc *oauth2.Config
	if enabled {
		oc = &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
		}
	}
	return &OAuthProvider{
		config:  cfg,
		enabled: enabled,
		oauth2:  oc,
	}
}

// IsEnabled returns true if Google OAuth is configured.
func (p *OAuthProvider) IsEnabled() bool {
	return p.enabled
}

// GenerateAuthURL creates the Google authorization URL with PKCE.
// Returns: authURL, state, codeVerifier, error.
func (p *OAuthProvider) GenerateAuthURL() (string, string, string, error) {
	if !p.enabled {
		return "", "", "", errors.New("OAuth is not configured")
	}

	// Generate state: 32 random bytes, base64url-encoded
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", "", "", fmt.Errorf("generating state: %w", err)
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)

	// Generate PKCE code_verifier: 32 random bytes, base64url-encoded (no padding)
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return "", "", "", fmt.Errorf("generating code verifier: %w", err)
	}
	codeVerifier := base64.RawURLEncoding.EncodeToString(verifierBytes)

	// Generate code_challenge: SHA-256(code_verifier), base64url-encoded (no padding)
	h := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(h[:])

	authURL := p.oauth2.AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge", codeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)

	return authURL, state, codeVerifier, nil
}

// ExchangeCode exchanges the authorization code for tokens using PKCE.
// Returns the Google user claims (sub, email, name, picture).
func (p *OAuthProvider) ExchangeCode(ctx context.Context, code, codeVerifier string) (*GoogleClaims, error) {
	if !p.enabled {
		return nil, errors.New("OAuth is not configured")
	}

	token, err := p.oauth2.Exchange(ctx, code,
		oauth2.SetAuthURLParam("code_verifier", codeVerifier),
	)
	if err != nil {
		return nil, fmt.Errorf("exchanging code: %w", err)
	}

	// Extract ID token from the extra fields
	idTokenRaw, ok := token.Extra("id_token").(string)
	if !ok || idTokenRaw == "" {
		return nil, errors.New("no id_token in token response")
	}

	// Parse the ID token (JWT) — we only need the claims payload
	claims, err := parseIDToken(idTokenRaw)
	if err != nil {
		return nil, fmt.Errorf("parsing id_token: %w", err)
	}

	return claims, nil
}

// parseIDToken extracts claims from a JWT ID token without full signature verification.
// Google's token endpoint is trusted (TLS), and we validate via the token exchange itself.
func parseIDToken(idToken string) (*GoogleClaims, error) {
	parts := splitJWT(idToken)
	if len(parts) != 3 {
		return nil, errors.New("invalid JWT format")
	}

	// Decode the payload (second part)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decoding JWT payload: %w", err)
	}

	var claims GoogleClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("unmarshalling claims: %w", err)
	}

	if claims.Sub == "" {
		return nil, errors.New("missing sub claim")
	}
	if claims.Email == "" {
		return nil, errors.New("missing email claim")
	}

	return &claims, nil
}

// splitJWT splits a JWT into its three parts.
func splitJWT(token string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			parts = append(parts, token[start:i])
			start = i + 1
		}
	}
	parts = append(parts, token[start:])
	return parts
}

// SetOAuthStateCookie encrypts the state and code verifier and sets an HTTP cookie.
func SetOAuthStateCookie(w http.ResponseWriter, state, codeVerifier string, encKey []byte) error {
	data := OAuthStateCookie{
		State:        state,
		CodeVerifier: codeVerifier,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshalling state cookie: %w", err)
	}

	encrypted, err := Encrypt(jsonData, encKey)
	if err != nil {
		return fmt.Errorf("encrypting state cookie: %w", err)
	}

	// Use base64url encoding for the cookie value (binary data)
	cookieValue := base64.RawURLEncoding.EncodeToString(encrypted)

	http.SetCookie(w, &http.Cookie{
		Name:     OAuthStateCookieName(),
		Value:    cookieValue,
		Path:     "/",
		MaxAge:   oauthStateCookieMaxAge,
		Secure:   secureCookies,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	return nil
}

// GetOAuthStateCookie reads and decrypts the OAuth state cookie from the request.
func GetOAuthStateCookie(r *http.Request, encKey []byte) (*OAuthStateCookie, error) {
	cookie, err := r.Cookie(OAuthStateCookieName())
	if err != nil {
		return nil, fmt.Errorf("reading oauth state cookie: %w", err)
	}

	encrypted, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return nil, fmt.Errorf("decoding cookie value: %w", err)
	}

	decrypted, err := Decrypt(encrypted, encKey)
	if err != nil {
		return nil, fmt.Errorf("decrypting state cookie: %w", err)
	}

	var data OAuthStateCookie
	if err := json.Unmarshal(decrypted, &data); err != nil {
		return nil, fmt.Errorf("unmarshalling state cookie: %w", err)
	}

	return &data, nil
}

// ClearOAuthStateCookie clears the OAuth state cookie.
func ClearOAuthStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     OAuthStateCookieName(),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   secureCookies,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
