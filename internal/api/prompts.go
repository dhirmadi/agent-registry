package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/auth"
	apierrors "github.com/agent-smit/agentic-registry/internal/errors"
	"github.com/agent-smit/agentic-registry/internal/store"
)

var validPromptModes = map[string]bool{
	"rag_readonly":      true,
	"toolcalling_safe":  true,
	"toolcalling_auto":  true,
}

// PromptStoreForAPI is the interface the prompts handler needs from the store.
type PromptStoreForAPI interface {
	List(ctx context.Context, agentID string, activeOnly bool, offset, limit int) ([]store.Prompt, int, error)
	GetActive(ctx context.Context, agentID string) (*store.Prompt, error)
	GetByID(ctx context.Context, id uuid.UUID) (*store.Prompt, error)
	Create(ctx context.Context, prompt *store.Prompt) error
	Activate(ctx context.Context, id uuid.UUID) (*store.Prompt, error)
	Rollback(ctx context.Context, agentID string, targetVersion int, actor string) (*store.Prompt, error)
}

// AgentLookupForPrompts is the minimal interface prompts handler needs to check agent existence.
type AgentLookupForPrompts interface {
	GetByID(ctx context.Context, id string) (*store.Agent, error)
}

// PromptsHandler provides HTTP handlers for prompt management endpoints.
type PromptsHandler struct {
	prompts PromptStoreForAPI
	agents  AgentLookupForPrompts
	audit   AuditStoreForAPI
}

// NewPromptsHandler creates a new PromptsHandler.
func NewPromptsHandler(prompts PromptStoreForAPI, agents AgentLookupForPrompts, audit AuditStoreForAPI) *PromptsHandler {
	return &PromptsHandler{
		prompts: prompts,
		agents:  agents,
		audit:   audit,
	}
}

type createPromptRequest struct {
	SystemPrompt string          `json:"system_prompt"`
	TemplateVars json.RawMessage `json:"template_vars"`
	Mode         string          `json:"mode"`
}

// List handles GET /api/v1/agents/{agentId}/prompts.
func (h *PromptsHandler) List(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")

	activeOnly := false
	if r.URL.Query().Get("active_only") == "true" {
		activeOnly = true
	}

	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	prompts, total, err := h.prompts.List(r.Context(), agentID, activeOnly, offset, limit)
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to list prompts"))
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"prompts": prompts,
		"total":   total,
	})
}

// GetActive handles GET /api/v1/agents/{agentId}/prompts/active.
func (h *PromptsHandler) GetActive(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")

	prompt, err := h.prompts.GetActive(r.Context(), agentID)
	if err != nil {
		RespondError(w, r, apierrors.NotFound("prompt", "active for agent '"+agentID+"'"))
		return
	}

	RespondJSON(w, r, http.StatusOK, prompt)
}

// GetByID handles GET /api/v1/agents/{agentId}/prompts/{promptId}.
func (h *PromptsHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	promptIDStr := chi.URLParam(r, "promptId")
	promptID, err := uuid.Parse(promptIDStr)
	if err != nil {
		RespondError(w, r, apierrors.Validation("invalid prompt ID"))
		return
	}

	prompt, err := h.prompts.GetByID(r.Context(), promptID)
	if err != nil {
		RespondError(w, r, apierrors.NotFound("prompt", promptIDStr))
		return
	}

	RespondJSON(w, r, http.StatusOK, prompt)
}

// Create handles POST /api/v1/agents/{agentId}/prompts.
func (h *PromptsHandler) Create(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")

	// Verify agent exists
	if _, err := h.agents.GetByID(r.Context(), agentID); err != nil {
		RespondError(w, r, apierrors.NotFound("agent", agentID))
		return
	}

	var req createPromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	if req.SystemPrompt == "" {
		RespondError(w, r, apierrors.Validation("system_prompt is required"))
		return
	}

	if req.Mode == "" {
		req.Mode = "toolcalling_safe"
	}
	if !validPromptModes[req.Mode] {
		RespondError(w, r, apierrors.Validation("mode must be one of: rag_readonly, toolcalling_safe, toolcalling_auto"))
		return
	}

	if req.TemplateVars == nil {
		req.TemplateVars = json.RawMessage(`{}`)
	}

	userID, _ := auth.UserIDFromContext(r.Context())
	prompt := &store.Prompt{
		AgentID:      agentID,
		SystemPrompt: req.SystemPrompt,
		TemplateVars: req.TemplateVars,
		Mode:         req.Mode,
		CreatedBy:    userID.String(),
	}

	if err := h.prompts.Create(r.Context(), prompt); err != nil {
		RespondError(w, r, apierrors.Internal("failed to create prompt"))
		return
	}

	h.auditLog(r, "prompt_create", "prompt", prompt.ID.String())

	RespondJSON(w, r, http.StatusCreated, prompt)
}

// Activate handles POST /api/v1/agents/{agentId}/prompts/{promptId}/activate.
func (h *PromptsHandler) Activate(w http.ResponseWriter, r *http.Request) {
	promptIDStr := chi.URLParam(r, "promptId")
	promptID, err := uuid.Parse(promptIDStr)
	if err != nil {
		RespondError(w, r, apierrors.Validation("invalid prompt ID"))
		return
	}

	prompt, err := h.prompts.Activate(r.Context(), promptID)
	if err != nil {
		RespondError(w, r, apierrors.NotFound("prompt", promptIDStr))
		return
	}

	h.auditLog(r, "prompt_activate", "prompt", promptIDStr)

	RespondJSON(w, r, http.StatusOK, prompt)
}

type promptRollbackRequest struct {
	TargetVersion *int `json:"target_version"`
}

// Rollback handles POST /api/v1/agents/{agentId}/prompts/rollback.
func (h *PromptsHandler) Rollback(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")

	var req promptRollbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	if req.TargetVersion == nil || *req.TargetVersion <= 0 {
		RespondError(w, r, apierrors.Validation("target_version is required and must be positive"))
		return
	}

	userID, _ := auth.UserIDFromContext(r.Context())

	prompt, err := h.prompts.Rollback(r.Context(), agentID, *req.TargetVersion, userID.String())
	if err != nil {
		if isNotFoundError(err) {
			RespondError(w, r, apierrors.NotFound("prompt_version", agentID))
			return
		}
		RespondError(w, r, apierrors.Internal("failed to rollback prompt"))
		return
	}

	h.auditLog(r, "prompt_rollback", "prompt", prompt.ID.String())

	RespondJSON(w, r, http.StatusOK, prompt)
}

func (h *PromptsHandler) auditLog(r *http.Request, action, resourceType, resourceID string) {
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
