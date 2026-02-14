package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/auth"
	apierrors "github.com/agent-smit/agentic-registry/internal/errors"
	"github.com/agent-smit/agentic-registry/internal/store"
)

// WebhookStoreForAPI is the interface the webhooks handler needs from the store.
type WebhookStoreForAPI interface {
	Create(ctx context.Context, sub *store.WebhookSubscription) error
	List(ctx context.Context) ([]store.WebhookSubscription, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// WebhooksHandler provides HTTP handlers for webhook subscription endpoints.
type WebhooksHandler struct {
	webhooks WebhookStoreForAPI
	audit    AuditStoreForAPI
}

// NewWebhooksHandler creates a new WebhooksHandler.
func NewWebhooksHandler(webhooks WebhookStoreForAPI, audit AuditStoreForAPI) *WebhooksHandler {
	return &WebhooksHandler{
		webhooks: webhooks,
		audit:    audit,
	}
}

// List handles GET /api/v1/webhooks.
func (h *WebhooksHandler) List(w http.ResponseWriter, r *http.Request) {
	subs, err := h.webhooks.List(r.Context())
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to list webhook subscriptions"))
		return
	}

	if subs == nil {
		subs = []store.WebhookSubscription{}
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"subscriptions": subs,
		"total":         len(subs),
	})
}

type createWebhookRequest struct {
	URL    string   `json:"url"`
	Secret string   `json:"secret"`
	Events []string `json:"events"`
}

// Create handles POST /api/v1/webhooks.
func (h *WebhooksHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	if req.URL == "" {
		RespondError(w, r, apierrors.Validation("url is required"))
		return
	}
	if len(req.URL) > 2000 {
		RespondError(w, r, apierrors.Validation("url must be at most 2000 characters"))
		return
	}
	parsedURL, err := url.Parse(req.URL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		RespondError(w, r, apierrors.Validation("url must be a valid HTTP or HTTPS URL"))
		return
	}

	if len(req.Events) == 0 {
		RespondError(w, r, apierrors.Validation("events must be a non-empty array"))
		return
	}

	eventsJSON, err := json.Marshal(req.Events)
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to encode events"))
		return
	}

	sub := &store.WebhookSubscription{
		URL:      req.URL,
		Secret:   req.Secret,
		Events:   eventsJSON,
		IsActive: true,
	}

	if err := h.webhooks.Create(r.Context(), sub); err != nil {
		RespondError(w, r, apierrors.Internal("failed to create webhook subscription"))
		return
	}

	h.auditLog(r, "webhook_create", "webhook_subscription", sub.ID.String())

	RespondJSON(w, r, http.StatusCreated, sub)
}

// Delete handles DELETE /api/v1/webhooks/{webhookId}.
func (h *WebhooksHandler) Delete(w http.ResponseWriter, r *http.Request) {
	webhookID, err := uuid.Parse(chi.URLParam(r, "webhookId"))
	if err != nil {
		RespondError(w, r, apierrors.Validation("invalid webhook ID"))
		return
	}

	if err := h.webhooks.Delete(r.Context(), webhookID); err != nil {
		if isNotFoundError(err) {
			RespondError(w, r, apierrors.NotFound("webhook_subscription", webhookID.String()))
			return
		}
		RespondError(w, r, apierrors.Internal("failed to delete webhook subscription"))
		return
	}

	h.auditLog(r, "webhook_delete", "webhook_subscription", webhookID.String())

	RespondNoContent(w)
}

func (h *WebhooksHandler) auditLog(r *http.Request, action, resourceType, resourceID string) {
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