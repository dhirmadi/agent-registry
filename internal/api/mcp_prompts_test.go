package api

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/agent-smit/agentic-registry/internal/store"
)

func TestMCPPromptProvider_ListPrompts(t *testing.T) {
	agents := []store.Agent{
		{ID: "code_agent", Name: "Code Agent", IsActive: true},
		{ID: "test_agent", Name: "Test Agent", IsActive: true},
		{ID: "no_prompt_agent", Name: "No Prompt", IsActive: true},
	}
	agentStore := &mockAgentStoreForMCP{
		agents: agents,
		byID:   map[string]*store.Agent{},
	}

	templateVars := json.RawMessage(`[{"name":"language","description":"Programming language","required":true}]`)
	promptStore := &mockPromptStoreForMCP{
		prompts: map[string]*store.Prompt{
			"code_agent": {
				AgentID:      "code_agent",
				SystemPrompt: "You write {{language}} code",
				TemplateVars: templateVars,
				Version:      2,
				IsActive:     true,
				Mode:         "toolcalling_auto",
			},
			"test_agent": {
				AgentID:      "test_agent",
				SystemPrompt: "You test code",
				Version:      1,
				IsActive:     true,
				Mode:         "rag_readonly",
			},
		},
	}

	p := NewMCPPromptProvider(agentStore, promptStore)
	defs, err := p.ListPrompts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// no_prompt_agent should be skipped
	if len(defs) != 2 {
		t.Fatalf("expected 2 prompt definitions, got %d", len(defs))
	}

	// First prompt should be code_agent with arguments
	if defs[0].Name != "code_agent" {
		t.Errorf("expected name code_agent, got %s", defs[0].Name)
	}
	if defs[0].Description == "" {
		t.Error("expected non-empty description")
	}
	if len(defs[0].Arguments) != 1 {
		t.Fatalf("expected 1 argument, got %d", len(defs[0].Arguments))
	}
	if defs[0].Arguments[0].Name != "language" {
		t.Errorf("expected argument name=language, got %s", defs[0].Arguments[0].Name)
	}
	if !defs[0].Arguments[0].Required {
		t.Error("expected argument to be required")
	}

	// Second prompt should be test_agent with no arguments
	if defs[1].Name != "test_agent" {
		t.Errorf("expected name test_agent, got %s", defs[1].Name)
	}
	if defs[1].Arguments != nil {
		t.Errorf("expected nil arguments, got %v", defs[1].Arguments)
	}
}

func TestMCPPromptProvider_ListPrompts_NoAgents(t *testing.T) {
	agentStore := &mockAgentStoreForMCP{agents: nil}
	promptStore := &mockPromptStoreForMCP{prompts: map[string]*store.Prompt{}}
	p := NewMCPPromptProvider(agentStore, promptStore)

	defs, err := p.ListPrompts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(defs) != 0 {
		t.Errorf("expected 0 definitions, got %d", len(defs))
	}
}

func TestMCPPromptProvider_ListPrompts_StoreError(t *testing.T) {
	agentStore := &mockAgentStoreForMCP{err: fmt.Errorf("db down")}
	p := NewMCPPromptProvider(agentStore, nil)

	_, err := p.ListPrompts(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMCPPromptProvider_GetPrompt(t *testing.T) {
	agent := &store.Agent{ID: "code_agent", Name: "Code Agent"}
	agentStore := &mockAgentStoreForMCP{
		byID: map[string]*store.Agent{"code_agent": agent},
	}
	promptStore := &mockPromptStoreForMCP{
		prompts: map[string]*store.Prompt{
			"code_agent": {
				AgentID:      "code_agent",
				SystemPrompt: "You write {{language}} code for {{project}}",
				Version:      3,
				Mode:         "toolcalling_auto",
				IsActive:     true,
			},
		},
	}

	p := NewMCPPromptProvider(agentStore, promptStore)

	t.Run("without args", func(t *testing.T) {
		result, rpcErr := p.GetPrompt(context.Background(), "code_agent", nil)
		if rpcErr != nil {
			t.Fatalf("unexpected error: %+v", rpcErr)
		}

		if result.Description == "" {
			t.Error("expected non-empty description")
		}
		if len(result.Messages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(result.Messages))
		}
		msg := result.Messages[0]
		if msg.Role != "user" {
			t.Errorf("expected role=user, got %s", msg.Role)
		}
		if msg.Content.Type != "text" {
			t.Errorf("expected type=text, got %s", msg.Content.Type)
		}
		if msg.Content.Text != "You write {{language}} code for {{project}}" {
			t.Errorf("unexpected text: %s", msg.Content.Text)
		}
	})

	t.Run("with args substitution", func(t *testing.T) {
		args := map[string]string{
			"language": "Go",
			"project":  "registry",
		}
		result, rpcErr := p.GetPrompt(context.Background(), "code_agent", args)
		if rpcErr != nil {
			t.Fatalf("unexpected error: %+v", rpcErr)
		}

		expected := "You write Go code for registry"
		if result.Messages[0].Content.Text != expected {
			t.Errorf("expected %q, got %q", expected, result.Messages[0].Content.Text)
		}
	})

	t.Run("with partial args", func(t *testing.T) {
		args := map[string]string{"language": "Python"}
		result, rpcErr := p.GetPrompt(context.Background(), "code_agent", args)
		if rpcErr != nil {
			t.Fatalf("unexpected error: %+v", rpcErr)
		}

		expected := "You write Python code for {{project}}"
		if result.Messages[0].Content.Text != expected {
			t.Errorf("expected %q, got %q", expected, result.Messages[0].Content.Text)
		}
	})
}

func TestMCPPromptProvider_GetPrompt_NotFound(t *testing.T) {
	promptStore := &mockPromptStoreForMCP{prompts: map[string]*store.Prompt{}}
	agentStore := &mockAgentStoreForMCP{byID: map[string]*store.Agent{}}
	p := NewMCPPromptProvider(agentStore, promptStore)

	_, rpcErr := p.GetPrompt(context.Background(), "nonexistent", nil)
	if rpcErr == nil {
		t.Fatal("expected error")
	}
	if rpcErr.Code != -32602 {
		t.Errorf("expected code -32602, got %d", rpcErr.Code)
	}
}

func TestMCPPromptProvider_GetPrompt_EmptyName(t *testing.T) {
	p := NewMCPPromptProvider(nil, nil)

	_, rpcErr := p.GetPrompt(context.Background(), "", nil)
	if rpcErr == nil {
		t.Fatal("expected error")
	}
	if rpcErr.Code != -32602 {
		t.Errorf("expected code -32602, got %d", rpcErr.Code)
	}
}

func TestMCPPromptProvider_GetPrompt_AgentNotFound(t *testing.T) {
	// Prompt exists but agent lookup fails — should still return prompt with empty description
	promptStore := &mockPromptStoreForMCP{
		prompts: map[string]*store.Prompt{
			"orphan": {
				AgentID:      "orphan",
				SystemPrompt: "test prompt",
				Version:      1,
				Mode:         "rag_readonly",
			},
		},
	}
	agentStore := &mockAgentStoreForMCP{byID: map[string]*store.Agent{}}
	p := NewMCPPromptProvider(agentStore, promptStore)

	result, rpcErr := p.GetPrompt(context.Background(), "orphan", nil)
	if rpcErr != nil {
		t.Fatalf("unexpected error: %+v", rpcErr)
	}

	if result.Description != "" {
		t.Errorf("expected empty description when agent not found, got %q", result.Description)
	}
	if result.Messages[0].Content.Text != "test prompt" {
		t.Errorf("unexpected text: %s", result.Messages[0].Content.Text)
	}
}

func TestParseTemplateVarsToArguments(t *testing.T) {
	tests := []struct {
		name     string
		input    json.RawMessage
		expected int
	}{
		{"nil", nil, 0},
		{"null", json.RawMessage(`null`), 0},
		{"empty array", json.RawMessage(`[]`), 0},
		{"one var", json.RawMessage(`[{"name":"lang","description":"Language","required":true}]`), 1},
		{"two vars", json.RawMessage(`[{"name":"a","description":"A"},{"name":"b","description":"B","required":true}]`), 2},
		{"invalid json", json.RawMessage(`not json`), 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := parseTemplateVarsToArguments(tc.input)
			if tc.expected == 0 {
				if args != nil && len(args) > 0 {
					t.Errorf("expected nil/empty, got %d args", len(args))
				}
			} else {
				if len(args) != tc.expected {
					t.Errorf("expected %d args, got %d", tc.expected, len(args))
				}
			}
		})
	}
}

func TestSubstituteTemplateVars(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		args     map[string]string
		expected string
	}{
		{
			"single replacement",
			"Hello {{name}}!",
			map[string]string{"name": "World"},
			"Hello World!",
		},
		{
			"multiple replacements",
			"{{greeting}} {{name}}!",
			map[string]string{"greeting": "Hi", "name": "Go"},
			"Hi Go!",
		},
		{
			"no matches",
			"Hello World",
			map[string]string{"name": "Go"},
			"Hello World",
		},
		{
			"empty args",
			"Hello {{name}}",
			map[string]string{},
			"Hello {{name}}",
		},
		{
			"repeated var",
			"{{x}} and {{x}}",
			map[string]string{"x": "Y"},
			"Y and Y",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := substituteTemplateVars(tc.text, tc.args)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}
