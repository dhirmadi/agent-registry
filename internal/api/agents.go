package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/agent-smit/agentic-registry/internal/auth"
	apierrors "github.com/agent-smit/agentic-registry/internal/errors"
	"github.com/agent-smit/agentic-registry/internal/notify"
	"github.com/agent-smit/agentic-registry/internal/store"
)

var agentIDRegex = regexp.MustCompile(`^[a-z][a-z0-9_]{1,49}$`)

// AgentStoreForAPI is the interface the agents handler needs from the store.
type AgentStoreForAPI interface {
	Create(ctx context.Context, agent *store.Agent) error
	GetByID(ctx context.Context, id string) (*store.Agent, error)
	List(ctx context.Context, activeOnly bool, offset, limit int) ([]store.Agent, int, error)
	Update(ctx context.Context, agent *store.Agent, updatedAt time.Time) error
	Patch(ctx context.Context, id string, fields map[string]interface{}, updatedAt time.Time, actor string) (*store.Agent, error)
	Delete(ctx context.Context, id string) error
	ListVersions(ctx context.Context, agentID string, offset, limit int) ([]store.AgentVersion, int, error)
	GetVersion(ctx context.Context, agentID string, version int) (*store.AgentVersion, error)
	Rollback(ctx context.Context, agentID string, targetVersion int, actor string) (*store.Agent, error)
}

// AgentsHandler provides HTTP handlers for agent management endpoints.
type AgentsHandler struct {
	agents       AgentStoreForAPI
	audit        AuditStoreForAPI
	dispatcher   notify.EventDispatcher
	a2aPublisher *notify.A2APublisher
}

// NewAgentsHandler creates a new AgentsHandler.
func NewAgentsHandler(agents AgentStoreForAPI, audit AuditStoreForAPI, dispatcher notify.EventDispatcher) *AgentsHandler {
	return &AgentsHandler{
		agents:     agents,
		audit:      audit,
		dispatcher: dispatcher,
	}
}

// SetA2APublisher configures the A2A registry publisher for this handler.
func (h *AgentsHandler) SetA2APublisher(pub *notify.A2APublisher) {
	h.a2aPublisher = pub
}

// agentTool is used to parse the tools JSONB for computing derived fields.
type agentTool struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	ServerLabel string `json:"server_label"`
	Description string `json:"description"`
}

// agentAPIResponse is the JSON representation of an agent with derived fields.
type agentAPIResponse struct {
	ID                  string          `json:"id"`
	Name                string          `json:"name"`
	Description         string          `json:"description"`
	SystemPrompt        string          `json:"system_prompt"`
	Tools               json.RawMessage `json:"tools,omitempty"`
	TrustOverrides      json.RawMessage `json:"trust_overrides"`
	Capabilities        []string        `json:"capabilities"`
	ExamplePrompts      json.RawMessage `json:"example_prompts"`
	RequiredConnections []string        `json:"required_connections"`
	IsActive            bool            `json:"is_active"`
	Version             int             `json:"version"`
	CreatedBy           string          `json:"created_by"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

func toAgentAPIResponse(a *store.Agent, includeTools bool) agentAPIResponse {
	capabilities, requiredConnections := computeDerivedFields(a.Tools)
	resp := agentAPIResponse{
		ID:                  a.ID,
		Name:                a.Name,
		Description:         a.Description,
		SystemPrompt:        a.SystemPrompt,
		TrustOverrides:      a.TrustOverrides,
		Capabilities:        capabilities,
		ExamplePrompts:      a.ExamplePrompts,
		RequiredConnections: requiredConnections,
		IsActive:            a.IsActive,
		Version:             a.Version,
		CreatedBy:           a.CreatedBy,
		CreatedAt:           a.CreatedAt,
		UpdatedAt:           a.UpdatedAt,
	}
	if includeTools {
		resp.Tools = a.Tools
	}
	return resp
}

func computeDerivedFields(toolsJSON json.RawMessage) (capabilities []string, requiredConnections []string) {
	capabilities = []string{}
	requiredConnections = []string{}

	if len(toolsJSON) == 0 {
		return
	}

	var tools []agentTool
	if err := json.Unmarshal(toolsJSON, &tools); err != nil {
		return
	}

	connSet := make(map[string]bool)
	for _, t := range tools {
		if t.Source == "internal" {
			capabilities = append(capabilities, t.Name)
		}
		if t.Source == "mcp" && t.ServerLabel != "" && t.ServerLabel != "mcp-git" {
			if !connSet[t.ServerLabel] {
				connSet[t.ServerLabel] = true
				requiredConnections = append(requiredConnections, t.ServerLabel)
			}
		}
	}
	return
}

var validToolSources = map[string]bool{
	"internal": true,
	"mcp":      true,
}

// validateAgentTools checks that all tools have valid source and required fields.
func validateAgentTools(toolsJSON json.RawMessage) error {
	if len(toolsJSON) == 0 {
		return nil
	}
	var tools []agentTool
	if err := json.Unmarshal(toolsJSON, &tools); err != nil {
		return apierrors.Validation("tools must be a valid JSON array of tool objects")
	}
	for _, t := range tools {
		if t.Name == "" {
			return apierrors.Validation("each tool must have a non-empty name")
		}
		if len(t.Name) > 100 {
			return apierrors.Validation("tool name must be at most 100 characters")
		}
		if !validToolSources[t.Source] {
			return apierrors.Validation("tool source must be one of: internal, mcp")
		}
		if t.Source == "mcp" && t.ServerLabel == "" {
			return apierrors.Validation("MCP tools must have a non-empty server_label")
		}
	}
	return nil
}

type createAgentRequest struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	SystemPrompt   string          `json:"system_prompt"`
	Tools          json.RawMessage `json:"tools"`
	TrustOverrides json.RawMessage `json:"trust_overrides"`
	ExamplePrompts json.RawMessage `json:"example_prompts"`
}

// Create handles POST /api/v1/agents.
func (h *AgentsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	if req.ID == "" {
		RespondError(w, r, apierrors.Validation("id is required"))
		return
	}
	if !agentIDRegex.MatchString(req.ID) {
		RespondError(w, r, apierrors.Validation("id must match pattern: lowercase letters, digits, underscores; 2-50 chars; must start with a letter"))
		return
	}
	if req.Name == "" {
		RespondError(w, r, apierrors.Validation("name is required"))
		return
	}
	if len(req.SystemPrompt) > 100*1024 {
		RespondError(w, r, apierrors.Validation("system_prompt must be at most 100KB"))
		return
	}

	// Set defaults for JSONB fields
	if req.Tools == nil {
		req.Tools = json.RawMessage(`[]`)
	}
	if req.TrustOverrides == nil {
		req.TrustOverrides = json.RawMessage(`{}`)
	}
	if req.ExamplePrompts == nil {
		req.ExamplePrompts = json.RawMessage(`[]`)
	}

	// Validate tool definitions
	if err := validateAgentTools(req.Tools); err != nil {
		RespondError(w, r, err.(*apierrors.APIError))
		return
	}

	userID, _ := auth.UserIDFromContext(r.Context())
	agent := &store.Agent{
		ID:             req.ID,
		Name:           req.Name,
		Description:    req.Description,
		SystemPrompt:   req.SystemPrompt,
		Tools:          req.Tools,
		TrustOverrides: req.TrustOverrides,
		ExamplePrompts: req.ExamplePrompts,
		IsActive:       true,
		CreatedBy:      userID.String(),
	}

	if err := h.agents.Create(r.Context(), agent); err != nil {
		if isConflictError(err) {
			RespondError(w, r, apierrors.Conflict("agent '"+req.ID+"' already exists"))
			return
		}
		RespondError(w, r, apierrors.Internal("failed to create agent"))
		return
	}

	h.auditLog(r, "agent_create", "agent", agent.ID)
	h.dispatchEvent(r, "agent.created", "agent", agent.ID)
	h.publishA2A(agent.ID, "upsert")

	RespondJSON(w, r, http.StatusCreated, toAgentAPIResponse(agent, true))
}

// Get handles GET /api/v1/agents/{agentId}.
func (h *AgentsHandler) Get(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")

	agent, err := h.agents.GetByID(r.Context(), agentID)
	if err != nil {
		if isNotFoundError(err) {
			RespondError(w, r, apierrors.NotFound("agent", agentID))
		} else {
			RespondError(w, r, apierrors.Internal("failed to retrieve agent"))
		}
		return
	}

	RespondJSON(w, r, http.StatusOK, toAgentAPIResponse(agent, true))
}

// List handles GET /api/v1/agents.
func (h *AgentsHandler) List(w http.ResponseWriter, r *http.Request) {
	activeOnly := true
	if r.URL.Query().Get("active_only") == "false" {
		activeOnly = false
	}

	includeTools := true
	if r.URL.Query().Get("include_tools") == "false" {
		includeTools = false
	}

	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	agents, total, err := h.agents.List(r.Context(), activeOnly, offset, limit)
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to list agents"))
		return
	}

	resp := make([]agentAPIResponse, 0, len(agents))
	for i := range agents {
		resp = append(resp, toAgentAPIResponse(&agents[i], includeTools))
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"agents": resp,
		"total":  total,
	})
}

type updateAgentRequest struct {
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	SystemPrompt   string          `json:"system_prompt"`
	Tools          json.RawMessage `json:"tools"`
	TrustOverrides json.RawMessage `json:"trust_overrides"`
	ExamplePrompts json.RawMessage `json:"example_prompts"`
	IsActive       *bool           `json:"is_active"`
}

// Update handles PUT /api/v1/agents/{agentId}.
func (h *AgentsHandler) Update(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")

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

	existing, err := h.agents.GetByID(r.Context(), agentID)
	if err != nil {
		RespondError(w, r, apierrors.NotFound("agent", agentID))
		return
	}

	var req updateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	if len(req.SystemPrompt) > 100*1024 {
		RespondError(w, r, apierrors.Validation("system_prompt must be at most 100KB"))
		return
	}

	// Apply all fields (PUT = full update)
	existing.Name = req.Name
	existing.Description = req.Description
	existing.SystemPrompt = req.SystemPrompt
	if req.Tools != nil {
		existing.Tools = req.Tools
	}
	if req.TrustOverrides != nil {
		existing.TrustOverrides = req.TrustOverrides
	}
	if req.ExamplePrompts != nil {
		existing.ExamplePrompts = req.ExamplePrompts
	}
	if req.IsActive != nil {
		existing.IsActive = *req.IsActive
	}

	if err := h.agents.Update(r.Context(), existing, etag); err != nil {
		if isConflictError(err) {
			RespondError(w, r, apierrors.Conflict("resource was modified by another client"))
			return
		}
		RespondError(w, r, apierrors.Internal("failed to update agent"))
		return
	}

	h.auditLog(r, "agent_update", "agent", agentID)
	h.dispatchEvent(r, "agent.updated", "agent", agentID)
	h.publishA2A(agentID, "upsert")

	RespondJSON(w, r, http.StatusOK, toAgentAPIResponse(existing, true))
}

// PatchAgent handles PATCH /api/v1/agents/{agentId}.
func (h *AgentsHandler) PatchAgent(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")

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

	var rawFields map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&rawFields); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	userID, _ := auth.UserIDFromContext(r.Context())

	agent, err := h.agents.Patch(r.Context(), agentID, rawFields, etag, userID.String())
	if err != nil {
		if isNotFoundError(err) {
			RespondError(w, r, apierrors.NotFound("agent", agentID))
			return
		}
		if isConflictError(err) {
			RespondError(w, r, apierrors.Conflict("resource was modified by another client"))
			return
		}
		RespondError(w, r, apierrors.Internal("failed to patch agent"))
		return
	}

	h.auditLog(r, "agent_update", "agent", agentID)
	h.dispatchEvent(r, "agent.updated", "agent", agentID)
	h.publishA2A(agentID, "upsert")

	RespondJSON(w, r, http.StatusOK, toAgentAPIResponse(agent, true))
}

// Delete handles DELETE /api/v1/agents/{agentId}.
func (h *AgentsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")

	if err := h.agents.Delete(r.Context(), agentID); err != nil {
		if isNotFoundError(err) {
			RespondError(w, r, apierrors.NotFound("agent", agentID))
			return
		}
		RespondError(w, r, apierrors.Internal("failed to delete agent"))
		return
	}

	h.auditLog(r, "agent_delete", "agent", agentID)
	h.dispatchEvent(r, "agent.deleted", "agent", agentID)
	h.publishA2A(agentID, "delete")

	RespondNoContent(w)
}

// ListVersions handles GET /api/v1/agents/{agentId}/versions.
func (h *AgentsHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")

	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	versions, total, err := h.agents.ListVersions(r.Context(), agentID, offset, limit)
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to list agent versions"))
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"versions": versions,
		"total":    total,
	})
}

// GetVersion handles GET /api/v1/agents/{agentId}/versions/{version}.
func (h *AgentsHandler) GetVersion(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	versionStr := chi.URLParam(r, "version")
	version, err := strconv.Atoi(versionStr)
	if err != nil {
		RespondError(w, r, apierrors.Validation("invalid version number"))
		return
	}

	v, err := h.agents.GetVersion(r.Context(), agentID, version)
	if err != nil {
		RespondError(w, r, apierrors.NotFound("agent_version", agentID+"/v"+versionStr))
		return
	}

	RespondJSON(w, r, http.StatusOK, v)
}

type rollbackRequest struct {
	TargetVersion *int `json:"target_version"`
}

// Rollback handles POST /api/v1/agents/{agentId}/rollback.
func (h *AgentsHandler) Rollback(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")

	var req rollbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	if req.TargetVersion == nil || *req.TargetVersion <= 0 {
		RespondError(w, r, apierrors.Validation("target_version is required and must be positive"))
		return
	}

	userID, _ := auth.UserIDFromContext(r.Context())

	agent, err := h.agents.Rollback(r.Context(), agentID, *req.TargetVersion, userID.String())
	if err != nil {
		if isNotFoundError(err) {
			RespondError(w, r, apierrors.NotFound("agent_version", agentID))
			return
		}
		RespondError(w, r, apierrors.Internal("failed to rollback agent"))
		return
	}

	h.auditLog(r, "agent_rollback", "agent", agentID)
	h.dispatchEvent(r, "agent.rolled_back", "agent", agentID)
	h.publishA2A(agentID, "upsert")

	RespondJSON(w, r, http.StatusOK, toAgentAPIResponse(agent, true))
}

func (h *AgentsHandler) auditLog(r *http.Request, action, resourceType, resourceID string) {
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

func (h *AgentsHandler) dispatchEvent(r *http.Request, eventType, resourceType, resourceID string) {
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

func (h *AgentsHandler) publishA2A(agentID, action string) {
	if h.a2aPublisher == nil {
		return
	}
	go func() {
		if err := h.a2aPublisher.Publish(context.Background(), agentID, action); err != nil {
			log.Printf("a2a publish failed for %s %s: %v", action, agentID, err)
		}
	}()
}

func isConflictError(err error) bool {
	if err == nil {
		return false
	}
	apiErr, ok := err.(*apierrors.APIError)
	return ok && apiErr.Code == "CONFLICT"
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	apiErr, ok := err.(*apierrors.APIError)
	return ok && apiErr.Code == "NOT_FOUND"
}
