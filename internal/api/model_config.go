package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/agent-smit/agentic-registry/internal/auth"
	apierrors "github.com/agent-smit/agentic-registry/internal/errors"
	"github.com/agent-smit/agentic-registry/internal/notify"
	"github.com/agent-smit/agentic-registry/internal/store"
)

// ModelConfigStoreForAPI is the interface the model config handler needs from the store.
type ModelConfigStoreForAPI interface {
	GetByScope(ctx context.Context, scope, scopeID string) (*store.ModelConfig, error)
	GetMerged(ctx context.Context, scope, scopeID string) (*store.ModelConfig, error)
	Update(ctx context.Context, config *store.ModelConfig, etag time.Time) error
	Upsert(ctx context.Context, config *store.ModelConfig) error
}

// ModelConfigHandler provides HTTP handlers for model config endpoints.
type ModelConfigHandler struct {
	configs    ModelConfigStoreForAPI
	audit      AuditStoreForAPI
	dispatcher notify.EventDispatcher
}

// NewModelConfigHandler creates a new ModelConfigHandler.
func NewModelConfigHandler(configs ModelConfigStoreForAPI, audit AuditStoreForAPI, dispatcher notify.EventDispatcher) *ModelConfigHandler {
	return &ModelConfigHandler{
		configs:    configs,
		audit:      audit,
		dispatcher: dispatcher,
	}
}

type updateModelConfigRequest struct {
	DefaultModel           *string  `json:"default_model"`
	Temperature            *float64 `json:"temperature"`
	MaxTokens              *int     `json:"max_tokens"`
	MaxToolRounds          *int     `json:"max_tool_rounds"`
	DefaultContextWindow   *int     `json:"default_context_window"`
	DefaultMaxOutputTokens *int     `json:"default_max_output_tokens"`
	HistoryTokenBudget     *int     `json:"history_token_budget"`
	MaxHistoryMessages     *int     `json:"max_history_messages"`
	EmbeddingModel         *string  `json:"embedding_model"`
}

// GetGlobal handles GET /api/v1/model-config.
func (h *ModelConfigHandler) GetGlobal(w http.ResponseWriter, r *http.Request) {
	config, err := h.configs.GetByScope(r.Context(), "global", "")
	if err != nil {
		RespondError(w, r, apierrors.NotFound("model_config", "global"))
		return
	}
	RespondJSON(w, r, http.StatusOK, config)
}

// UpdateGlobal handles PUT /api/v1/model-config.
func (h *ModelConfigHandler) UpdateGlobal(w http.ResponseWriter, r *http.Request) {
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

	config, err := h.configs.GetByScope(r.Context(), "global", "")
	if err != nil {
		RespondError(w, r, apierrors.NotFound("model_config", "global"))
		return
	}

	var req updateModelConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	applyModelConfigUpdates(config, &req)

	if err := h.configs.Update(r.Context(), config, etag); err != nil {
		RespondError(w, r, apierrors.Conflict("model config was modified by another request"))
		return
	}

	h.auditLog(r, "model_config_update", "model_config", "global")
	h.dispatchEvent(r, "model_config.updated", "model_config", "global")

	RespondJSON(w, r, http.StatusOK, config)
}

// GetWorkspace handles GET /api/v1/workspaces/{workspaceId}/model-config.
func (h *ModelConfigHandler) GetWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID := chi.URLParam(r, "workspaceId")
	if wsID == "" {
		RespondError(w, r, apierrors.Validation("workspaceId is required"))
		return
	}

	config, err := h.configs.GetMerged(r.Context(), "workspace", wsID)
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to get workspace model config"))
		return
	}

	RespondJSON(w, r, http.StatusOK, config)
}

// UpdateWorkspace handles PUT /api/v1/workspaces/{workspaceId}/model-config.
func (h *ModelConfigHandler) UpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID := chi.URLParam(r, "workspaceId")
	if wsID == "" {
		RespondError(w, r, apierrors.Validation("workspaceId is required"))
		return
	}

	var req updateModelConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	config := &store.ModelConfig{
		Scope:   "workspace",
		ScopeID: wsID,
	}
	applyModelConfigUpdates(config, &req)

	if err := h.configs.Upsert(r.Context(), config); err != nil {
		RespondError(w, r, apierrors.Internal("failed to upsert workspace model config"))
		return
	}

	h.auditLog(r, "model_config_update", "model_config", "workspace/"+wsID)
	h.dispatchEvent(r, "model_config.updated", "model_config", "workspace/"+wsID)

	RespondJSON(w, r, http.StatusOK, config)
}

func applyModelConfigUpdates(config *store.ModelConfig, req *updateModelConfigRequest) {
	if req.DefaultModel != nil {
		config.DefaultModel = *req.DefaultModel
	}
	if req.Temperature != nil {
		config.Temperature = *req.Temperature
	}
	if req.MaxTokens != nil {
		config.MaxTokens = *req.MaxTokens
	}
	if req.MaxToolRounds != nil {
		config.MaxToolRounds = *req.MaxToolRounds
	}
	if req.DefaultContextWindow != nil {
		config.DefaultContextWindow = *req.DefaultContextWindow
	}
	if req.DefaultMaxOutputTokens != nil {
		config.DefaultMaxOutputTokens = *req.DefaultMaxOutputTokens
	}
	if req.HistoryTokenBudget != nil {
		config.HistoryTokenBudget = *req.HistoryTokenBudget
	}
	if req.MaxHistoryMessages != nil {
		config.MaxHistoryMessages = *req.MaxHistoryMessages
	}
	if req.EmbeddingModel != nil {
		config.EmbeddingModel = *req.EmbeddingModel
	}
}

func (h *ModelConfigHandler) auditLog(r *http.Request, action, resourceType, resourceID string) {
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

func (h *ModelConfigHandler) dispatchEvent(r *http.Request, eventType, resourceType, resourceID string) {
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
