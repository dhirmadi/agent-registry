package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/auth"
	apierrors "github.com/agent-smit/agentic-registry/internal/errors"
	"github.com/agent-smit/agentic-registry/internal/notify"
	"github.com/agent-smit/agentic-registry/internal/store"
)

// MCPServerStoreForAPI is the interface the MCP servers handler needs from the store.
type MCPServerStoreForAPI interface {
	Create(ctx context.Context, server *store.MCPServer) error
	GetByID(ctx context.Context, id uuid.UUID) (*store.MCPServer, error)
	GetByLabel(ctx context.Context, label string) (*store.MCPServer, error)
	List(ctx context.Context) ([]store.MCPServer, error)
	Update(ctx context.Context, server *store.MCPServer) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// MCPServersHandler provides HTTP handlers for MCP server endpoints.
type MCPServersHandler struct {
	servers    MCPServerStoreForAPI
	audit      AuditStoreForAPI
	encKey     []byte
	dispatcher notify.EventDispatcher
}

// NewMCPServersHandler creates a new MCPServersHandler.
func NewMCPServersHandler(servers MCPServerStoreForAPI, audit AuditStoreForAPI, encKey []byte, dispatcher notify.EventDispatcher) *MCPServersHandler {
	return &MCPServersHandler{
		servers:    servers,
		audit:      audit,
		encKey:     encKey,
		dispatcher: dispatcher,
	}
}

var validAuthTypes = map[string]bool{
	"none":   true,
	"bearer": true,
	"basic":  true,
}

// validateEndpointURL checks that an endpoint is a valid HTTP(S) URL pointing to a public host.
func validateEndpointURL(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return apierrors.Validation("endpoint is not a valid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return apierrors.Validation("endpoint must use http or https scheme")
	}
	host := u.Hostname()
	if host == "" {
		return apierrors.Validation("endpoint must have a valid host")
	}
	if isPrivateHost(host) {
		return apierrors.Validation("endpoint must not point to a private or internal address")
	}
	return nil
}

// isPrivateHost returns true if the host is a private/internal/loopback address.
func isPrivateHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	privateRanges := []struct {
		network string
	}{
		{"127.0.0.0/8"},
		{"10.0.0.0/8"},
		{"172.16.0.0/12"},
		{"192.168.0.0/16"},
		{"169.254.0.0/16"},
		{"::1/128"},
		{"fc00::/7"},
		{"fe80::/10"},
	}
	for _, r := range privateRanges {
		_, cidr, _ := net.ParseCIDR(r.network)
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// circuitBreakerSchema is used to validate the circuit_breaker JSON field.
type circuitBreakerSchema struct {
	FailThreshold float64 `json:"fail_threshold"`
	OpenDurationS float64 `json:"open_duration_s"`
}

// validateCircuitBreaker checks that circuit_breaker has valid schema and size.
func validateCircuitBreaker(raw json.RawMessage) error {
	if len(raw) > 1024 {
		return apierrors.Validation("circuit_breaker exceeds maximum size of 1KB")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return apierrors.Validation("circuit_breaker must be a valid JSON object")
	}
	// Only allow known fields
	for key := range fields {
		if key != "fail_threshold" && key != "open_duration_s" {
			return apierrors.Validation("circuit_breaker contains unknown field: " + key)
		}
	}
	if _, ok := fields["fail_threshold"]; !ok {
		return apierrors.Validation("circuit_breaker must contain fail_threshold")
	}
	if _, ok := fields["open_duration_s"]; !ok {
		return apierrors.Validation("circuit_breaker must contain open_duration_s")
	}
	var cb circuitBreakerSchema
	if err := json.Unmarshal(raw, &cb); err != nil {
		return apierrors.Validation("circuit_breaker fields must be numeric")
	}
	if cb.FailThreshold < 1 || cb.FailThreshold > 100 || cb.FailThreshold != float64(int(cb.FailThreshold)) {
		return apierrors.Validation("circuit_breaker fail_threshold must be an integer between 1 and 100")
	}
	if cb.OpenDurationS < 1 || cb.OpenDurationS > 3600 || cb.OpenDurationS != float64(int(cb.OpenDurationS)) {
		return apierrors.Validation("circuit_breaker open_duration_s must be an integer between 1 and 3600")
	}
	return nil
}

// validateDiscoveryInterval checks that the discovery interval is a valid Go duration within range.
func validateDiscoveryInterval(interval string) error {
	if interval == "" {
		return apierrors.Validation("discovery_interval is required")
	}
	d, err := time.ParseDuration(interval)
	if err != nil {
		return apierrors.Validation("discovery_interval must be a valid duration (e.g., 5m, 30s)")
	}
	if d < 30*time.Second {
		return apierrors.Validation("discovery_interval must be at least 30s")
	}
	if d > 24*time.Hour {
		return apierrors.Validation("discovery_interval must not exceed 24h")
	}
	return nil
}

// mcpServerResponse is the JSON representation of an MCP server, never exposing auth_credential.
type mcpServerResponse struct {
	ID                uuid.UUID       `json:"id"`
	Label             string          `json:"label"`
	Endpoint          string          `json:"endpoint"`
	AuthType          string          `json:"auth_type"`
	HealthEndpoint    string          `json:"health_endpoint"`
	CircuitBreaker    json.RawMessage `json:"circuit_breaker"`
	DiscoveryInterval string          `json:"discovery_interval"`
	IsEnabled         bool            `json:"is_enabled"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

func toMCPServerResponse(s *store.MCPServer) mcpServerResponse {
	return mcpServerResponse{
		ID:                s.ID,
		Label:             s.Label,
		Endpoint:          s.Endpoint,
		AuthType:          s.AuthType,
		HealthEndpoint:    s.HealthEndpoint,
		CircuitBreaker:    s.CircuitBreaker,
		DiscoveryInterval: s.DiscoveryInterval,
		IsEnabled:         s.IsEnabled,
		CreatedAt:         s.CreatedAt,
		UpdatedAt:         s.UpdatedAt,
	}
}

type createMCPServerRequest struct {
	Label             string          `json:"label"`
	Endpoint          string          `json:"endpoint"`
	AuthType          string          `json:"auth_type"`
	AuthCredential    string          `json:"auth_credential"`
	HealthEndpoint    string          `json:"health_endpoint"`
	CircuitBreaker    json.RawMessage `json:"circuit_breaker"`
	DiscoveryInterval *string         `json:"discovery_interval"`
	IsEnabled         *bool           `json:"is_enabled"`
}

// Create handles POST /api/v1/mcp-servers.
func (h *MCPServersHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createMCPServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	if req.Label == "" {
		RespondError(w, r, apierrors.Validation("label is required"))
		return
	}
	if req.Endpoint == "" {
		RespondError(w, r, apierrors.Validation("endpoint is required"))
		return
	}
	if err := validateEndpointURL(req.Endpoint); err != nil {
		RespondError(w, r, err.(*apierrors.APIError))
		return
	}
	if req.HealthEndpoint != "" {
		if err := validateEndpointURL(req.HealthEndpoint); err != nil {
			RespondError(w, r, apierrors.Validation("health_endpoint: "+err.Error()))
			return
		}
	}
	if req.AuthType == "" {
		req.AuthType = "none"
	}
	if !validAuthTypes[req.AuthType] {
		RespondError(w, r, apierrors.Validation("auth_type must be one of: none, bearer, basic"))
		return
	}
	if req.CircuitBreaker != nil {
		if err := validateCircuitBreaker(req.CircuitBreaker); err != nil {
			RespondError(w, r, err.(*apierrors.APIError))
			return
		}
	}

	// Encrypt credential if provided
	encryptedCred := ""
	if req.AuthCredential != "" && len(h.encKey) == 32 {
		encrypted, err := auth.Encrypt([]byte(req.AuthCredential), h.encKey)
		if err != nil {
			RespondError(w, r, apierrors.Internal("failed to encrypt credential"))
			return
		}
		encryptedCred = base64.StdEncoding.EncodeToString(encrypted)
	}

	isEnabled := true
	if req.IsEnabled != nil {
		isEnabled = *req.IsEnabled
	}
	// Validate discovery_interval: nil = use default "5m", non-nil = validate.
	discoveryInterval := "5m"
	if req.DiscoveryInterval != nil {
		if err := validateDiscoveryInterval(*req.DiscoveryInterval); err != nil {
			RespondError(w, r, err.(*apierrors.APIError))
			return
		}
		discoveryInterval = *req.DiscoveryInterval
	}

	server := &store.MCPServer{
		Label:             req.Label,
		Endpoint:          req.Endpoint,
		AuthType:          req.AuthType,
		AuthCredential:    encryptedCred,
		HealthEndpoint:    req.HealthEndpoint,
		CircuitBreaker:    req.CircuitBreaker,
		DiscoveryInterval: discoveryInterval,
		IsEnabled:         isEnabled,
	}

	if err := h.servers.Create(r.Context(), server); err != nil {
		if strings.Contains(err.Error(), "duplicate") {
			RespondError(w, r, apierrors.Conflict("mcp server with this label already exists"))
			return
		}
		RespondError(w, r, apierrors.Internal("failed to create mcp server"))
		return
	}

	h.auditLog(r, "mcp_server_create", "mcp_server", server.ID.String())
	h.dispatchEvent(r, "mcp_server.created", "mcp_server", server.ID.String())

	RespondJSON(w, r, http.StatusCreated, toMCPServerResponse(server))
}

// List handles GET /api/v1/mcp-servers.
func (h *MCPServersHandler) List(w http.ResponseWriter, r *http.Request) {
	servers, err := h.servers.List(r.Context())
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to list mcp servers"))
		return
	}

	resp := make([]mcpServerResponse, 0, len(servers))
	for i := range servers {
		resp = append(resp, toMCPServerResponse(&servers[i]))
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"servers": resp,
		"total":   len(resp),
	})
}

// Get handles GET /api/v1/mcp-servers/{serverId}.
func (h *MCPServersHandler) Get(w http.ResponseWriter, r *http.Request) {
	serverID, err := uuid.Parse(chi.URLParam(r, "serverId"))
	if err != nil {
		RespondError(w, r, apierrors.Validation("invalid server ID"))
		return
	}

	server, err := h.servers.GetByID(r.Context(), serverID)
	if err != nil {
		RespondError(w, r, apierrors.NotFound("mcp_server", serverID.String()))
		return
	}

	RespondJSON(w, r, http.StatusOK, toMCPServerResponse(server))
}

type updateMCPServerRequest struct {
	Label             *string          `json:"label"`
	Endpoint          *string          `json:"endpoint"`
	AuthType          *string          `json:"auth_type"`
	AuthCredential    *string          `json:"auth_credential"`
	HealthEndpoint    *string          `json:"health_endpoint"`
	CircuitBreaker    *json.RawMessage `json:"circuit_breaker"`
	DiscoveryInterval *string          `json:"discovery_interval"`
	IsEnabled         *bool            `json:"is_enabled"`
}

// Update handles PUT /api/v1/mcp-servers/{serverId}.
func (h *MCPServersHandler) Update(w http.ResponseWriter, r *http.Request) {
	serverID, err := uuid.Parse(chi.URLParam(r, "serverId"))
	if err != nil {
		RespondError(w, r, apierrors.Validation("invalid server ID"))
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

	server, err := h.servers.GetByID(r.Context(), serverID)
	if err != nil {
		RespondError(w, r, apierrors.NotFound("mcp_server", serverID.String()))
		return
	}

	var req updateMCPServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	if req.Label != nil {
		server.Label = *req.Label
	}
	if req.Endpoint != nil {
		if err := validateEndpointURL(*req.Endpoint); err != nil {
			RespondError(w, r, err.(*apierrors.APIError))
			return
		}
		server.Endpoint = *req.Endpoint
	}
	if req.AuthType != nil {
		if !validAuthTypes[*req.AuthType] {
			RespondError(w, r, apierrors.Validation("auth_type must be one of: none, bearer, basic"))
			return
		}
		server.AuthType = *req.AuthType
	}
	if req.AuthCredential != nil && len(h.encKey) == 32 {
		encrypted, err := auth.Encrypt([]byte(*req.AuthCredential), h.encKey)
		if err != nil {
			RespondError(w, r, apierrors.Internal("failed to encrypt credential"))
			return
		}
		server.AuthCredential = base64.StdEncoding.EncodeToString(encrypted)
	}
	if req.HealthEndpoint != nil {
		if *req.HealthEndpoint != "" {
			if err := validateEndpointURL(*req.HealthEndpoint); err != nil {
				RespondError(w, r, apierrors.Validation("health_endpoint: "+err.Error()))
				return
			}
		}
		server.HealthEndpoint = *req.HealthEndpoint
	}
	if req.CircuitBreaker != nil {
		if err := validateCircuitBreaker(*req.CircuitBreaker); err != nil {
			RespondError(w, r, err.(*apierrors.APIError))
			return
		}
		server.CircuitBreaker = *req.CircuitBreaker
	}
	if req.DiscoveryInterval != nil {
		if err := validateDiscoveryInterval(*req.DiscoveryInterval); err != nil {
			RespondError(w, r, err.(*apierrors.APIError))
			return
		}
		server.DiscoveryInterval = *req.DiscoveryInterval
	}
	if req.IsEnabled != nil {
		server.IsEnabled = *req.IsEnabled
	}

	server.UpdatedAt = etag

	if err := h.servers.Update(r.Context(), server); err != nil {
		if strings.Contains(err.Error(), "conflict") || strings.Contains(err.Error(), "modified") {
			RespondError(w, r, apierrors.Conflict("mcp server was modified by another request"))
			return
		}
		RespondError(w, r, apierrors.Internal("failed to update mcp server"))
		return
	}

	h.auditLog(r, "mcp_server_update", "mcp_server", server.ID.String())
	h.dispatchEvent(r, "mcp_server.updated", "mcp_server", server.ID.String())

	RespondJSON(w, r, http.StatusOK, toMCPServerResponse(server))
}

// Delete handles DELETE /api/v1/mcp-servers/{serverId}.
func (h *MCPServersHandler) Delete(w http.ResponseWriter, r *http.Request) {
	serverID, err := uuid.Parse(chi.URLParam(r, "serverId"))
	if err != nil {
		RespondError(w, r, apierrors.Validation("invalid server ID"))
		return
	}

	if err := h.servers.Delete(r.Context(), serverID); err != nil {
		RespondError(w, r, apierrors.NotFound("mcp_server", serverID.String()))
		return
	}

	h.auditLog(r, "mcp_server_delete", "mcp_server", serverID.String())
	h.dispatchEvent(r, "mcp_server.deleted", "mcp_server", serverID.String())

	RespondNoContent(w)
}

func (h *MCPServersHandler) auditLog(r *http.Request, action, resourceType, resourceID string) {
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

func (h *MCPServersHandler) dispatchEvent(r *http.Request, eventType, resourceType, resourceID string) {
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
