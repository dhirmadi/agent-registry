package api

import (
	"net/http"
	"time"

	apierrors "github.com/agent-smit/agentic-registry/internal/errors"
	"golang.org/x/sync/errgroup"
)

// DiscoveryHandler provides the discovery endpoint for fetching all active configuration.
type DiscoveryHandler struct {
	agents AgentStoreForAPI
	mcp    MCPServerStoreForAPI
	trust  TrustDefaultStoreForAPI
	model  ModelConfigStoreForAPI
}

// NewDiscoveryHandler creates a new DiscoveryHandler.
func NewDiscoveryHandler(
	agents AgentStoreForAPI,
	mcp MCPServerStoreForAPI,
	trust TrustDefaultStoreForAPI,
	model ModelConfigStoreForAPI,
) *DiscoveryHandler {
	return &DiscoveryHandler{
		agents: agents,
		mcp:    mcp,
		trust:  trust,
		model:  model,
	}
}

// GetDiscovery handles GET /api/v1/discovery and returns all active configuration
// in a single response for BFF cold-start.
func (h *DiscoveryHandler) GetDiscovery(w http.ResponseWriter, r *http.Request) {
	// Create errgroup to fetch all data in parallel
	g, ctx := errgroup.WithContext(r.Context())

	// Storage for results
	var (
		agentsList        []agentAPIResponse
		mcpServersList    []mcpServerResponse
		trustDefaultsList interface{}
		modelConfig       interface{}
	)

	// Fetch agents (active only, limit 1000)
	g.Go(func() error {
		agents, _, err := h.agents.List(ctx, true, 0, 1000)
		if err != nil {
			return err
		}
		agentsList = make([]agentAPIResponse, 0, len(agents))
		for i := range agents {
			// Use includeTools=false for summary view
			agentsList = append(agentsList, toAgentAPIResponse(&agents[i], false))
		}
		return nil
	})

	// Fetch MCP servers
	g.Go(func() error {
		servers, err := h.mcp.List(ctx)
		if err != nil {
			return err
		}
		mcpServersList = make([]mcpServerResponse, 0, len(servers))
		for i := range servers {
			mcpServersList = append(mcpServersList, toMCPServerResponse(&servers[i]))
		}
		return nil
	})

	// Fetch trust defaults
	g.Go(func() error {
		defaults, err := h.trust.List(ctx)
		if err != nil {
			return err
		}
		if defaults == nil {
			trustDefaultsList = []interface{}{}
		} else {
			trustDefaultsList = defaults
		}
		return nil
	})

	// Fetch global model config
	g.Go(func() error {
		config, err := h.model.GetByScope(ctx, "global", "")
		if err != nil {
			return err
		}
		if config == nil {
			modelConfig = map[string]interface{}{}
		} else {
			modelConfig = config
		}
		return nil
	})

	// Wait for all fetches to complete
	if err := g.Wait(); err != nil {
		RespondError(w, r, apierrors.Internal("failed to fetch discovery data"))
		return
	}

	// Build response
	response := map[string]interface{}{
		"agents":         agentsList,
		"mcp_servers":    mcpServersList,
		"trust_defaults": trustDefaultsList,
		"model_config":   modelConfig,
		"fetched_at":     time.Now().UTC().Format(time.RFC3339),
	}

	RespondJSON(w, r, http.StatusOK, response)
}
