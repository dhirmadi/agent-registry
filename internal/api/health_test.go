package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockPinger implements the Pinger interface for testing.
type mockPinger struct {
	err error
}

func (m *mockPinger) Ping(ctx context.Context) error {
	return m.err
}

func TestHealthzHandler(t *testing.T) {
	h := &HealthHandler{}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	h.Healthz(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatal("expected data to be a map")
	}
	if data["status"] != "ok" {
		t.Errorf("status = %v, want ok", data["status"])
	}
}

func TestReadyzHandler(t *testing.T) {
	tests := []struct {
		name       string
		pingErr    error
		wantStatus int
		wantData   string
	}{
		{
			name:       "db reachable",
			pingErr:    nil,
			wantStatus: http.StatusOK,
			wantData:   "ready",
		},
		{
			name:       "db unreachable",
			pingErr:    errors.New("connection refused"),
			wantStatus: http.StatusServiceUnavailable,
			wantData:   "not ready",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := &HealthHandler{
				DB: &mockPinger{err: tc.pingErr},
			}
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/readyz", nil)

			h.Readyz(w, r)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}

			var body map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}
			data, ok := body["data"].(map[string]any)
			if !ok {
				t.Fatal("expected data to be a map")
			}
			if data["status"] != tc.wantData {
				t.Errorf("status = %v, want %v", data["status"], tc.wantData)
			}
		})
	}
}
