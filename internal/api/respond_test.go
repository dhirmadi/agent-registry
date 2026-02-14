package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apierrors "github.com/agent-smit/agentic-registry/internal/errors"
)

func TestRespondJSON(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		data       any
		wantStatus int
		wantBody   func(t *testing.T, body map[string]any)
	}{
		{
			name:       "200 with data",
			status:     http.StatusOK,
			data:       map[string]string{"id": "abc"},
			wantStatus: 200,
			wantBody: func(t *testing.T, body map[string]any) {
				if body["success"] != true {
					t.Error("expected success=true")
				}
				data, ok := body["data"].(map[string]any)
				if !ok {
					t.Fatal("expected data to be a map")
				}
				if data["id"] != "abc" {
					t.Errorf("data.id = %v, want abc", data["id"])
				}
				if body["error"] != nil {
					t.Error("expected error=nil")
				}
				meta, ok := body["meta"].(map[string]any)
				if !ok {
					t.Fatal("expected meta to be a map")
				}
				if meta["timestamp"] == nil {
					t.Error("expected meta.timestamp")
				}
				if meta["request_id"] == nil {
					t.Error("expected meta.request_id")
				}
			},
		},
		{
			name:       "201 with data",
			status:     http.StatusCreated,
			data:       map[string]string{"name": "test"},
			wantStatus: 201,
			wantBody: func(t *testing.T, body map[string]any) {
				if body["success"] != true {
					t.Error("expected success=true")
				}
			},
		},
		{
			name:       "200 with nil data",
			status:     http.StatusOK,
			data:       nil,
			wantStatus: 200,
			wantBody: func(t *testing.T, body map[string]any) {
				if body["success"] != true {
					t.Error("expected success=true")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/test", nil)

			RespondJSON(w, r, tc.status, tc.data)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if ct := w.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}

			var body map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("failed to unmarshal body: %v", err)
			}
			tc.wantBody(t, body)
		})
	}
}

func TestRespondError(t *testing.T) {
	tests := []struct {
		name       string
		err        *apierrors.APIError
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{
			name:       "not found",
			err:        apierrors.NotFound("agent", "abc"),
			wantStatus: 404,
			wantCode:   "NOT_FOUND",
			wantMsg:    "agent 'abc' not found",
		},
		{
			name:       "validation error",
			err:        apierrors.Validation("name is required"),
			wantStatus: 400,
			wantCode:   "VALIDATION_ERROR",
			wantMsg:    "name is required",
		},
		{
			name:       "internal error",
			err:        apierrors.Internal("something went wrong"),
			wantStatus: 500,
			wantCode:   "INTERNAL_ERROR",
			wantMsg:    "something went wrong",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/test", nil)

			RespondError(w, r, tc.err)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}

			var body map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("failed to unmarshal body: %v", err)
			}

			if body["success"] != false {
				t.Error("expected success=false")
			}
			if body["data"] != nil {
				t.Error("expected data=nil")
			}

			errObj, ok := body["error"].(map[string]any)
			if !ok {
				t.Fatal("expected error to be a map")
			}
			if errObj["code"] != tc.wantCode {
				t.Errorf("error.code = %v, want %v", errObj["code"], tc.wantCode)
			}
			if errObj["message"] != tc.wantMsg {
				t.Errorf("error.message = %v, want %v", errObj["message"], tc.wantMsg)
			}

			meta, ok := body["meta"].(map[string]any)
			if !ok {
				t.Fatal("expected meta to be a map")
			}
			if meta["timestamp"] == nil {
				t.Error("expected meta.timestamp")
			}
		})
	}
}

func TestRespondNoContent(t *testing.T) {
	w := httptest.NewRecorder()

	RespondNoContent(w)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
	if w.Body.Len() != 0 {
		t.Errorf("body length = %d, want 0", w.Body.Len())
	}
}
