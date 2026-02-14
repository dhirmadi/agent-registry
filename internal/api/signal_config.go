package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/auth"
	apierrors "github.com/agent-smit/agentic-registry/internal/errors"
	"github.com/agent-smit/agentic-registry/internal/notify"
	"github.com/agent-smit/agentic-registry/internal/store"
)

// SignalConfigStoreForAPI is the interface the signal config handler needs from the store.
type SignalConfigStoreForAPI interface {
	List(ctx context.Context) ([]store.SignalConfig, error)
	GetByID(ctx context.Context, id uuid.UUID) (*store.SignalConfig, error)
	Update(ctx context.Context, config *store.SignalConfig, etag time.Time) error
}

// SignalConfigHandler provides HTTP handlers for signal config endpoints.
type SignalConfigHandler struct {
	signals    SignalConfigStoreForAPI
	audit      AuditStoreForAPI
	dispatcher notify.EventDispatcher
}

// NewSignalConfigHandler creates a new SignalConfigHandler.
func NewSignalConfigHandler(signals SignalConfigStoreForAPI, audit AuditStoreForAPI, dispatcher notify.EventDispatcher) *SignalConfigHandler {
	return &SignalConfigHandler{
		signals:    signals,
		audit:      audit,
		dispatcher: dispatcher,
	}
}

// List handles GET /api/v1/signal-config.
func (h *SignalConfigHandler) List(w http.ResponseWriter, r *http.Request) {
	signals, err := h.signals.List(r.Context())
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to list signal configs"))
		return
	}

	if signals == nil {
		signals = []store.SignalConfig{}
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"signals": signals,
	})
}

type updateSignalConfigRequest struct {
	PollInterval string `json:"poll_interval"`
	IsEnabled    bool   `json:"is_enabled"`
}

// Update handles PUT /api/v1/signal-config/{signalId}.
func (h *SignalConfigHandler) Update(w http.ResponseWriter, r *http.Request) {
	signalID, err := uuid.Parse(chi.URLParam(r, "signalId"))
	if err != nil {
		RespondError(w, r, apierrors.Validation("invalid signal ID"))
		return
	}

	ifMatch := r.Header.Get("If-Match")
	if ifMatch == "" {
		RespondError(w, r, apierrors.Validation("If-Match header is required for updates"))
		return
	}

	etag, err := time.Parse(time.RFC3339Nano, ifMatch)
	if err != nil {
		RespondError(w, r, apierrors.Validation("invalid If-Match value"))
		return
	}

	sc, err := h.signals.GetByID(r.Context(), signalID)
	if err != nil {
		RespondError(w, r, apierrors.NotFound("signal_config", signalID.String()))
		return
	}

	var req updateSignalConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	// Validate poll_interval is parseable and >= 1s
	dur, err := time.ParseDuration(req.PollInterval)
	if err != nil {
		RespondError(w, r, apierrors.Validation("invalid poll_interval: must be a valid duration"))
		return
	}
	if dur < time.Second {
		RespondError(w, r, apierrors.Validation("poll_interval must be at least 1s"))
		return
	}

	sc.PollInterval = req.PollInterval
	sc.IsEnabled = req.IsEnabled

	if err := h.signals.Update(r.Context(), sc, etag); err != nil {
		RespondError(w, r, apierrors.Conflict("signal config was modified by another request"))
		return
	}

	h.auditLog(r, "signal_config_update", "signal_config", sc.ID.String())
	h.dispatchEvent(r, "signal_config.updated", "signal_config", sc.ID.String())

	RespondJSON(w, r, http.StatusOK, sc)
}

func (h *SignalConfigHandler) auditLog(r *http.Request, action, resourceType, resourceID string) {
	if h.audit == nil {
		return
	}
	callerID, _ := auth.UserIDFromContext(r.Context())
	h.audit.Insert(r.Context(), &store.AuditEntry{
		Actor:        callerID.String(),
		ActorID:      &callerID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		IPAddress:    clientIPFromRequest(r),
	})
}

func (h *SignalConfigHandler) dispatchEvent(r *http.Request, eventType, resourceType, resourceID string) {
	if h.dispatcher == nil {
		return
	}
	callerID, _ := auth.UserIDFromContext(r.Context())
	h.dispatcher.Dispatch(notify.Event{
		Type:         eventType,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
		Actor:        callerID.String(),
	})
}
