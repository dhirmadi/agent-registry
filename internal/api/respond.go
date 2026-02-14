package api

import (
	"encoding/json"
	"net/http"
	"time"

	apierrors "github.com/agent-smit/agentic-registry/internal/errors"
)

// Envelope is the standard API response format.
type Envelope struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Error   interface{} `json:"error"`
	Meta    Meta        `json:"meta"`
}

// Meta contains request metadata.
type Meta struct {
	Timestamp string `json:"timestamp"`
	RequestID string `json:"request_id"`
}

func newMeta(r *http.Request) Meta {
	reqID := ""
	if r != nil {
		reqID = r.Header.Get("X-Request-Id")
	}
	return Meta{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		RequestID: reqID,
	}
}

// RespondJSON writes a success JSON response with the standard envelope.
func RespondJSON(w http.ResponseWriter, r *http.Request, status int, data interface{}) {
	env := Envelope{
		Success: true,
		Data:    data,
		Meta:    newMeta(r),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(env)
}

// RespondError writes an error JSON response with the standard envelope.
func RespondError(w http.ResponseWriter, r *http.Request, err *apierrors.APIError) {
	env := Envelope{
		Success: false,
		Error: map[string]string{
			"code":    err.Code,
			"message": err.Message,
		},
		Meta: newMeta(r),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.Status)
	json.NewEncoder(w).Encode(env)
}

// RespondNoContent writes a 204 No Content response.
func RespondNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}
