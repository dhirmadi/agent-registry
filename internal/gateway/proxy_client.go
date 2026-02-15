package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"
)

// ProxyRequest contains all data needed to forward a tool call to an upstream MCP server.
type ProxyRequest struct {
	ServerEndpoint string          // MCP server endpoint URL
	ToolName       string          // Tool to call
	Arguments      json.RawMessage // Tool arguments as JSON
	AuthType       string          // "none", "bearer", "basic"
	AuthCredential string          // Decrypted credential (plaintext)
}

// ProxyResponse contains the upstream response and metadata.
type ProxyResponse struct {
	StatusCode   int             // HTTP status from upstream
	Body         json.RawMessage // Response body as raw bytes
	Latency      time.Duration   // Request duration
	RequestSize  int64           // Bytes sent
	ResponseSize int64           // Bytes received
}

// ProxyClientConfig configures the HTTP client for proxying.
type ProxyClientConfig struct {
	Timeout             time.Duration
	MaxIdleConnsPerHost int
}

// ProxyClient forwards tool calls to upstream MCP servers.
type ProxyClient struct {
	client *http.Client
}

// NewProxyClient creates a configured HTTP client for MCP proxying.
func NewProxyClient(cfg ProxyClientConfig) *ProxyClient {
	transport := &http.Transport{
		MaxIdleConnsPerHost: cfg.MaxIdleConnsPerHost,
	}

	return &ProxyClient{
		client: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: transport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse // Do not follow redirects
			},
		},
	}
}

// Forward sends a tool call to the upstream MCP server using JSON-RPC 2.0.
func (pc *ProxyClient) Forward(ctx context.Context, req ProxyRequest) (*ProxyResponse, error) {
	start := time.Now()

	// Build JSON-RPC 2.0 request
	rpcReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"id":      rand.Int63(),
		"params": map[string]interface{}{
			"name":      req.ToolName,
			"arguments": req.Arguments,
		},
	}

	body, err := json.Marshal(rpcReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, req.ServerEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Inject authentication
	switch req.AuthType {
	case "bearer":
		httpReq.Header.Set("Authorization", "Bearer "+req.AuthCredential)
	case "basic":
		httpReq.Header.Set("Authorization", "Basic "+req.AuthCredential)
	case "none":
		// No auth header
	default:
		return nil, fmt.Errorf("unsupported auth type: %s", req.AuthType)
	}

	resp, err := pc.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	latency := time.Since(start)

	return &ProxyResponse{
		StatusCode:   resp.StatusCode,
		Body:         respBody,
		Latency:      latency,
		RequestSize:  int64(len(body)),
		ResponseSize: int64(len(respBody)),
	}, nil
}
