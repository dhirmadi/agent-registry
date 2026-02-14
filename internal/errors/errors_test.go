package errors

import (
	"testing"
)

func TestAPIError_Error(t *testing.T) {
	err := &APIError{Code: "NOT_FOUND", Message: "agent 'foo' not found", Status: 404}
	got := err.Error()
	want := "agent 'foo' not found"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestConstructors(t *testing.T) {
	tests := []struct {
		name       string
		fn         func() *APIError
		wantCode   string
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "NotFound",
			fn:         func() *APIError { return NotFound("agent", "abc") },
			wantCode:   "NOT_FOUND",
			wantStatus: 404,
			wantMsg:    "agent 'abc' not found",
		},
		{
			name:       "Conflict",
			fn:         func() *APIError { return Conflict("resource was modified") },
			wantCode:   "CONFLICT",
			wantStatus: 409,
			wantMsg:    "resource was modified",
		},
		{
			name:       "Validation",
			fn:         func() *APIError { return Validation("name is required") },
			wantCode:   "VALIDATION_ERROR",
			wantStatus: 400,
			wantMsg:    "name is required",
		},
		{
			name:       "Forbidden",
			fn:         func() *APIError { return Forbidden("admin role required") },
			wantCode:   "FORBIDDEN",
			wantStatus: 403,
			wantMsg:    "admin role required",
		},
		{
			name:       "Locked",
			fn:         func() *APIError { return Locked("account temporarily locked") },
			wantCode:   "ACCOUNT_LOCKED",
			wantStatus: 423,
			wantMsg:    "account temporarily locked",
		},
		{
			name:       "Unauthorized",
			fn:         func() *APIError { return Unauthorized("invalid credentials") },
			wantCode:   "UNAUTHORIZED",
			wantStatus: 401,
			wantMsg:    "invalid credentials",
		},
		{
			name:       "Internal",
			fn:         func() *APIError { return Internal("unexpected error") },
			wantCode:   "INTERNAL_ERROR",
			wantStatus: 500,
			wantMsg:    "unexpected error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			if err.Code != tc.wantCode {
				t.Errorf("Code = %q, want %q", err.Code, tc.wantCode)
			}
			if err.Status != tc.wantStatus {
				t.Errorf("Status = %d, want %d", err.Status, tc.wantStatus)
			}
			if err.Message != tc.wantMsg {
				t.Errorf("Message = %q, want %q", err.Message, tc.wantMsg)
			}
		})
	}
}
