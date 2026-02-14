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
		Name:     CSRFCookieName(),
		Value:    token,
		Path:     "/",
		MaxAge:   SessionCookieMaxAge,
		Secure:   secureCookies,
		HttpOnly: false, // JS needs to read this
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearCSRFCookie clears the CSRF cookie by setting MaxAge=-1.
// It clears BOTH the __Host-csrf and csrf cookie names to ensure
// stale cookies from a different mode (HTTPS→HTTP or vice versa) are removed.
func ClearCSRFCookie(w http.ResponseWriter) {
	// Clear the current-mode cookie
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName(),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   secureCookies,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
	// Also clear the opposite-mode cookie name to prevent stale duplicates.
	// Browsers may hold both if the deployment switched between HTTP and HTTPS.
	altName := "csrf"
	if !secureCookies {
		altName = "__Host-csrf"
	}
	http.SetCookie(w, &http.Cookie{
		Name:     altName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
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

	cookie, err := r.Cookie(CSRFCookieName())
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
