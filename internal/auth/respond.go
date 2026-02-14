package auth

import (
	"encoding/json"
	"net/http"
	"time"

	apierrors "github.com/agent-smit/agentic-registry/internal/errors"
)

// respondJSON and respondError are local helpers that avoid importing internal/api
// to prevent circular imports (api imports auth, so auth cannot import api).

type envelope struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   interface{} `json:"error,omitempty"`
	Meta    meta        `json:"meta"`
}

type meta struct {
	Timestamp string `json:"timestamp"`
	RequestID string `json:"request_id"`
}

func respondJSON(w http.ResponseWriter, r *http.Request, status int, data interface{}) {
	resp := envelope{
		Success: true,
		Data:    data,
		Meta:    makeMeta(r),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

func respondError(w http.ResponseWriter, r *http.Request, err *apierrors.APIError) {
	resp := envelope{
		Success: false,
		Error: map[string]string{
			"code":    err.Code,
			"message": err.Message,
		},
		Meta: makeMeta(r),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.Status)
	json.NewEncoder(w).Encode(resp)
}

func makeMeta(r *http.Request) meta {
	reqID := ""
	if r != nil {
		reqID = r.Header.Get("X-Request-Id")
	}
	return meta{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		RequestID: reqID,
	}
}
