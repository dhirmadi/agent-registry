package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/auth"
	apierrors "github.com/agent-smit/agentic-registry/internal/errors"
	"github.com/agent-smit/agentic-registry/internal/notify"
	"github.com/agent-smit/agentic-registry/internal/store"
)

// TrustRuleStoreForAPI is the interface the trust rules handler needs from the store.
type TrustRuleStoreForAPI interface {
	List(ctx context.Context, workspaceID uuid.UUID) ([]store.TrustRule, error)
	Upsert(ctx context.Context, rule *store.TrustRule) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// TrustRulesHandler provides HTTP handlers for trust rule endpoints.
type TrustRulesHandler struct {
	rules      TrustRuleStoreForAPI
	audit      AuditStoreForAPI
	dispatcher notify.EventDispatcher
}

// NewTrustRulesHandler creates a new TrustRulesHandler.
func NewTrustRulesHandler(rules TrustRuleStoreForAPI, audit AuditStoreForAPI, dispatcher notify.EventDispatcher) *TrustRulesHandler {
	return &TrustRulesHandler{
		rules:      rules,
		audit:      audit,
		dispatcher: dispatcher,
	}
}

var validTiers = map[string]bool{
	"auto":   true,
	"review": true,
	"block":  true,
}

// safeToolPatternRegex allows alphanumeric, underscores, hyphens, dots, and glob wildcards (* and ?).
var safeToolPatternRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-.*?]+$`)

// validateToolPattern checks that a tool pattern is safe and well-formed.
func validateToolPattern(pattern string) error {
	if len(pattern) > 200 {
		return apierrors.Validation("tool_pattern must not exceed 200 characters")
	}
	// Reject null bytes and control characters
	for _, r := range pattern {
		if r == 0 || unicode.IsControl(r) {
			return apierrors.Validation("tool_pattern must not contain control characters or null bytes")
		}
	}
	// Reject shell metacharacters, path traversal, etc.
	if strings.Contains(pattern, "..") || strings.Contains(pattern, "/") || strings.Contains(pattern, ";") {
		return apierrors.Validation("tool_pattern must not contain path traversal or shell metacharacters")
	}
	// Reject nested grouping (regex DoS patterns like ((a*)*))
	if strings.Contains(pattern, "(") || strings.Contains(pattern, ")") {
		return apierrors.Validation("tool_pattern must not contain parentheses")
	}
	if !safeToolPatternRegex.MatchString(pattern) {
		return apierrors.Validation("tool_pattern contains invalid characters; use only alphanumeric, underscore, hyphen, dot, *, ?")
	}
	return nil
}

type createTrustRuleRequest struct {
	ToolPattern string `json:"tool_pattern"`
	Tier        string `json:"tier"`
}

// Create handles POST /api/v1/workspaces/{workspaceId}/trust-rules.
func (h *TrustRulesHandler) Create(w http.ResponseWriter, r *http.Request) {
	wsID, err := uuid.Parse(chi.URLParam(r, "workspaceId"))
	if err != nil {
		RespondError(w, r, apierrors.Validation("invalid workspace ID"))
		return
	}

	var req createTrustRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	if req.ToolPattern == "" {
		RespondError(w, r, apierrors.Validation("tool_pattern is required"))
		return
	}
	if err := validateToolPattern(req.ToolPattern); err != nil {
		RespondError(w, r, err.(*apierrors.APIError))
		return
	}
	if req.Tier == "" {
		RespondError(w, r, apierrors.Validation("tier is required"))
		return
	}
	if !validTiers[req.Tier] {
		RespondError(w, r, apierrors.Validation("tier must be one of: auto, review, block"))
		return
	}

	callerID, _ := auth.UserIDFromContext(r.Context())
	rule := &store.TrustRule{
		WorkspaceID: wsID,
		ToolPattern: req.ToolPattern,
		Tier:        req.Tier,
		CreatedBy:   callerID.String(),
	}

	if err := h.rules.Upsert(r.Context(), rule); err != nil {
		RespondError(w, r, apierrors.Internal("failed to create trust rule"))
		return
	}

	h.auditLog(r, "trust_rule_upsert", "trust_rule", rule.ID.String())
	h.dispatchEvent(r, "trust_rule.changed", "trust_rule", rule.ID.String())

	RespondJSON(w, r, http.StatusCreated, rule)
}

// List handles GET /api/v1/workspaces/{workspaceId}/trust-rules.
func (h *TrustRulesHandler) List(w http.ResponseWriter, r *http.Request) {
	wsID, err := uuid.Parse(chi.URLParam(r, "workspaceId"))
	if err != nil {
		RespondError(w, r, apierrors.Validation("invalid workspace ID"))
		return
	}

	rules, err := h.rules.List(r.Context(), wsID)
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to list trust rules"))
		return
	}

	if rules == nil {
		rules = []store.TrustRule{}
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"rules": rules,
		"total": len(rules),
	})
}

// Delete handles DELETE /api/v1/workspaces/{workspaceId}/trust-rules/{ruleId}.
func (h *TrustRulesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ruleID, err := uuid.Parse(chi.URLParam(r, "ruleId"))
	if err != nil {
		RespondError(w, r, apierrors.Validation("invalid rule ID"))
		return
	}

	if err := h.rules.Delete(r.Context(), ruleID); err != nil {
		RespondError(w, r, apierrors.NotFound("trust_rule", ruleID.String()))
		return
	}

	h.auditLog(r, "trust_rule_delete", "trust_rule", ruleID.String())
	h.dispatchEvent(r, "trust_rule.changed", "trust_rule", ruleID.String())

	RespondNoContent(w)
}

func (h *TrustRulesHandler) auditLog(r *http.Request, action, resourceType, resourceID string) {
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

func (h *TrustRulesHandler) dispatchEvent(r *http.Request, eventType, resourceType, resourceID string) {
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
