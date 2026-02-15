package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/auth"
	apierrors "github.com/agent-smit/agentic-registry/internal/errors"
	"github.com/agent-smit/agentic-registry/internal/notify"
	"github.com/agent-smit/agentic-registry/internal/store"
)

// ModelEndpointStoreForAPI is the interface the model endpoints handler needs from the store.
type ModelEndpointStoreForAPI interface {
	Create(ctx context.Context, ep *store.ModelEndpoint, initialConfig json.RawMessage, changeNote string) error
	GetBySlug(ctx context.Context, slug string) (*store.ModelEndpoint, error)
	List(ctx context.Context, workspaceID *string, activeOnly bool, offset, limit int) ([]store.ModelEndpoint, int, error)
	Update(ctx context.Context, ep *store.ModelEndpoint, updatedAt time.Time) error
	Delete(ctx context.Context, slug string) error
	CreateVersion(ctx context.Context, endpointID uuid.UUID, config json.RawMessage, changeNote, createdBy string) (*store.ModelEndpointVersion, error)
	ListVersions(ctx context.Context, endpointID uuid.UUID, offset, limit int) ([]store.ModelEndpointVersion, int, error)
	GetVersion(ctx context.Context, endpointID uuid.UUID, version int) (*store.ModelEndpointVersion, error)
	GetActiveVersion(ctx context.Context, endpointID uuid.UUID) (*store.ModelEndpointVersion, error)
	ActivateVersion(ctx context.Context, endpointID uuid.UUID, version int) (*store.ModelEndpointVersion, error)
	CountAll(ctx context.Context) (int, error)
}

// ModelEndpointsHandler provides HTTP handlers for model endpoint CRUD and versioning.
type ModelEndpointsHandler struct {
	endpoints  ModelEndpointStoreForAPI
	audit      AuditStoreForAPI
	encKey     []byte
	dispatcher notify.EventDispatcher
}

// NewModelEndpointsHandler creates a new ModelEndpointsHandler.
func NewModelEndpointsHandler(endpoints ModelEndpointStoreForAPI, audit AuditStoreForAPI, encKey []byte, dispatcher notify.EventDispatcher) *ModelEndpointsHandler {
	return &ModelEndpointsHandler{
		endpoints:  endpoints,
		audit:      audit,
		encKey:     encKey,
		dispatcher: dispatcher,
	}
}

var validProviders = map[string]bool{
	"openai":    true,
	"azure":     true,
	"anthropic": true,
	"ollama":    true,
	"custom":    true,
}

var slugRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{2,98}$`)

// maxConfigSize is the maximum allowed size for a version config JSON payload (64KB).
const maxConfigSize = 64 * 1024

// Create handles POST /api/v1/model-endpoints.
func (h *ModelEndpointsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Slug          string          `json:"slug"`
		Name          string          `json:"name"`
		Provider      string          `json:"provider"`
		EndpointURL   string          `json:"endpoint_url"`
		IsFixedModel  bool            `json:"is_fixed_model"`
		ModelName     string          `json:"model_name"`
		AllowedModels json.RawMessage `json:"allowed_models"`
		WorkspaceID   *string         `json:"workspace_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	// Validate required fields
	if req.Slug == "" {
		RespondError(w, r, apierrors.Validation("slug is required"))
		return
	}
	if !slugRegex.MatchString(req.Slug) {
		RespondError(w, r, apierrors.Validation("slug must be 3-99 lowercase alphanumeric characters with hyphens, starting with a letter"))
		return
	}
	if req.Name == "" {
		RespondError(w, r, apierrors.Validation("name is required"))
		return
	}
	if req.Provider == "" {
		RespondError(w, r, apierrors.Validation("provider is required"))
		return
	}
	if !validProviders[req.Provider] {
		RespondError(w, r, apierrors.Validation("provider must be one of: openai, azure, anthropic, ollama, custom"))
		return
	}
	if req.EndpointURL == "" {
		RespondError(w, r, apierrors.Validation("endpoint_url is required"))
		return
	}

	// SSRF validation
	if err := validateEndpointURL(req.EndpointURL); err != nil {
		RespondError(w, r, apierrors.Validation(err.Error()))
		return
	}

	// Fixed/flexible model validation
	if req.IsFixedModel && req.ModelName == "" {
		RespondError(w, r, apierrors.Validation("model_name is required when is_fixed_model is true"))
		return
	}
	if req.IsFixedModel {
		// When is_fixed_model=true, allowed_models must be empty
		var models []string
		if req.AllowedModels != nil {
			if err := json.Unmarshal(req.AllowedModels, &models); err != nil {
				RespondError(w, r, apierrors.Validation("allowed_models must be a string array"))
				return
			}
		}
		if len(models) > 0 {
			RespondError(w, r, apierrors.Validation("allowed_models must be empty when is_fixed_model is true"))
			return
		}
	} else {
		// When is_fixed_model=false, allowed_models is required and non-empty
		var models []string
		if req.AllowedModels != nil {
			if err := json.Unmarshal(req.AllowedModels, &models); err != nil {
				RespondError(w, r, apierrors.Validation("allowed_models must be a string array"))
				return
			}
		}
		if len(models) == 0 {
			RespondError(w, r, apierrors.Validation("allowed_models is required when is_fixed_model is false"))
			return
		}
	}

	callerID, _ := auth.UserIDFromContext(r.Context())
	allowedModels := req.AllowedModels
	if allowedModels == nil {
		allowedModels = json.RawMessage(`[]`)
	}

	ep := &store.ModelEndpoint{
		Slug:          req.Slug,
		Name:          req.Name,
		Provider:      req.Provider,
		EndpointURL:   req.EndpointURL,
		IsFixedModel:  req.IsFixedModel,
		ModelName:     req.ModelName,
		AllowedModels: allowedModels,
		IsActive:      true,
		WorkspaceID:   req.WorkspaceID,
		CreatedBy:     callerID.String(),
	}

	initialConfig := json.RawMessage(`{}`)
	if err := h.endpoints.Create(r.Context(), ep, initialConfig, ""); err != nil {
		if strings.Contains(err.Error(), "CONFLICT") {
			RespondError(w, r, apierrors.Conflict("model endpoint with this slug already exists"))
			return
		}
		RespondError(w, r, apierrors.Internal("failed to create model endpoint"))
		return
	}

	h.auditLog(r, "model_endpoint_create", "model_endpoint", ep.Slug)
	h.dispatchEvent(r, "model_endpoint.created", "model_endpoint", ep.Slug)

	RespondJSON(w, r, http.StatusCreated, ep)
}

// Get handles GET /api/v1/model-endpoints/{slug}.
func (h *ModelEndpointsHandler) Get(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		RespondError(w, r, apierrors.Validation("slug is required"))
		return
	}

	ep, err := h.endpoints.GetBySlug(r.Context(), slug)
	if err != nil {
		if strings.Contains(err.Error(), "NOT_FOUND") {
			RespondError(w, r, apierrors.NotFound("model_endpoint", slug))
			return
		}
		RespondError(w, r, apierrors.Internal("failed to get model endpoint"))
		return
	}

	w.Header().Set("ETag", ep.UpdatedAt.UTC().Format(time.RFC3339Nano))
	RespondJSON(w, r, http.StatusOK, ep)
}

// List handles GET /api/v1/model-endpoints.
func (h *ModelEndpointsHandler) List(w http.ResponseWriter, r *http.Request) {
	activeOnly := r.URL.Query().Get("active_only") != "false"
	workspaceIDParam := r.URL.Query().Get("workspace_id")
	var workspaceID *string
	if workspaceIDParam != "" {
		workspaceID = &workspaceIDParam
	}

	offset := 0
	limit := 50
	if v := r.URL.Query().Get("offset"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 200 {
		limit = 200
	}

	endpoints, total, err := h.endpoints.List(r.Context(), workspaceID, activeOnly, offset, limit)
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to list model endpoints"))
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"model_endpoints": endpoints,
		"total":           total,
		"offset":          offset,
		"limit":           limit,
	})
}

// Update handles PUT /api/v1/model-endpoints/{slug}.
func (h *ModelEndpointsHandler) Update(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		RespondError(w, r, apierrors.Validation("slug is required"))
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

	existing, err := h.endpoints.GetBySlug(r.Context(), slug)
	if err != nil {
		if strings.Contains(err.Error(), "NOT_FOUND") {
			RespondError(w, r, apierrors.NotFound("model_endpoint", slug))
			return
		}
		RespondError(w, r, apierrors.Internal("failed to get model endpoint"))
		return
	}

	var req struct {
		Name        *string `json:"name"`
		EndpointURL *string `json:"endpoint_url"`
		ModelName   *string `json:"model_name"`
		IsActive    *bool   `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.EndpointURL != nil {
		if err := validateEndpointURL(*req.EndpointURL); err != nil {
			RespondError(w, r, apierrors.Validation(err.Error()))
			return
		}
		existing.EndpointURL = *req.EndpointURL
	}
	if req.ModelName != nil {
		existing.ModelName = *req.ModelName
	}
	if req.IsActive != nil {
		existing.IsActive = *req.IsActive
	}

	if err := h.endpoints.Update(r.Context(), existing, etag); err != nil {
		if strings.Contains(err.Error(), "CONFLICT") {
			RespondError(w, r, apierrors.Conflict("model endpoint was modified by another client"))
			return
		}
		if strings.Contains(err.Error(), "NOT_FOUND") {
			RespondError(w, r, apierrors.NotFound("model_endpoint", slug))
			return
		}
		RespondError(w, r, apierrors.Internal("failed to update model endpoint"))
		return
	}

	h.auditLog(r, "model_endpoint_update", "model_endpoint", slug)
	h.dispatchEvent(r, "model_endpoint.updated", "model_endpoint", slug)

	w.Header().Set("ETag", existing.UpdatedAt.UTC().Format(time.RFC3339Nano))
	RespondJSON(w, r, http.StatusOK, existing)
}

// Delete handles DELETE /api/v1/model-endpoints/{slug}.
func (h *ModelEndpointsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		RespondError(w, r, apierrors.Validation("slug is required"))
		return
	}

	if err := h.endpoints.Delete(r.Context(), slug); err != nil {
		if strings.Contains(err.Error(), "NOT_FOUND") {
			RespondError(w, r, apierrors.NotFound("model_endpoint", slug))
			return
		}
		RespondError(w, r, apierrors.Internal("failed to delete model endpoint"))
		return
	}

	h.auditLog(r, "model_endpoint_delete", "model_endpoint", slug)
	h.dispatchEvent(r, "model_endpoint.deleted", "model_endpoint", slug)

	RespondNoContent(w)
}

// CreateVersion handles POST /api/v1/model-endpoints/{slug}/versions.
func (h *ModelEndpointsHandler) CreateVersion(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		RespondError(w, r, apierrors.Validation("slug is required"))
		return
	}

	ep, err := h.endpoints.GetBySlug(r.Context(), slug)
	if err != nil {
		if strings.Contains(err.Error(), "NOT_FOUND") {
			RespondError(w, r, apierrors.NotFound("model_endpoint", slug))
			return
		}
		RespondError(w, r, apierrors.Internal("failed to get model endpoint"))
		return
	}

	var req struct {
		Config     json.RawMessage `json:"config"`
		ChangeNote string          `json:"change_note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	if req.Config == nil || len(req.Config) == 0 || string(req.Config) == "null" {
		RespondError(w, r, apierrors.Validation("config is required"))
		return
	}

	// Validate config size and structure
	if err := validateModelEndpointConfig(req.Config); err != nil {
		RespondError(w, r, err.(*apierrors.APIError))
		return
	}

	callerID, _ := auth.UserIDFromContext(r.Context())

	v, err := h.endpoints.CreateVersion(r.Context(), ep.ID, req.Config, req.ChangeNote, callerID.String())
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to create version"))
		return
	}

	h.auditLog(r, "model_endpoint_version_create", "model_endpoint_version", slug)
	h.dispatchEvent(r, "model_endpoint_version.created", "model_endpoint_version", slug)

	RespondJSON(w, r, http.StatusCreated, v)
}

// ListVersions handles GET /api/v1/model-endpoints/{slug}/versions.
func (h *ModelEndpointsHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		RespondError(w, r, apierrors.Validation("slug is required"))
		return
	}

	ep, err := h.endpoints.GetBySlug(r.Context(), slug)
	if err != nil {
		if strings.Contains(err.Error(), "NOT_FOUND") {
			RespondError(w, r, apierrors.NotFound("model_endpoint", slug))
			return
		}
		RespondError(w, r, apierrors.Internal("failed to get model endpoint"))
		return
	}

	offset := 0
	limit := 50
	if v := r.URL.Query().Get("offset"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	versions, total, err := h.endpoints.ListVersions(r.Context(), ep.ID, offset, limit)
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to list versions"))
		return
	}

	// Redact headers from version configs
	for i := range versions {
		versions[i].Config = redactConfigHeaders(versions[i].Config)
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"versions": versions,
		"total":    total,
		"offset":   offset,
		"limit":    limit,
	})
}

// GetVersion handles GET /api/v1/model-endpoints/{slug}/versions/{version}.
func (h *ModelEndpointsHandler) GetVersion(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	versionStr := chi.URLParam(r, "version")

	ep, err := h.endpoints.GetBySlug(r.Context(), slug)
	if err != nil {
		if strings.Contains(err.Error(), "NOT_FOUND") {
			RespondError(w, r, apierrors.NotFound("model_endpoint", slug))
			return
		}
		RespondError(w, r, apierrors.Internal("failed to get model endpoint"))
		return
	}

	version, err := strconv.Atoi(versionStr)
	if err != nil {
		RespondError(w, r, apierrors.Validation("version must be an integer"))
		return
	}

	v, err := h.endpoints.GetVersion(r.Context(), ep.ID, version)
	if err != nil {
		if strings.Contains(err.Error(), "NOT_FOUND") {
			RespondError(w, r, apierrors.NotFound("model_endpoint_version", versionStr))
			return
		}
		RespondError(w, r, apierrors.Internal("failed to get version"))
		return
	}

	// Redact headers
	v.Config = redactConfigHeaders(v.Config)

	RespondJSON(w, r, http.StatusOK, v)
}

// ActivateVersion handles POST /api/v1/model-endpoints/{slug}/versions/{version}/activate.
func (h *ModelEndpointsHandler) ActivateVersion(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	versionStr := chi.URLParam(r, "version")

	ep, err := h.endpoints.GetBySlug(r.Context(), slug)
	if err != nil {
		if strings.Contains(err.Error(), "NOT_FOUND") {
			RespondError(w, r, apierrors.NotFound("model_endpoint", slug))
			return
		}
		RespondError(w, r, apierrors.Internal("failed to get model endpoint"))
		return
	}

	version, err := strconv.Atoi(versionStr)
	if err != nil {
		RespondError(w, r, apierrors.Validation("version must be an integer"))
		return
	}

	v, err := h.endpoints.ActivateVersion(r.Context(), ep.ID, version)
	if err != nil {
		if strings.Contains(err.Error(), "NOT_FOUND") {
			RespondError(w, r, apierrors.NotFound("model_endpoint_version", versionStr))
			return
		}
		RespondError(w, r, apierrors.Internal("failed to activate version"))
		return
	}

	h.auditLog(r, "model_endpoint_version_activate", "model_endpoint_version", slug)
	h.dispatchEvent(r, "model_endpoint_version.activated", "model_endpoint_version", slug)

	// Redact headers before returning to prevent secret leakage
	v.Config = redactConfigHeaders(v.Config)

	RespondJSON(w, r, http.StatusOK, v)
}

// ListByWorkspace handles GET /api/v1/workspaces/{wid}/model-endpoints.
func (h *ModelEndpointsHandler) ListByWorkspace(w http.ResponseWriter, r *http.Request) {
	wid := chi.URLParam(r, "workspaceId")
	if wid == "" {
		RespondError(w, r, apierrors.Validation("workspace_id is required"))
		return
	}

	activeOnly := r.URL.Query().Get("active_only") != "false"
	offset := 0
	limit := 50
	if v := r.URL.Query().Get("offset"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 200 {
		limit = 200
	}

	endpoints, total, err := h.endpoints.List(r.Context(), &wid, activeOnly, offset, limit)
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to list model endpoints"))
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"model_endpoints": endpoints,
		"total":           total,
		"offset":          offset,
		"limit":           limit,
	})
}

// CreateForWorkspace handles POST /api/v1/workspaces/{wid}/model-endpoints.
func (h *ModelEndpointsHandler) CreateForWorkspace(w http.ResponseWriter, r *http.Request) {
	wid := chi.URLParam(r, "workspaceId")
	if wid == "" {
		RespondError(w, r, apierrors.Validation("workspace_id is required"))
		return
	}

	var req struct {
		Slug          string          `json:"slug"`
		Name          string          `json:"name"`
		Provider      string          `json:"provider"`
		EndpointURL   string          `json:"endpoint_url"`
		IsFixedModel  bool            `json:"is_fixed_model"`
		ModelName     string          `json:"model_name"`
		AllowedModels json.RawMessage `json:"allowed_models"`
		Config        json.RawMessage `json:"config"`
		ChangeNote    string          `json:"change_note"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	if req.Slug == "" {
		RespondError(w, r, apierrors.Validation("slug is required"))
		return
	}
	if !slugRegex.MatchString(req.Slug) {
		RespondError(w, r, apierrors.Validation("slug must be 3-99 lowercase alphanumeric characters with hyphens, starting with a letter"))
		return
	}
	if req.Name == "" {
		RespondError(w, r, apierrors.Validation("name is required"))
		return
	}
	if req.Provider == "" {
		RespondError(w, r, apierrors.Validation("provider is required"))
		return
	}
	if !validProviders[req.Provider] {
		RespondError(w, r, apierrors.Validation("provider must be one of: openai, azure, anthropic, ollama, custom"))
		return
	}
	if req.EndpointURL == "" {
		RespondError(w, r, apierrors.Validation("endpoint_url is required"))
		return
	}
	if err := validateEndpointURL(req.EndpointURL); err != nil {
		RespondError(w, r, apierrors.Validation(err.Error()))
		return
	}
	if req.IsFixedModel && req.ModelName == "" {
		RespondError(w, r, apierrors.Validation("model_name is required when is_fixed_model is true"))
		return
	}
	if req.IsFixedModel {
		// When is_fixed_model=true, allowed_models must be empty
		var models []string
		if req.AllowedModels != nil {
			if err := json.Unmarshal(req.AllowedModels, &models); err != nil {
				RespondError(w, r, apierrors.Validation("allowed_models must be a string array"))
				return
			}
		}
		if len(models) > 0 {
			RespondError(w, r, apierrors.Validation("allowed_models must be empty when is_fixed_model is true"))
			return
		}
	} else {
		// When is_fixed_model=false, allowed_models is required and non-empty
		var models []string
		if req.AllowedModels != nil {
			if err := json.Unmarshal(req.AllowedModels, &models); err != nil {
				RespondError(w, r, apierrors.Validation("allowed_models must be a string array"))
				return
			}
		}
		if len(models) == 0 {
			RespondError(w, r, apierrors.Validation("allowed_models is required when is_fixed_model is false"))
			return
		}
	}

	initialConfig := req.Config
	if initialConfig == nil {
		initialConfig = json.RawMessage(`{}`)
	}
	if err := validateModelEndpointConfig(initialConfig); err != nil {
		RespondError(w, r, err.(*apierrors.APIError))
		return
	}

	callerID, _ := auth.UserIDFromContext(r.Context())
	allowedModels := req.AllowedModels
	if allowedModels == nil {
		allowedModels = json.RawMessage(`[]`)
	}

	ep := &store.ModelEndpoint{
		Slug:          req.Slug,
		Name:          req.Name,
		Provider:      req.Provider,
		EndpointURL:   req.EndpointURL,
		IsFixedModel:  req.IsFixedModel,
		ModelName:     req.ModelName,
		AllowedModels: allowedModels,
		IsActive:      true,
		WorkspaceID:   &wid,
		CreatedBy:     callerID.String(),
	}

	if err := h.endpoints.Create(r.Context(), ep, initialConfig, req.ChangeNote); err != nil {
		if strings.Contains(err.Error(), "CONFLICT") {
			RespondError(w, r, apierrors.Conflict("model endpoint with this slug already exists"))
			return
		}
		RespondError(w, r, apierrors.Internal("failed to create model endpoint"))
		return
	}

	h.auditLog(r, "model_endpoint_create", "model_endpoint", ep.Slug)
	h.dispatchEvent(r, "model_endpoint.created", "model_endpoint", ep.Slug)

	RespondJSON(w, r, http.StatusCreated, ep)
}

// validateModelEndpointConfig validates the config JSONB for a model endpoint version.
func validateModelEndpointConfig(config json.RawMessage) error {
	if len(config) > maxConfigSize {
		return apierrors.Validation("config must be at most 64KB")
	}

	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(config, &parsed); err != nil {
		return apierrors.Validation("config must be a valid JSON object")
	}

	// Validate temperature if present
	if raw, ok := parsed["temperature"]; ok {
		var temp float64
		if err := json.Unmarshal(raw, &temp); err != nil {
			return apierrors.Validation("config.temperature must be a number")
		}
		if temp < 0 || temp > 2 {
			return apierrors.Validation("config.temperature must be between 0 and 2")
		}
	}

	// Validate max_tokens if present
	if raw, ok := parsed["max_tokens"]; ok {
		var maxTokens float64
		if err := json.Unmarshal(raw, &maxTokens); err != nil {
			return apierrors.Validation("config.max_tokens must be a number")
		}
		if maxTokens <= 0 {
			return apierrors.Validation("config.max_tokens must be greater than 0")
		}
	}

	// Validate max_output_tokens if present
	if raw, ok := parsed["max_output_tokens"]; ok {
		var v float64
		if err := json.Unmarshal(raw, &v); err != nil {
			return apierrors.Validation("config.max_output_tokens must be a number")
		}
		if v <= 0 {
			return apierrors.Validation("config.max_output_tokens must be greater than 0")
		}
	}

	// Validate top_p if present
	if raw, ok := parsed["top_p"]; ok {
		var v float64
		if err := json.Unmarshal(raw, &v); err != nil {
			return apierrors.Validation("config.top_p must be a number")
		}
		if v < 0 || v > 1 {
			return apierrors.Validation("config.top_p must be between 0 and 1")
		}
	}

	// Validate context_window if present
	if raw, ok := parsed["context_window"]; ok {
		var v float64
		if err := json.Unmarshal(raw, &v); err != nil {
			return apierrors.Validation("config.context_window must be a number")
		}
		if v <= 0 {
			return apierrors.Validation("config.context_window must be greater than 0")
		}
	}

	return nil
}

// redactConfigHeaders replaces header values with "***REDACTED***" in the config JSON.
func redactConfigHeaders(config json.RawMessage) json.RawMessage {
	var parsed map[string]interface{}
	if err := json.Unmarshal(config, &parsed); err != nil {
		return config
	}

	if headers, ok := parsed["headers"]; ok {
		if headerMap, ok := headers.(map[string]interface{}); ok {
			for k := range headerMap {
				headerMap[k] = "***REDACTED***"
			}
			parsed["headers"] = headerMap
		}
	}

	result, err := json.Marshal(parsed)
	if err != nil {
		return config
	}
	return result
}

func (h *ModelEndpointsHandler) auditLog(r *http.Request, action, resourceType, resourceID string) {
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

func (h *ModelEndpointsHandler) dispatchEvent(r *http.Request, eventType, resourceType, resourceID string) {
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
