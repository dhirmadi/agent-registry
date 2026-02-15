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
	"github.com/agent-smit/agentic-registry/internal/store"
)

// APIKeyStoreForAPI is the interface the API keys handler needs from the store.
type APIKeyStoreForAPI interface {
	Create(ctx context.Context, key *store.APIKey) error
	List(ctx context.Context, userID *uuid.UUID) ([]store.APIKey, error)
	Delete(ctx context.Context, id uuid.UUID) error
	GetByID(ctx context.Context, id uuid.UUID) (*store.APIKey, error)
}

// APIKeysHandler provides HTTP handlers for API key management endpoints.
type APIKeysHandler struct {
	apiKeys APIKeyStoreForAPI
	audit   AuditStoreForAPI
}

// NewAPIKeysHandler creates a new APIKeysHandler.
func NewAPIKeysHandler(apiKeys APIKeyStoreForAPI, audit AuditStoreForAPI) *APIKeysHandler {
	return &APIKeysHandler{
		apiKeys: apiKeys,
		audit:   audit,
	}
}

type createAPIKeyRequest struct {
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	ExpiresAt *string  `json:"expires_at"`
}

type apiKeyCreateResponse struct {
	Key       string    `json:"key"`
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Scopes    []string  `json:"scopes"`
	KeyPrefix string    `json:"key_prefix"`
	CreatedAt time.Time `json:"created_at"`
}

type apiKeyListItem struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"key_prefix"`
	Scopes     []string   `json:"scopes"`
	IsActive   bool       `json:"is_active"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
}

// Create handles POST /api/v1/api-keys.
func (h *APIKeysHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())

	var req createAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	if req.Name == "" {
		RespondError(w, r, apierrors.Validation("name is required"))
		return
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			RespondError(w, r, apierrors.Validation("invalid expires_at format"))
			return
		}
		expiresAt = &t
	}

	plaintext, hash, prefix, err := auth.GenerateAPIKey()
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to generate API key"))
		return
	}

	apiKey := &store.APIKey{
		UserID:    &userID,
		Name:      req.Name,
		KeyPrefix: prefix,
		KeyHash:   hash,
		Scopes:    req.Scopes,
		IsActive:  true,
		ExpiresAt: expiresAt,
	}

	if err := h.apiKeys.Create(r.Context(), apiKey); err != nil {
		RespondError(w, r, apierrors.Internal("failed to create API key"))
		return
	}

	h.auditLog(r, "api_key_create", "api_key", apiKey.ID.String())

	RespondJSON(w, r, http.StatusCreated, apiKeyCreateResponse{
		Key:       plaintext,
		ID:        apiKey.ID,
		Name:      apiKey.Name,
		Scopes:    apiKey.Scopes,
		KeyPrefix: apiKey.KeyPrefix,
		CreatedAt: apiKey.CreatedAt,
	})
}

// List handles GET /api/v1/api-keys.
func (h *APIKeysHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())
	role, _ := auth.UserRoleFromContext(r.Context())

	var filterUserID *uuid.UUID
	if role != "admin" {
		filterUserID = &userID
	}

	keys, err := h.apiKeys.List(r.Context(), filterUserID)
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to list API keys"))
		return
	}

	resp := make([]apiKeyListItem, 0, len(keys))
	for _, k := range keys {
		resp = append(resp, apiKeyListItem{
			ID:         k.ID,
			Name:       k.Name,
			KeyPrefix:  k.KeyPrefix,
			Scopes:     k.Scopes,
			IsActive:   k.IsActive,
			LastUsedAt: k.LastUsedAt,
			CreatedAt:  k.CreatedAt,
			ExpiresAt:  k.ExpiresAt,
		})
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"keys":  resp,
		"total": len(resp),
	})
}

// Revoke handles DELETE /api/v1/api-keys/{keyId}.
func (h *APIKeysHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	keyID, err := uuid.Parse(chi.URLParam(r, "keyId"))
	if err != nil {
		RespondError(w, r, apierrors.Validation("invalid key ID"))
		return
	}

	userID, _ := auth.UserIDFromContext(r.Context())
	role, _ := auth.UserRoleFromContext(r.Context())

	// Non-admins can only revoke their own keys
	if role != "admin" {
		key, err := h.apiKeys.GetByID(r.Context(), keyID)
		if err != nil {
			RespondError(w, r, apierrors.NotFound("api_key", keyID.String()))
			return
		}
		if key.UserID == nil || *key.UserID != userID {
			RespondError(w, r, apierrors.Forbidden("cannot revoke another user's API key"))
			return
		}
	}

	if err := h.apiKeys.Delete(r.Context(), keyID); err != nil {
		RespondError(w, r, apierrors.NotFound("api_key", keyID.String()))
		return
	}

	h.auditLog(r, "api_key_revoke", "api_key", keyID.String())

	RespondNoContent(w)
}

func (h *APIKeysHandler) auditLog(r *http.Request, action, resourceType, resourceID string) {
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
