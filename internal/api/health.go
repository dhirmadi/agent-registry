package api

import (
	"context"
	"net/http"
)

// Pinger is an interface for checking database connectivity.
type Pinger interface {
	Ping(ctx context.Context) error
}

// HealthHandler provides HTTP handlers for health check endpoints.
type HealthHandler struct {
	DB Pinger
}

// Healthz is a liveness probe. Returns 200 if the process is running.
func (h *HealthHandler) Healthz(w http.ResponseWriter, r *http.Request) {
	RespondJSON(w, r, http.StatusOK, map[string]string{"status": "ok"})
}

// Readyz is a readiness probe. Returns 200 if the database is reachable.
func (h *HealthHandler) Readyz(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		RespondJSON(w, r, http.StatusServiceUnavailable, map[string]string{"status": "not ready"})
		return
	}

	if err := h.DB.Ping(r.Context()); err != nil {
		RespondJSON(w, r, http.StatusServiceUnavailable, map[string]string{"status": "not ready"})
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]string{"status": "ready"})
}
