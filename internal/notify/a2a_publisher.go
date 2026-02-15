package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	a2aMaxRetries     = 3
	a2aRequestTimeout = 10 * time.Second
)

// AgentCardProvider returns agent card data for publishing to an A2A registry.
type AgentCardProvider interface {
	GetAgentCard(ctx context.Context, agentID string) (map[string]interface{}, error)
}

// A2APublisher pushes agent cards to an external A2A registry.
type A2APublisher struct {
	registryURL string
	externalURL string
	agents      AgentCardProvider
	client      *http.Client
}

// NewA2APublisher creates a publisher that pushes agent cards to registryURL.
func NewA2APublisher(registryURL, externalURL string, agents AgentCardProvider) *A2APublisher {
	return &A2APublisher{
		registryURL: strings.TrimRight(registryURL, "/"),
		externalURL: externalURL,
		agents:      agents,
		client:      &http.Client{Timeout: a2aRequestTimeout},
	}
}

// Publish sends an agent card to the external A2A registry.
// action must be "upsert" or "delete".
func (p *A2APublisher) Publish(ctx context.Context, agentID, action string) error {
	if p == nil {
		return nil
	}

	switch action {
	case "upsert":
		return p.publishUpsert(ctx, agentID)
	case "delete":
		return p.publishDelete(ctx, agentID)
	default:
		return fmt.Errorf("a2a publisher: invalid action %q, must be upsert or delete", action)
	}
}

func (p *A2APublisher) publishUpsert(ctx context.Context, agentID string) error {
	card, err := p.agents.GetAgentCard(ctx, agentID)
	if err != nil {
		return fmt.Errorf("a2a publisher: failed to get agent card for %s: %w", agentID, err)
	}

	body, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("a2a publisher: failed to marshal agent card: %w", err)
	}

	targetURL := p.registryURL + "/agents/" + agentID
	return p.doWithRetry(ctx, http.MethodPut, targetURL, body)
}

func (p *A2APublisher) publishDelete(ctx context.Context, agentID string) error {
	targetURL := p.registryURL + "/agents/" + agentID
	return p.doWithRetry(ctx, http.MethodDelete, targetURL, nil)
}

func (p *A2APublisher) doWithRetry(ctx context.Context, method, targetURL string, body []byte) error {
	var lastErr error

	for attempt := 0; attempt <= a2aMaxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		var req *http.Request
		var err error
		if body != nil {
			req, err = http.NewRequestWithContext(ctx, method, targetURL, bytes.NewReader(body))
		} else {
			req, err = http.NewRequestWithContext(ctx, method, targetURL, nil)
		}
		if err != nil {
			return fmt.Errorf("a2a publisher: failed to create request: %w", err)
		}

		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := p.client.Do(req)
		if err != nil {
			lastErr = err
			log.Printf("a2a publisher: attempt %d/%d failed for %s %s: %v",
				attempt+1, a2aMaxRetries+1, method, targetURL, err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}

		lastErr = fmt.Errorf("unexpected status %d", resp.StatusCode)
		log.Printf("a2a publisher: attempt %d/%d failed for %s %s: status %d",
			attempt+1, a2aMaxRetries+1, method, targetURL, resp.StatusCode)
	}

	return fmt.Errorf("a2a publisher: exhausted retries for %s %s: %w", method, targetURL, lastErr)
}

// ValidateA2ARegistryURL validates that the registry URL is a valid public HTTP(S) URL.
func ValidateA2ARegistryURL(registryURL string) (string, error) {
	if len(registryURL) > 2000 {
		return "", fmt.Errorf("A2A_REGISTRY_URL must be at most 2000 characters")
	}

	u, err := url.Parse(registryURL)
	if err != nil {
		return "", fmt.Errorf("A2A_REGISTRY_URL is not a valid URL: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("A2A_REGISTRY_URL must use http or https scheme")
	}

	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("A2A_REGISTRY_URL must have a valid host")
	}

	if isA2APrivateHost(host) {
		return "", fmt.Errorf("A2A_REGISTRY_URL must not point to a private or internal address")
	}

	return strings.TrimRight(registryURL, "/"), nil
}

// isA2APrivateHost returns true if the host resolves to a private/internal address.
func isA2APrivateHost(host string) bool {
	if host == "localhost" {
		return true
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	if ip.Equal(net.IPv4zero) {
		return true
	}

	privateRanges := []string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	}

	for _, r := range privateRanges {
		_, cidr, _ := net.ParseCIDR(r)
		if cidr.Contains(ip) {
			return true
		}
	}

	return false
}
