package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// MCPManifest is the server manifest served at GET /mcp.json.
type MCPManifest struct {
	Name           string                 `json:"name"`
	Version        string                 `json:"version"`
	Description    string                 `json:"description"`
	Transport      map[string]interface{} `json:"transport"`
	Authentication map[string]interface{} `json:"authentication"`
	Tools          []MCPManifestTool      `json:"tools"`
}

// MCPManifestTool is a tool summary in the manifest.
type MCPManifestTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// MCPManifestHandler serves the /mcp.json discovery manifest.
type MCPManifestHandler struct {
	externalURL string
}

// NewMCPManifestHandler creates a new MCPManifestHandler.
func NewMCPManifestHandler(externalURL string) *MCPManifestHandler {
	return &MCPManifestHandler{externalURL: externalURL}
}

// GetManifest handles GET /mcp.json (public, no auth).
func (h *MCPManifestHandler) GetManifest(w http.ResponseWriter, r *http.Request) {
	baseURL := strings.TrimRight(h.externalURL, "/")

	manifest := MCPManifest{
		Name:        "agentic-registry",
		Version:     "1.0.0",
		Description: "Agentic Registry — central configuration service for AI agents, exposing agent definitions, prompts, model config, and MCP server metadata via the Model Context Protocol.",
		Transport: map[string]interface{}{
			"streamableHttp": map[string]interface{}{
				"url": baseURL + "/mcp/v1",
			},
		},
		Authentication: map[string]interface{}{
			"type": "bearer",
		},
		Tools: []MCPManifestTool{
			{Name: "list_agents", Description: "List all active agent definitions"},
			{Name: "get_agent", Description: "Get a single agent with its active prompt"},
			{Name: "get_discovery", Description: "Get the full composite discovery payload"},
			{Name: "list_mcp_servers", Description: "List MCP server configurations (without credentials)"},
			{Name: "get_model_config", Description: "Get merged model configuration for a scope"},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(manifest)
}
