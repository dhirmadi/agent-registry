package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	"github.com/agent-smit/agentic-registry/internal/auth"
	apierrors "github.com/agent-smit/agentic-registry/internal/errors"
	"github.com/agent-smit/agentic-registry/internal/notify"
	"github.com/agent-smit/agentic-registry/internal/store"
)

// TriggerRuleStoreForAPI is the interface the trigger rules handler needs from the store.
type TriggerRuleStoreForAPI interface {
	List(ctx context.Context, workspaceID uuid.UUID) ([]store.TriggerRule, error)
	GetByID(ctx context.Context, id uuid.UUID) (*store.TriggerRule, error)
	Create(ctx context.Context, rule *store.TriggerRule) error
	Update(ctx context.Context, rule *store.TriggerRule) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// AgentLookupForAPI is the interface for checking agent existence.
type AgentLookupForAPI interface {
	AgentExists(ctx context.Context, agentID string) (bool, error)
}

// TriggerRulesHandler provides HTTP handlers for trigger rule endpoints.
type TriggerRulesHandler struct {
	triggers   TriggerRuleStoreForAPI
	audit      AuditStoreForAPI
	agents     AgentLookupForAPI
	dispatcher notify.EventDispatcher
}

// NewTriggerRulesHandler creates a new TriggerRulesHandler.
func NewTriggerRulesHandler(triggers TriggerRuleStoreForAPI, audit AuditStoreForAPI, agents AgentLookupForAPI, dispatcher notify.EventDispatcher) *TriggerRulesHandler {
	return &TriggerRulesHandler{
		triggers:   triggers,
		audit:      audit,
		agents:     agents,
		dispatcher: dispatcher,
	}
}

var validEventTypes = map[string]bool{
	"push":                  true,
	"scheduled_tick":        true,
	"webhook":               true,
	"email.ingested":        true,
	"calendar.change":       true,
	"transcript.available":  true,
	"drive.file_changed":    true,
	"slack.message":         true,
	"file_changed":          true,
	"nudge_created":         true,
}

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

type createTriggerRuleRequest struct {
	Name             string          `json:"name"`
	EventType        string          `json:"event_type"`
	Condition        json.RawMessage `json:"condition"`
	AgentID          string          `json:"agent_id"`
	PromptTemplate   string          `json:"prompt_template"`
	Enabled          *bool           `json:"enabled"`
	RateLimitPerHour *int            `json:"rate_limit_per_hour"`
	Schedule         string          `json:"schedule"`
	RunAsUserID      *uuid.UUID      `json:"run_as_user_id"`
}

// Create handles POST /api/v1/workspaces/{workspaceId}/trigger-rules.
func (h *TriggerRulesHandler) Create(w http.ResponseWriter, r *http.Request) {
	wsID, err := uuid.Parse(chi.URLParam(r, "workspaceId"))
	if err != nil {
		RespondError(w, r, apierrors.Validation("invalid workspace ID"))
		return
	}

	var req createTriggerRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	if req.Name == "" {
		RespondError(w, r, apierrors.Validation("name is required"))
		return
	}
	if req.EventType == "" {
		RespondError(w, r, apierrors.Validation("event_type is required"))
		return
	}
	if !validEventTypes[req.EventType] {
		RespondError(w, r, apierrors.Validation("invalid event_type"))
		return
	}
	if req.AgentID == "" {
		RespondError(w, r, apierrors.Validation("agent_id is required"))
		return
	}

	// Validate agent exists
	if h.agents != nil {
		exists, err := h.agents.AgentExists(r.Context(), req.AgentID)
		if err != nil {
			RespondError(w, r, apierrors.Internal("failed to validate agent"))
			return
		}
		if !exists {
			RespondError(w, r, apierrors.Validation("agent_id references a non-existent agent"))
			return
		}
	}

	// Validate cron schedule if provided
	if req.Schedule != "" {
		if _, err := cronParser.Parse(req.Schedule); err != nil {
			RespondError(w, r, apierrors.Validation("invalid cron schedule expression"))
			return
		}
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	rateLimitPerHour := 10
	if req.RateLimitPerHour != nil {
		rateLimitPerHour = *req.RateLimitPerHour
	}

	rule := &store.TriggerRule{
		WorkspaceID:      wsID,
		Name:             req.Name,
		EventType:        req.EventType,
		Condition:        req.Condition,
		AgentID:          req.AgentID,
		PromptTemplate:   req.PromptTemplate,
		Enabled:          enabled,
		RateLimitPerHour: rateLimitPerHour,
		Schedule:         req.Schedule,
		RunAsUserID:      req.RunAsUserID,
	}

	if err := h.triggers.Create(r.Context(), rule); err != nil {
		RespondError(w, r, apierrors.Internal("failed to create trigger rule"))
		return
	}

	h.auditLog(r, "trigger_rule_create", "trigger_rule", rule.ID.String())
	h.dispatchEvent(r, "trigger_rule.changed", "trigger_rule", rule.ID.String())

	RespondJSON(w, r, http.StatusCreated, rule)
}

// List handles GET /api/v1/workspaces/{workspaceId}/trigger-rules.
func (h *TriggerRulesHandler) List(w http.ResponseWriter, r *http.Request) {
	wsID, err := uuid.Parse(chi.URLParam(r, "workspaceId"))
	if err != nil {
		RespondError(w, r, apierrors.Validation("invalid workspace ID"))
		return
	}

	rules, err := h.triggers.List(r.Context(), wsID)
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to list trigger rules"))
		return
	}

	if rules == nil {
		rules = []store.TriggerRule{}
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"rules": rules,
		"total": len(rules),
	})
}

// Get handles GET /api/v1/workspaces/{workspaceId}/trigger-rules/{triggerId}.
func (h *TriggerRulesHandler) Get(w http.ResponseWriter, r *http.Request) {
	triggerID, err := uuid.Parse(chi.URLParam(r, "triggerId"))
	if err != nil {
		RespondError(w, r, apierrors.Validation("invalid trigger ID"))
		return
	}

	rule, err := h.triggers.GetByID(r.Context(), triggerID)
	if err != nil {
		RespondError(w, r, apierrors.NotFound("trigger_rule", triggerID.String()))
		return
	}

	RespondJSON(w, r, http.StatusOK, rule)
}

type updateTriggerRuleRequest struct {
	Name             *string          `json:"name"`
	EventType        *string          `json:"event_type"`
	Condition        *json.RawMessage `json:"condition"`
	AgentID          *string          `json:"agent_id"`
	PromptTemplate   *string          `json:"prompt_template"`
	Enabled          *bool            `json:"enabled"`
	RateLimitPerHour *int             `json:"rate_limit_per_hour"`
	Schedule         *string          `json:"schedule"`
	RunAsUserID      *uuid.UUID       `json:"run_as_user_id"`
}

// Update handles PUT /api/v1/workspaces/{workspaceId}/trigger-rules/{triggerId}.
func (h *TriggerRulesHandler) Update(w http.ResponseWriter, r *http.Request) {
	triggerID, err := uuid.Parse(chi.URLParam(r, "triggerId"))
	if err != nil {
		RespondError(w, r, apierrors.Validation("invalid trigger ID"))
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

	rule, err := h.triggers.GetByID(r.Context(), triggerID)
	if err != nil {
		RespondError(w, r, apierrors.NotFound("trigger_rule", triggerID.String()))
		return
	}

	var req updateTriggerRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	if req.Name != nil {
		rule.Name = *req.Name
	}
	if req.EventType != nil {
		if !validEventTypes[*req.EventType] {
			RespondError(w, r, apierrors.Validation("invalid event_type"))
			return
		}
		rule.EventType = *req.EventType
	}
	if req.Condition != nil {
		rule.Condition = *req.Condition
	}
	if req.AgentID != nil {
		if h.agents != nil {
			exists, err := h.agents.AgentExists(r.Context(), *req.AgentID)
			if err != nil {
				RespondError(w, r, apierrors.Internal("failed to validate agent"))
				return
			}
			if !exists {
				RespondError(w, r, apierrors.Validation("agent_id references a non-existent agent"))
				return
			}
		}
		rule.AgentID = *req.AgentID
	}
	if req.PromptTemplate != nil {
		rule.PromptTemplate = *req.PromptTemplate
	}
	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
	}
	if req.RateLimitPerHour != nil {
		rule.RateLimitPerHour = *req.RateLimitPerHour
	}
	if req.Schedule != nil {
		if *req.Schedule != "" {
			if _, err := cronParser.Parse(*req.Schedule); err != nil {
				RespondError(w, r, apierrors.Validation("invalid cron schedule expression"))
				return
			}
		}
		rule.Schedule = *req.Schedule
	}
	if req.RunAsUserID != nil {
		rule.RunAsUserID = req.RunAsUserID
	}

	rule.UpdatedAt = etag

	if err := h.triggers.Update(r.Context(), rule); err != nil {
		RespondError(w, r, apierrors.Conflict("trigger rule was modified by another request"))
		return
	}

	h.auditLog(r, "trigger_rule_update", "trigger_rule", rule.ID.String())
	h.dispatchEvent(r, "trigger_rule.changed", "trigger_rule", rule.ID.String())

	RespondJSON(w, r, http.StatusOK, rule)
}

// Delete handles DELETE /api/v1/workspaces/{workspaceId}/trigger-rules/{triggerId}.
func (h *TriggerRulesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	triggerID, err := uuid.Parse(chi.URLParam(r, "triggerId"))
	if err != nil {
		RespondError(w, r, apierrors.Validation("invalid trigger ID"))
		return
	}

	if err := h.triggers.Delete(r.Context(), triggerID); err != nil {
		RespondError(w, r, apierrors.NotFound("trigger_rule", triggerID.String()))
		return
	}

	h.auditLog(r, "trigger_rule_delete", "trigger_rule", triggerID.String())
	h.dispatchEvent(r, "trigger_rule.changed", "trigger_rule", triggerID.String())

	RespondNoContent(w)
}

func (h *TriggerRulesHandler) auditLog(r *http.Request, action, resourceType, resourceID string) {
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

func (h *TriggerRulesHandler) dispatchEvent(r *http.Request, eventType, resourceType, resourceID string) {
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
