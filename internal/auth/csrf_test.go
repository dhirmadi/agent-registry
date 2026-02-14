package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSetCSRFCookie(t *testing.T) {
	w := httptest.NewRecorder()
	token := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

	SetCSRFCookie(w, token)

	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected CSRF cookie to be set")
	}

	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == CSRFCookieName() {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatalf("cookie %s not found", CSRFCookieName())
	}

	if found.Value != token {
		t.Fatalf("expected cookie value %s, got %s", token, found.Value)
	}
	if found.HttpOnly {
		t.Fatal("CSRF cookie must NOT be HttpOnly (JS needs to read it)")
	}
	if !found.Secure {
		t.Fatal("CSRF cookie must be Secure")
	}
	if found.SameSite != http.SameSiteLaxMode {
		t.Fatalf("expected SameSite=Lax, got %v", found.SameSite)
	}
	if found.Path != "/" {
		t.Fatalf("expected Path=/, got %s", found.Path)
	}
	if found.MaxAge != 28800 {
		t.Fatalf("expected MaxAge=28800, got %d", found.MaxAge)
	}
}

func TestValidateCSRF(t *testing.T) {
	validToken := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

	tests := []struct {
		name        string
		method      string
		cookieValue string
		headerValue string
		wantErr     bool
	}{
		{
			name:        "valid POST with matching token",
			method:      http.MethodPost,
			cookieValue: validToken,
			headerValue: validToken,
			wantErr:     false,
		},
		{
			name:        "valid PUT with matching token",
			method:      http.MethodPut,
			cookieValue: validToken,
			headerValue: validToken,
			wantErr:     false,
		},
		{
			name:        "valid DELETE with matching token",
			method:      http.MethodDelete,
			cookieValue: validToken,
			headerValue: validToken,
			wantErr:     false,
		},
		{
			name:        "valid PATCH with matching token",
			method:      http.MethodPatch,
			cookieValue: validToken,
			headerValue: validToken,
			wantErr:     false,
		},
		{
			name:    "GET is exempt",
			method:  http.MethodGet,
			wantErr: false,
		},
		{
			name:    "HEAD is exempt",
			method:  http.MethodHead,
			wantErr: false,
		},
		{
			name:    "OPTIONS is exempt",
			method:  http.MethodOptions,
			wantErr: false,
		},
		{
			name:        "mismatched tokens",
			method:      http.MethodPost,
			cookieValue: validToken,
			headerValue: "wrong_token_value_1234567890abcdef1234567890abcdef1234567890abcd",
			wantErr:     true,
		},
		{
			name:        "missing header",
			method:      http.MethodPost,
			cookieValue: validToken,
			headerValue: "",
			wantErr:     true,
		},
		{
			name:        "missing cookie",
			method:      http.MethodPost,
			cookieValue: "",
			headerValue: validToken,
			wantErr:     true,
		},
		{
			name:        "both missing",
			method:      http.MethodPost,
			cookieValue: "",
			headerValue: "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/v1/agents", nil)
			if tt.cookieValue != "" {
				req.AddCookie(&http.Cookie{
					Name:  CSRFCookieName(),
					Value: tt.cookieValue,
				})
			}
			if tt.headerValue != "" {
				req.Header.Set(CSRFHeaderName, tt.headerValue)
			}

			err := ValidateCSRF(req)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestClearCSRFCookie(t *testing.T) {
	w := httptest.NewRecorder()

	ClearCSRFCookie(w)

	cookies := w.Result().Cookies()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == CSRFCookieName() {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatalf("cookie %s not found", CSRFCookieName())
	}
	if found.MaxAge != -1 {
		t.Fatalf("expected MaxAge=-1 for clearing, got %d", found.MaxAge)
	}
}
