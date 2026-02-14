package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/agent-smit/agentic-registry/internal/auth"
	apierrors "github.com/agent-smit/agentic-registry/internal/errors"
	"github.com/agent-smit/agentic-registry/internal/notify"
	"github.com/agent-smit/agentic-registry/internal/store"
)

// ContextConfigStoreForAPI is the interface the context config handler needs from the store.
type ContextConfigStoreForAPI interface {
	GetByScope(ctx context.Context, scope, scopeID string) (*store.ContextConfig, error)
	GetMerged(ctx context.Context, scope, scopeID string) (*store.ContextConfig, error)
	Update(ctx context.Context, config *store.ContextConfig, etag time.Time) error
	Upsert(ctx context.Context, config *store.ContextConfig) error
}

// ContextConfigHandler provides HTTP handlers for context config endpoints.
type ContextConfigHandler struct {
	configs    ContextConfigStoreForAPI
	audit      AuditStoreForAPI
	dispatcher notify.EventDispatcher
}

// NewContextConfigHandler creates a new ContextConfigHandler.
func NewContextConfigHandler(configs ContextConfigStoreForAPI, audit AuditStoreForAPI, dispatcher notify.EventDispatcher) *ContextConfigHandler {
	return &ContextConfigHandler{
		configs:    configs,
		audit:      audit,
		dispatcher: dispatcher,
	}
}

type updateContextConfigRequest struct {
	MaxTotalTokens int             `json:"max_total_tokens"`
	LayerBudgets   json.RawMessage `json:"layer_budgets"`
	EnabledLayers  json.RawMessage `json:"enabled_layers"`
}

// GetGlobal handles GET /api/v1/context-config.
func (h *ContextConfigHandler) GetGlobal(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.configs.GetByScope(r.Context(), "global", "")
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to get global context config"))
		return
	}

	RespondJSON(w, r, http.StatusOK, cfg)
}

// UpdateGlobal handles PUT /api/v1/context-config.
func (h *ContextConfigHandler) UpdateGlobal(w http.ResponseWriter, r *http.Request) {
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

	var req updateContextConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	cfg, err := h.configs.GetByScope(r.Context(), "global", "")
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to get global context config"))
		return
	}

	cfg.MaxTotalTokens = req.MaxTotalTokens
	if req.LayerBudgets != nil {
		cfg.LayerBudgets = req.LayerBudgets
	}
	if req.EnabledLayers != nil {
		cfg.EnabledLayers = req.EnabledLayers
	}

	if err := h.configs.Update(r.Context(), cfg, etag); err != nil {
		RespondError(w, r, apierrors.Conflict("context config was modified by another request"))
		return
	}

	h.auditLog(r, "context_config_update", "context_config", cfg.ID.String())
	h.dispatchEvent(r, "context_config.updated", "context_config", cfg.ID.String())

	RespondJSON(w, r, http.StatusOK, cfg)
}

// GetWorkspace handles GET /api/v1/workspaces/{workspaceId}/context-config.
// Returns the merged config (global + workspace overrides).
func (h *ContextConfigHandler) GetWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID := chi.URLParam(r, "workspaceId")
	if workspaceID == "" {
		RespondError(w, r, apierrors.Validation("workspace ID is required"))
		return
	}

	cfg, err := h.configs.GetMerged(r.Context(), "workspace", workspaceID)
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to get workspace context config"))
		return
	}

	RespondJSON(w, r, http.StatusOK, cfg)
}

// UpdateWorkspace handles PUT /api/v1/workspaces/{workspaceId}/context-config.
// Upserts the workspace-scoped config.
func (h *ContextConfigHandler) UpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID := chi.URLParam(r, "workspaceId")
	if workspaceID == "" {
		RespondError(w, r, apierrors.Validation("workspace ID is required"))
		return
	}

	var req updateContextConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	cfg := &store.ContextConfig{
		Scope:          "workspace",
		ScopeID:        workspaceID,
		MaxTotalTokens: req.MaxTotalTokens,
		LayerBudgets:   req.LayerBudgets,
		EnabledLayers:  req.EnabledLayers,
	}

	if err := h.configs.Upsert(r.Context(), cfg); err != nil {
		RespondError(w, r, apierrors.Internal("failed to upsert workspace context config"))
		return
	}

	h.auditLog(r, "context_config_update", "context_config", cfg.ID.String())
	h.dispatchEvent(r, "context_config.updated", "context_config", cfg.ID.String())

	RespondJSON(w, r, http.StatusOK, cfg)
}

func (h *ContextConfigHandler) auditLog(r *http.Request, action, resourceType, resourceID string) {
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

func (h *ContextConfigHandler) dispatchEvent(r *http.Request, eventType, resourceType, resourceID string) {
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
