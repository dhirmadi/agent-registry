package gateway

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
)

// --- Mock providers ---

type mockTrustRuleProvider struct {
	rules []TrustRuleRecord
	err   error
}

func (m *mockTrustRuleProvider) List(ctx context.Context, workspaceID uuid.UUID) ([]TrustRuleRecord, error) {
	return m.rules, m.err
}

type mockTrustDefaultProvider struct {
	defaults []TrustDefaultRecord
	err      error
}

func (m *mockTrustDefaultProvider) List(ctx context.Context) ([]TrustDefaultRecord, error) {
	return m.defaults, m.err
}

type mockAgentTrustProvider struct {
	overrides map[string]string
	err       error
}

func (m *mockAgentTrustProvider) GetTrustOverrides(ctx context.Context, agentID string) (map[string]string, error) {
	return m.overrides, m.err
}

// --- Tests ---

func TestTrustClassifier_DefaultToAuto(t *testing.T) {
	tc := NewTrustClassifier(nil, nil, nil)
	tier, err := tc.Classify(context.Background(), ClassifyInput{ToolName: "git_commit"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != TrustAuto {
		t.Errorf("expected TrustAuto, got %q", tier)
	}
}

func TestTrustClassifier_DefaultToAutoWithEmptyProviders(t *testing.T) {
	tc := NewTrustClassifier(
		&mockTrustRuleProvider{rules: nil},
		&mockTrustDefaultProvider{defaults: nil},
		&mockAgentTrustProvider{overrides: nil},
	)
	tier, err := tc.Classify(context.Background(), ClassifyInput{ToolName: "anything"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != TrustAuto {
		t.Errorf("expected TrustAuto, got %q", tier)
	}
}

func TestTrustClassifier_AgentOverrideExactMatch(t *testing.T) {
	tc := NewTrustClassifier(nil, nil, &mockAgentTrustProvider{
		overrides: map[string]string{"git_push": "block"},
	})
	tier, err := tc.Classify(context.Background(), ClassifyInput{
		ToolName: "git_push",
		AgentID:  "agent-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != TrustBlock {
		t.Errorf("expected TrustBlock, got %q", tier)
	}
}

func TestTrustClassifier_AgentOverrideGlob(t *testing.T) {
	tc := NewTrustClassifier(nil, nil, &mockAgentTrustProvider{
		overrides: map[string]string{"git_*": "review"},
	})
	tier, err := tc.Classify(context.Background(), ClassifyInput{
		ToolName: "git_commit",
		AgentID:  "agent-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != TrustReview {
		t.Errorf("expected TrustReview, got %q", tier)
	}
}

func TestTrustClassifier_AgentOverridePrecedenceOverWorkspace(t *testing.T) {
	wsID := uuid.New()
	tc := NewTrustClassifier(
		&mockTrustRuleProvider{rules: []TrustRuleRecord{
			{ToolPattern: "git_push", Tier: "auto"},
		}},
		nil,
		&mockAgentTrustProvider{
			overrides: map[string]string{"git_push": "block"},
		},
	)
	tier, err := tc.Classify(context.Background(), ClassifyInput{
		ToolName:    "git_push",
		WorkspaceID: &wsID,
		AgentID:     "agent-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != TrustBlock {
		t.Errorf("expected agent override TrustBlock, got %q", tier)
	}
}

func TestTrustClassifier_WorkspaceRuleExactMatch(t *testing.T) {
	wsID := uuid.New()
	tc := NewTrustClassifier(
		&mockTrustRuleProvider{rules: []TrustRuleRecord{
			{ToolPattern: "file_delete", Tier: "block"},
		}},
		nil, nil,
	)
	tier, err := tc.Classify(context.Background(), ClassifyInput{
		ToolName:    "file_delete",
		WorkspaceID: &wsID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != TrustBlock {
		t.Errorf("expected TrustBlock, got %q", tier)
	}
}

func TestTrustClassifier_WorkspaceRuleGlob(t *testing.T) {
	wsID := uuid.New()
	tc := NewTrustClassifier(
		&mockTrustRuleProvider{rules: []TrustRuleRecord{
			{ToolPattern: "*_read", Tier: "auto"},
		}},
		nil, nil,
	)
	tier, err := tc.Classify(context.Background(), ClassifyInput{
		ToolName:    "file_read",
		WorkspaceID: &wsID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != TrustAuto {
		t.Errorf("expected TrustAuto, got %q", tier)
	}
}

func TestTrustClassifier_WorkspaceRulePrecedenceOverDefaults(t *testing.T) {
	wsID := uuid.New()
	tc := NewTrustClassifier(
		&mockTrustRuleProvider{rules: []TrustRuleRecord{
			{ToolPattern: "git_push", Tier: "review"},
		}},
		&mockTrustDefaultProvider{defaults: []TrustDefaultRecord{
			{ToolPattern: "git_push", Tier: "auto", Priority: 1},
		}},
		nil,
	)
	tier, err := tc.Classify(context.Background(), ClassifyInput{
		ToolName:    "git_push",
		WorkspaceID: &wsID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != TrustReview {
		t.Errorf("expected workspace TrustReview, got %q", tier)
	}
}

func TestTrustClassifier_SystemDefaultExactMatch(t *testing.T) {
	tc := NewTrustClassifier(nil,
		&mockTrustDefaultProvider{defaults: []TrustDefaultRecord{
			{ToolPattern: "shell_exec", Tier: "review", Priority: 1},
		}},
		nil,
	)
	tier, err := tc.Classify(context.Background(), ClassifyInput{ToolName: "shell_exec"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != TrustReview {
		t.Errorf("expected TrustReview, got %q", tier)
	}
}

func TestTrustClassifier_SystemDefaultPriorityOrder(t *testing.T) {
	tc := NewTrustClassifier(nil,
		&mockTrustDefaultProvider{defaults: []TrustDefaultRecord{
			{ToolPattern: "git_*", Tier: "review", Priority: 1},
			{ToolPattern: "*", Tier: "block", Priority: 2},
		}},
		nil,
	)
	// git_push should match git_* (priority 1) before * (priority 2)
	tier, err := tc.Classify(context.Background(), ClassifyInput{ToolName: "git_push"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != TrustReview {
		t.Errorf("expected TrustReview from higher priority, got %q", tier)
	}
}

func TestTrustClassifier_SystemDefaultCatchAll(t *testing.T) {
	tc := NewTrustClassifier(nil,
		&mockTrustDefaultProvider{defaults: []TrustDefaultRecord{
			{ToolPattern: "git_*", Tier: "review", Priority: 1},
			{ToolPattern: "*", Tier: "block", Priority: 2},
		}},
		nil,
	)
	// file_read doesn't match git_* but matches * catch-all
	tier, err := tc.Classify(context.Background(), ClassifyInput{ToolName: "file_read"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != TrustBlock {
		t.Errorf("expected TrustBlock from catch-all, got %q", tier)
	}
}

func TestTrustClassifier_NilWorkspaceSkipsRules(t *testing.T) {
	tc := NewTrustClassifier(
		&mockTrustRuleProvider{rules: []TrustRuleRecord{
			{ToolPattern: "*", Tier: "block"},
		}},
		nil, nil,
	)
	// WorkspaceID is nil, so workspace rules should be skipped
	tier, err := tc.Classify(context.Background(), ClassifyInput{
		ToolName:    "anything",
		WorkspaceID: nil,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != TrustAuto {
		t.Errorf("expected TrustAuto (workspace skipped), got %q", tier)
	}
}

func TestTrustClassifier_EmptyAgentSkipsOverrides(t *testing.T) {
	tc := NewTrustClassifier(nil, nil,
		&mockAgentTrustProvider{
			overrides: map[string]string{"*": "block"},
		},
	)
	// AgentID is empty, so agent overrides should be skipped
	tier, err := tc.Classify(context.Background(), ClassifyInput{
		ToolName: "anything",
		AgentID:  "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != TrustAuto {
		t.Errorf("expected TrustAuto (agent skipped), got %q", tier)
	}
}

func TestTrustClassifier_ErrorPropagation(t *testing.T) {
	tests := []struct {
		name string
		tc   *TrustClassifier
		input ClassifyInput
	}{
		{
			name: "agent provider error",
			tc: NewTrustClassifier(nil, nil, &mockAgentTrustProvider{
				err: fmt.Errorf("agent db error"),
			}),
			input: ClassifyInput{ToolName: "x", AgentID: "a1"},
		},
		{
			name: "workspace provider error",
			tc: NewTrustClassifier(
				&mockTrustRuleProvider{err: fmt.Errorf("rules db error")},
				nil, nil,
			),
			input: ClassifyInput{ToolName: "x", WorkspaceID: ptrUUID(uuid.New())},
		},
		{
			name: "defaults provider error",
			tc: NewTrustClassifier(nil,
				&mockTrustDefaultProvider{err: fmt.Errorf("defaults db error")},
				nil,
			),
			input: ClassifyInput{ToolName: "x"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.tc.Classify(context.Background(), tt.input)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestTrustClassifier_NoMatchInAgentFallsThrough(t *testing.T) {
	wsID := uuid.New()
	tc := NewTrustClassifier(
		&mockTrustRuleProvider{rules: []TrustRuleRecord{
			{ToolPattern: "file_delete", Tier: "block"},
		}},
		nil,
		&mockAgentTrustProvider{
			overrides: map[string]string{"git_*": "auto"},
		},
	)
	// Tool is file_delete, agent only overrides git_* -> falls through to workspace
	tier, err := tc.Classify(context.Background(), ClassifyInput{
		ToolName:    "file_delete",
		WorkspaceID: &wsID,
		AgentID:     "agent-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != TrustBlock {
		t.Errorf("expected TrustBlock from workspace fallthrough, got %q", tier)
	}
}

func TestTrustClassifier_FullPrecedenceChain(t *testing.T) {
	wsID := uuid.New()
	tc := NewTrustClassifier(
		&mockTrustRuleProvider{rules: []TrustRuleRecord{
			{ToolPattern: "*", Tier: "review"},
		}},
		&mockTrustDefaultProvider{defaults: []TrustDefaultRecord{
			{ToolPattern: "*", Tier: "block", Priority: 1},
		}},
		&mockAgentTrustProvider{
			overrides: map[string]string{"special_tool": "auto"},
		},
	)

	// Agent override wins for special_tool
	tier, err := tc.Classify(context.Background(), ClassifyInput{
		ToolName:    "special_tool",
		WorkspaceID: &wsID,
		AgentID:     "agent-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != TrustAuto {
		t.Errorf("expected TrustAuto from agent override, got %q", tier)
	}

	// Workspace rule wins for other_tool (no agent override match)
	tier, err = tc.Classify(context.Background(), ClassifyInput{
		ToolName:    "other_tool",
		WorkspaceID: &wsID,
		AgentID:     "agent-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != TrustReview {
		t.Errorf("expected TrustReview from workspace rule, got %q", tier)
	}

	// System default wins when no workspace
	tier, err = tc.Classify(context.Background(), ClassifyInput{
		ToolName: "other_tool",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != TrustBlock {
		t.Errorf("expected TrustBlock from system default, got %q", tier)
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"git_push", "git_push", true},
		{"git_push", "git_pull", false},
		{"git_*", "git_push", true},
		{"git_*", "git_commit", true},
		{"git_*", "file_read", false},
		{"*_read", "file_read", true},
		{"*_read", "file_write", false},
		{"*", "anything", true},
		{"[", "bad_pattern", false}, // invalid glob pattern
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/%s", tt.pattern, tt.name), func(t *testing.T) {
			got := matchGlob(tt.pattern, tt.name)
			if got != tt.want {
				t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.pattern, tt.name, got, tt.want)
			}
		})
	}
}

// helper
func ptrUUID(id uuid.UUID) *uuid.UUID {
	return &id
}
