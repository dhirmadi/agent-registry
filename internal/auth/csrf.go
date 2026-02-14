package auth

import (
	"crypto/subtle"
	"errors"
	"net/http"
)

const (
	CSRFHeaderName = "X-CSRF-Token"
)

// SetCSRFCookie sets the CSRF double-submit cookie.
// NOT HttpOnly so JavaScript can read it.
func SetCSRFCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   SessionCookieMaxAge,
		Secure:   true,
		HttpOnly: false, // JS needs to read this
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearCSRFCookie clears the CSRF cookie by setting MaxAge=-1.
func ClearCSRFCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   true,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
}

// ValidateCSRF validates the CSRF double-submit cookie pattern.
// GET, HEAD, and OPTIONS requests are exempt.
// The cookie value must match the X-CSRF-Token header using constant-time comparison.
func ValidateCSRF(r *http.Request) error {
	// Safe methods are exempt
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return nil
	}

	cookie, err := r.Cookie(CSRFCookieName)
	if err != nil {
		return errors.New("missing CSRF cookie")
	}

	headerVal := r.Header.Get(CSRFHeaderName)
	if headerVal == "" {
		return errors.New("missing CSRF header")
	}

	if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(headerVal)) != 1 {
		return errors.New("CSRF token mismatch")
	}

	return nil
}
