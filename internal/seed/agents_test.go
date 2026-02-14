package seed

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/agent-smit/agentic-registry/internal/store"
)

// mockAgentStore implements both AgentStore and AgentCounter for testing
type mockAgentStore struct {
	createFunc    func(ctx context.Context, agent *store.Agent) error
	countAllFunc  func(ctx context.Context) (int, error)
	createdAgents []*store.Agent
}

func (m *mockAgentStore) Create(ctx context.Context, agent *store.Agent) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, agent)
	}
	m.createdAgents = append(m.createdAgents, agent)
	return nil
}

func (m *mockAgentStore) CountAll(ctx context.Context) (int, error) {
	if m.countAllFunc != nil {
		return m.countAllFunc(ctx)
	}
	return len(m.createdAgents), nil
}

func TestSeedAgents_EmptyDatabase(t *testing.T) {
	ctx := context.Background()
	mock := &mockAgentStore{
		countAllFunc: func(ctx context.Context) (int, error) {
			return 0, nil
		},
	}

	err := SeedAgents(ctx, mock)
	if err != nil {
		t.Fatalf("SeedAgents() failed: %v", err)
	}

	// Verify 16 agents were created
	if len(mock.createdAgents) != 16 {
		t.Errorf("expected 16 agents, got %d", len(mock.createdAgents))
	}

	// Verify first 6 agents have tools
	expectedFullAgents := []string{"router", "pmo", "raid_manager", "task_manager", "comms_manager", "meeting_manager"}
	for i, agentID := range expectedFullAgents {
		if i >= len(mock.createdAgents) {
			t.Fatalf("missing agent at index %d", i)
		}
		agent := mock.createdAgents[i]
		if agent.ID != agentID {
			t.Errorf("agent[%d]: expected ID %q, got %q", i, agentID, agent.ID)
		}
		if len(agent.Tools) == 0 {
			t.Errorf("agent %q should have tools", agentID)
		}
		// Verify tools is valid JSON array
		var tools []interface{}
		if err := json.Unmarshal(agent.Tools, &tools); err != nil {
			t.Errorf("agent %q has invalid tools JSON: %v", agentID, err)
		}
		if len(tools) == 0 {
			t.Errorf("agent %q should have at least one tool", agentID)
		}
	}

	// Verify remaining 10 agents are placeholders with empty tools
	expectedPlaceholders := []string{"engagement_pm", "knowledge_steward", "document_manager", "strategist", "backlog_steward", "team_manager", "slack_manager", "initiateproject", "meeting_processor", "comms_lead"}
	for i, agentID := range expectedPlaceholders {
		idx := 6 + i
		if idx >= len(mock.createdAgents) {
			t.Fatalf("missing placeholder agent at index %d", idx)
		}
		agent := mock.createdAgents[idx]
		if agent.ID != agentID {
			t.Errorf("agent[%d]: expected ID %q, got %q", idx, agentID, agent.ID)
		}
		// Verify tools is an empty JSON array
		var tools []interface{}
		if err := json.Unmarshal(agent.Tools, &tools); err != nil {
			t.Errorf("agent %q has invalid tools JSON: %v", agentID, err)
		}
		if len(tools) != 0 {
			t.Errorf("placeholder agent %q should have empty tools array, got %d items", agentID, len(tools))
		}
	}

	// Verify all agents have required fields
	for _, agent := range mock.createdAgents {
		if agent.ID == "" {
			t.Error("agent missing ID")
		}
		if agent.Name == "" {
			t.Errorf("agent %q missing Name", agent.ID)
		}
		if agent.Description == "" {
			t.Errorf("agent %q missing Description", agent.ID)
		}
		if agent.SystemPrompt == "" {
			t.Errorf("agent %q missing SystemPrompt", agent.ID)
		}
		if len(agent.TrustOverrides) == 0 {
			t.Errorf("agent %q missing TrustOverrides (should be '{}')", agent.ID)
		}
		if len(agent.ExamplePrompts) == 0 {
			t.Errorf("agent %q missing ExamplePrompts", agent.ID)
		}
		if !agent.IsActive {
			t.Errorf("agent %q should be active", agent.ID)
		}
		if agent.CreatedBy != "system-seed" {
			t.Errorf("agent %q: expected CreatedBy 'system-seed', got %q", agent.ID, agent.CreatedBy)
		}
	}
}

func TestSeedAgents_DatabaseNotEmpty(t *testing.T) {
	ctx := context.Background()
	mock := &mockAgentStore{
		countAllFunc: func(ctx context.Context) (int, error) {
			return 5, nil
		},
	}

	err := SeedAgents(ctx, mock)
	if err != nil {
		t.Fatalf("SeedAgents() failed: %v", err)
	}

	// Verify no agents were created
	if len(mock.createdAgents) != 0 {
		t.Errorf("expected 0 agents created when DB not empty, got %d", len(mock.createdAgents))
	}
}

func TestSeedAgents_CountError(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("database connection failed")
	mock := &mockAgentStore{
		countAllFunc: func(ctx context.Context) (int, error) {
			return 0, expectedErr
		},
	}

	err := SeedAgents(ctx, mock)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error to wrap %v, got %v", expectedErr, err)
	}
}

func TestSeedAgents_CreateError(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("insert failed")
	mock := &mockAgentStore{
		countAllFunc: func(ctx context.Context) (int, error) {
			return 0, nil
		},
		createFunc: func(ctx context.Context, agent *store.Agent) error {
			return expectedErr
		},
	}

	err := SeedAgents(ctx, mock)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error to wrap %v, got %v", expectedErr, err)
	}
}
