package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/auth"
	apierrors "github.com/agent-smit/agentic-registry/internal/errors"
	"github.com/agent-smit/agentic-registry/internal/gateway"
	"github.com/agent-smit/agentic-registry/internal/ratelimit"
	"github.com/agent-smit/agentic-registry/internal/store"
)

type MCPGatewayServerStore interface {
	GetByLabel(ctx context.Context, label string) (*store.MCPServer, error)
	List(ctx context.Context) ([]store.MCPServer, error)
}

type Forwarder interface {
	Forward(ctx context.Context, req gateway.ProxyRequest) (*gateway.ProxyResponse, error)
}

type MCPGatewayHandler struct {
	servers         MCPGatewayServerStore
	audit           AuditStoreForAPI
	trustClassifier *gateway.TrustClassifier
	circuitBreaker  *gateway.CircuitBreaker
	forwarder       Forwarder
	rateLimiter     *ratelimit.RateLimiter
	encKey          []byte
}

func NewMCPGatewayHandler(
	servers MCPGatewayServerStore,
	audit AuditStoreForAPI,
	trustClassifier *gateway.TrustClassifier,
	circuitBreaker *gateway.CircuitBreaker,
	forwarder Forwarder,
	rateLimiter *ratelimit.RateLimiter,
	encKey []byte,
) *MCPGatewayHandler {
	return &MCPGatewayHandler{
		servers: servers, audit: audit, trustClassifier: trustClassifier,
		circuitBreaker: circuitBreaker, forwarder: forwarder,
		rateLimiter: rateLimiter, encKey: encKey,
	}
}

func (h *MCPGatewayHandler) ProxyToolCall(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	serverLabel := chi.URLParam(r, "serverLabel")
	toolName := chi.URLParam(r, "toolName")
	if serverLabel == "" || toolName == "" {
		RespondError(w, r, apierrors.Validation("serverLabel and toolName are required"))
		return
	}
	server, err := h.servers.GetByLabel(ctx, serverLabel)
	if err != nil {
		RespondError(w, r, apierrors.NotFound("mcp_server", serverLabel))
		return
	}
	if !server.IsEnabled {
		RespondError(w, r, apierrors.Validation("MCP server is disabled"))
		return
	}
	cbConfig, err := parseCircuitBreakerCfg(server.CircuitBreaker)
	if err != nil {
		RespondError(w, r, apierrors.Internal("invalid circuit breaker config"))
		return
	}
	if !h.circuitBreaker.Allow(serverLabel, cbConfig) {
		h.auditGatewayCall(r, serverLabel, toolName, 0, "circuit_open", 0)
		RespondError(w, r, apierrors.ServiceUnavailable("circuit breaker open for "+serverLabel))
		return
	}
	var reqBody struct {
		Arguments   json.RawMessage `json:"arguments"`
		WorkspaceID *string         `json:"workspace_id,omitempty"`
		AgentID     string          `json:"agent_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}
	classifyInput := gateway.ClassifyInput{ToolName: toolName, AgentID: reqBody.AgentID}
	if reqBody.WorkspaceID != nil {
		wid, err := uuid.Parse(*reqBody.WorkspaceID)
		if err == nil {
			classifyInput.WorkspaceID = &wid
		}
	}
	tier, err := h.trustClassifier.Classify(ctx, classifyInput)
	if err != nil {
		RespondError(w, r, apierrors.Internal("trust classification failed"))
		return
	}
	if tier == gateway.TrustBlock || tier == gateway.TrustReview {
		h.auditGatewayCall(r, serverLabel, toolName, 0, "trust_denied", 0)
		RespondError(w, r, apierrors.Forbidden("tool blocked by trust policy"))
		return
	}
	userID, _ := auth.UserIDFromContext(ctx)
	rateLimitKey := "gateway:" + serverLabel + ":" + toolName + ":" + userID.String()
	if allowed, _, _ := h.rateLimiter.Allow(rateLimitKey, 60, time.Minute); !allowed {
		h.auditGatewayCall(r, serverLabel, toolName, 0, "rate_limited", 0)
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(Envelope{
			Success: false,
			Error:   map[string]string{"code": "RATE_LIMITED", "message": "rate limit exceeded"},
			Meta:    newMeta(r),
		})
		return
	}
	var plainCredential string
	if server.AuthType != "none" && server.AuthCredential != "" {
		ciphertext, err := base64.StdEncoding.DecodeString(server.AuthCredential)
		if err != nil {
			RespondError(w, r, apierrors.Internal("credential decode failed"))
			return
		}
		plaintext, err := auth.Decrypt(ciphertext, h.encKey)
		if err != nil {
			RespondError(w, r, apierrors.Internal("credential decrypt failed"))
			return
		}
		plainCredential = string(plaintext)
	}
	proxyReq := gateway.ProxyRequest{
		ServerEndpoint: server.Endpoint, ToolName: toolName,
		Arguments: reqBody.Arguments, AuthType: server.AuthType,
		AuthCredential: plainCredential,
	}
	start := time.Now()
	proxyResp, err := h.forwarder.Forward(ctx, proxyReq)
	latency := time.Since(start)
	if err != nil {
		h.circuitBreaker.RecordFailure(serverLabel, cbConfig)
		h.auditGatewayCall(r, serverLabel, toolName, 0, "upstream_error", latency)
		RespondError(w, r, apierrors.BadGateway("upstream request failed"))
		return
	}
	if proxyResp.StatusCode >= 500 {
		h.circuitBreaker.RecordFailure(serverLabel, cbConfig)
		h.auditGatewayCall(r, serverLabel, toolName, proxyResp.StatusCode, "upstream_5xx", latency)
	} else {
		h.circuitBreaker.RecordSuccess(serverLabel)
		h.auditGatewayCall(r, serverLabel, toolName, proxyResp.StatusCode, "success", latency)
	}
	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"status_code": proxyResp.StatusCode,
		"body":        proxyResp.Body,
		"latency_ms":  proxyResp.Latency.Milliseconds(),
	})
}

func (h *MCPGatewayHandler) ListTools(w http.ResponseWriter, r *http.Request) {
	servers, err := h.servers.List(r.Context())
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to list servers"))
		return
	}
	var enabled []map[string]interface{}
	for _, s := range servers {
		if s.IsEnabled {
			enabled = append(enabled, map[string]interface{}{"label": s.Label, "endpoint": s.Endpoint})
		}
	}
	RespondJSON(w, r, http.StatusOK, map[string]interface{}{"servers": enabled})
}

func parseCircuitBreakerCfg(cbJSON json.RawMessage) (gateway.CircuitBreakerConfig, error) {
	if len(cbJSON) == 0 {
		return gateway.CircuitBreakerConfig{FailThreshold: 5, OpenDuration: 30 * time.Second}, nil
	}
	var cb struct {
		FailThreshold int `json:"fail_threshold"`
		OpenDurationS int `json:"open_duration_s"`
	}
	if err := json.Unmarshal(cbJSON, &cb); err != nil {
		return gateway.CircuitBreakerConfig{}, err
	}
	if cb.FailThreshold <= 0 {
		cb.FailThreshold = 5
	}
	if cb.OpenDurationS <= 0 {
		cb.OpenDurationS = 30
	}
	return gateway.CircuitBreakerConfig{
		FailThreshold: cb.FailThreshold,
		OpenDuration:  time.Duration(cb.OpenDurationS) * time.Second,
	}, nil
}

func (h *MCPGatewayHandler) auditGatewayCall(r *http.Request, serverLabel, toolName string, upstreamStatus int, outcome string, latency time.Duration) {
	if h.audit == nil {
		return
	}
	callerID, _ := auth.UserIDFromContext(r.Context())
	details := map[string]interface{}{
		"server_label": serverLabel, "tool_name": toolName,
		"outcome": outcome, "latency_ms": latency.Milliseconds(),
	}
	if upstreamStatus > 0 {
		details["upstream_status"] = upstreamStatus
	}
	detailsJSON, _ := json.Marshal(details)
	entry := &store.AuditEntry{
		Actor: callerID.String(), ActorID: &callerID,
		Action: "gateway_tool_call", ResourceType: "mcp_tool",
		ResourceID: serverLabel + "/" + toolName,
		Details: detailsJSON, IPAddress: clientIPFromRequest(r),
	}
	go func() {
		if err := h.audit.Insert(context.Background(), entry); err != nil {
			log.Printf("gateway audit log failed: %v", err)
		}
	}()
}
