package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Types MCPPromptArgument, MCPPromptDefinition, MCPPromptMessage, MCPTextContent,
// MCPPromptResult, MCPJSONRPCError are defined in mcp_types.go.

// --- Prompt provider ---

// MCPPromptProvider implements MCP prompts/list and prompts/get.
type MCPPromptProvider struct {
	agents  AgentStoreForMCP
	prompts PromptStoreForMCP
}

// NewMCPPromptProvider creates a new MCPPromptProvider.
func NewMCPPromptProvider(agents AgentStoreForMCP, prompts PromptStoreForMCP) *MCPPromptProvider {
	return &MCPPromptProvider{
		agents:  agents,
		prompts: prompts,
	}
}

// templateVar represents one entry in a prompt's template_vars JSON array.
type templateVar struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// ListPrompts returns all agents that have an active prompt, expressed as
// MCP prompt definitions. Template variables are exposed as prompt arguments.
func (p *MCPPromptProvider) ListPrompts(ctx context.Context) ([]MCPPromptDefinition, error) {
	agents, _, err := p.agents.List(ctx, true, 0, 1000)
	if err != nil {
		return nil, fmt.Errorf("listing agents: %w", err)
	}

	var defs []MCPPromptDefinition
	for _, a := range agents {
		prompt, err := p.prompts.GetActive(ctx, a.ID)
		if err != nil {
			// Agent has no active prompt — skip
			continue
		}

		def := MCPPromptDefinition{
			Name:        a.ID,
			Description: fmt.Sprintf("Active prompt for %s", a.Name),
		}

		// Parse template_vars into MCP arguments
		def.Arguments = parseTemplateVarsToArguments(prompt.TemplateVars)
		defs = append(defs, def)
	}

	if defs == nil {
		defs = []MCPPromptDefinition{}
	}
	return defs, nil
}

// GetPrompt fetches the active prompt for the named agent and returns it as
// an MCP prompt result with a single system message.
func (p *MCPPromptProvider) GetPrompt(ctx context.Context, name string, args map[string]string) (*MCPPromptResult, *MCPJSONRPCError) {
	if name == "" {
		return nil, &MCPJSONRPCError{Code: -32602, Message: "prompt name is required"}
	}

	prompt, err := p.prompts.GetActive(ctx, name)
	if err != nil {
		return nil, &MCPJSONRPCError{
			Code:    -32602,
			Message: fmt.Sprintf("active prompt not found for agent: %s", name),
		}
	}

	// Get agent for the description
	agent, _ := p.agents.GetByID(ctx, name)
	description := ""
	if agent != nil {
		description = fmt.Sprintf("Active prompt for %s (v%d, mode: %s)", agent.Name, prompt.Version, prompt.Mode)
	}

	text := prompt.SystemPrompt

	// Substitute template vars if args provided
	if len(args) > 0 {
		text = substituteTemplateVars(text, args)
	}

	return &MCPPromptResult{
		Description: description,
		Messages: []MCPPromptMessage{
			{
				Role: "user",
				Content: MCPTextContent{
					Type: "text",
					Text: text,
				},
			},
		},
	}, nil
}

// parseTemplateVarsToArguments converts the prompt's template_vars JSON into MCP arguments.
func parseTemplateVarsToArguments(templateVars json.RawMessage) []MCPPromptArgument {
	if len(templateVars) == 0 || string(templateVars) == "null" {
		return nil
	}

	var vars []templateVar
	if err := json.Unmarshal(templateVars, &vars); err != nil {
		return nil
	}

	args := make([]MCPPromptArgument, 0, len(vars))
	for _, v := range vars {
		args = append(args, MCPPromptArgument{
			Name:        v.Name,
			Description: v.Description,
			Required:    v.Required,
		})
	}
	return args
}

// substituteTemplateVars does simple {{key}} replacement in the prompt text.
func substituteTemplateVars(text string, args map[string]string) string {
	for k, v := range args {
		text = strings.ReplaceAll(text, "{{"+k+"}}", v)
	}
	return text
}
