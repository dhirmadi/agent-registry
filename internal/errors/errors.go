package errors

import "fmt"

// APIError represents a structured API error with an HTTP status code.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
}

func (e *APIError) Error() string {
	return e.Message
}

func NotFound(resource, id string) *APIError {
	return &APIError{
		Code:    "NOT_FOUND",
		Message: fmt.Sprintf("%s '%s' not found", resource, id),
		Status:  404,
	}
}

func Conflict(msg string) *APIError {
	return &APIError{
		Code:    "CONFLICT",
		Message: msg,
		Status:  409,
	}
}

func Validation(msg string) *APIError {
	return &APIError{
		Code:    "VALIDATION_ERROR",
		Message: msg,
		Status:  400,
	}
}

func Forbidden(msg string) *APIError {
	return &APIError{
		Code:    "FORBIDDEN",
		Message: msg,
		Status:  403,
	}
}

func Locked(msg string) *APIError {
	return &APIError{
		Code:    "ACCOUNT_LOCKED",
		Message: msg,
		Status:  423,
	}
}

func Unauthorized(msg string) *APIError {
	return &APIError{
		Code:    "UNAUTHORIZED",
		Message: msg,
		Status:  401,
	}
}

func Internal(msg string) *APIError {
	return &APIError{
		Code:    "INTERNAL_ERROR",
		Message: msg,
		Status:  500,
	}
}

func PasswordChangeRequired() *APIError {
	return &APIError{
		Code:    "PASSWORD_CHANGE_REQUIRED",
		Message: "password change required before accessing other resources",
		Status:  403,
	}
}

func ServiceUnavailable(msg string) *APIError {
	return &APIError{
		Code:    "SERVICE_UNAVAILABLE",
		Message: msg,
		Status:  503,
	}
}

func BadGateway(msg string) *APIError {
	return &APIError{
		Code:    "BAD_GATEWAY",
		Message: msg,
		Status:  502,
	}
}
