package api

import (
	"context"
	"net/http"
	"strconv"

	apierrors "github.com/agent-smit/agentic-registry/internal/errors"
	"github.com/agent-smit/agentic-registry/internal/store"
)

// AuditLogStore is the interface the audit handler needs from the audit store.
type AuditLogStore interface {
	List(ctx context.Context, filter store.AuditFilter) ([]store.AuditEntry, int, error)
}

// AuditHandler provides the HTTP handler for listing audit log entries.
type AuditHandler struct {
	store AuditLogStore
}

// NewAuditHandler creates a new AuditHandler.
func NewAuditHandler(s AuditLogStore) *AuditHandler {
	return &AuditHandler{store: s}
}

// List handles GET /api/v1/audit-log
func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	offset, _ := strconv.Atoi(q.Get("offset"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 50
	}

	filter := store.AuditFilter{
		Actor:        q.Get("actor"),
		Action:       q.Get("action"),
		ResourceType: q.Get("resource_type"),
		ResourceID:   q.Get("resource_id"),
		Offset:       offset,
		Limit:        limit,
	}

	entries, total, err := h.store.List(r.Context(), filter)
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to load audit log"))
		return
	}

	if entries == nil {
		entries = []store.AuditEntry{}
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"items": entries,
		"total": total,
	})
}
