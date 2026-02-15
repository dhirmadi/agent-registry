package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"time"
)

const (
	// MaxUpstreamResponseSize is the maximum size of an upstream MCP server response body.
	// Prevents memory exhaustion from malicious or buggy upstream servers.
	MaxUpstreamResponseSize = 10 << 20 // 10 MB
)

// privateIPRanges contains CIDR blocks for private/internal IP ranges.
// These are blocked to prevent SSRF attacks via DNS rebinding.
var privateIPRanges = []string{
	"10.0.0.0/8",       // RFC 1918
	"172.16.0.0/12",    // RFC 1918
	"192.168.0.0/16",   // RFC 1918
	"127.0.0.0/8",      // Loopback
	"169.254.0.0/16",   // Link-local (AWS metadata)
	"::1/128",          // IPv6 loopback
	"fc00::/7",         // IPv6 private
	"fe80::/10",        // IPv6 link-local
}

var privateCIDRs []*net.IPNet

func init() {
	for _, cidr := range privateIPRanges {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Sprintf("invalid CIDR %s: %v", cidr, err))
		}
		privateCIDRs = append(privateCIDRs, ipNet)
	}
}

// isPrivateIP checks if an IP address is in a private/internal range.
func isPrivateIP(ip net.IP) bool {
	for _, cidr := range privateCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

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
	AllowPrivateIPs     bool // If true, disables SSRF protection (for testing only)
}

// ProxyClient forwards tool calls to upstream MCP servers.
type ProxyClient struct {
	client *http.Client
}

// NewProxyClient creates a configured HTTP client for MCP proxying.
// The client includes SSRF protection via a custom dialer that blocks private IPs,
// unless AllowPrivateIPs is set to true (for testing only).
func NewProxyClient(cfg ProxyClientConfig) *ProxyClient {
	// Create a custom dialer that validates resolved IPs to prevent SSRF attacks
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		MaxIdleConnsPerHost: cfg.MaxIdleConnsPerHost,
	}

	// Add SSRF protection unless explicitly disabled (for testing)
	if !cfg.AllowPrivateIPs {
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			// Resolve the address to get actual IPs
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid address: %w", err)
			}

			// Look up all IPs for this host
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("DNS lookup failed: %w", err)
			}

			// Check if any resolved IP is private
			for _, ipAddr := range ips {
				if isPrivateIP(ipAddr.IP) {
					return nil, fmt.Errorf("SSRF protection: resolved IP %s is in a private range", ipAddr.IP)
				}
			}

			// All IPs are public, proceed with connection
			return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
		}
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

	// Limit response body size to prevent memory exhaustion from malicious upstream
	limitedBody := io.LimitReader(resp.Body, MaxUpstreamResponseSize)
	respBody, err := io.ReadAll(limitedBody)
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
