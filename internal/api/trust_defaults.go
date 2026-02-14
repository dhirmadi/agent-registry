package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/auth"
	apierrors "github.com/agent-smit/agentic-registry/internal/errors"
	"github.com/agent-smit/agentic-registry/internal/notify"
	"github.com/agent-smit/agentic-registry/internal/store"
)

// TrustDefaultStoreForAPI is the interface the trust defaults handler needs from the store.
type TrustDefaultStoreForAPI interface {
	List(ctx context.Context) ([]store.TrustDefault, error)
	GetByID(ctx context.Context, id uuid.UUID) (*store.TrustDefault, error)
	Update(ctx context.Context, d *store.TrustDefault) error
}

// TrustDefaultsHandler provides HTTP handlers for trust default endpoints.
type TrustDefaultsHandler struct {
	defaults   TrustDefaultStoreForAPI
	audit      AuditStoreForAPI
	dispatcher notify.EventDispatcher
}

// NewTrustDefaultsHandler creates a new TrustDefaultsHandler.
func NewTrustDefaultsHandler(defaults TrustDefaultStoreForAPI, audit AuditStoreForAPI, dispatcher notify.EventDispatcher) *TrustDefaultsHandler {
	return &TrustDefaultsHandler{
		defaults:   defaults,
		audit:      audit,
		dispatcher: dispatcher,
	}
}

// List handles GET /api/v1/trust-defaults.
func (h *TrustDefaultsHandler) List(w http.ResponseWriter, r *http.Request) {
	defaults, err := h.defaults.List(r.Context())
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to list trust defaults"))
		return
	}

	if defaults == nil {
		defaults = []store.TrustDefault{}
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"defaults": defaults,
		"total":    len(defaults),
	})
}

type updateTrustDefaultRequest struct {
	Patterns json.RawMessage `json:"patterns"`
}

// Update handles PUT /api/v1/trust-defaults/{defaultId}.
func (h *TrustDefaultsHandler) Update(w http.ResponseWriter, r *http.Request) {
	defaultID, err := uuid.Parse(chi.URLParam(r, "defaultId"))
	if err != nil {
		RespondError(w, r, apierrors.Validation("invalid default ID"))
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

	d, err := h.defaults.GetByID(r.Context(), defaultID)
	if err != nil {
		RespondError(w, r, apierrors.NotFound("trust_default", defaultID.String()))
		return
	}

	var req updateTrustDefaultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	if req.Patterns != nil {
		d.Patterns = req.Patterns
	}

	d.UpdatedAt = etag

	if err := h.defaults.Update(r.Context(), d); err != nil {
		RespondError(w, r, apierrors.Conflict("trust default was modified by another request"))
		return
	}

	h.auditLog(r, "trust_default_update", "trust_default", d.ID.String())
	h.dispatchEvent(r, "trust_default.changed", "trust_default", d.ID.String())

	RespondJSON(w, r, http.StatusOK, d)
}

func (h *TrustDefaultsHandler) auditLog(r *http.Request, action, resourceType, resourceID string) {
	if h.audit == nil {
		return
	}
	callerID, _ := auth.UserIDFromContext(r.Context())
	if err := h.audit.Insert(r.Context(), &store.AuditEntry{
		Actor:        callerID.String(),
		ActorID:      &callerID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		IPAddress:    clientIPFromRequest(r),
	}); err != nil {
		log.Printf("audit log failed for %s %s/%s: %v", action, resourceType, resourceID, err)
	}
}

func (h *TrustDefaultsHandler) dispatchEvent(r *http.Request, eventType, resourceType, resourceID string) {
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
