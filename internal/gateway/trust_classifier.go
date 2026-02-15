package gateway

import (
	"context"
	"path"

	"github.com/google/uuid"
)

// TrustTier represents the trust level for a tool.
type TrustTier string

const (
	TrustAuto   TrustTier = "auto"
	TrustReview TrustTier = "review"
	TrustBlock  TrustTier = "block"
)

// TrustRuleRecord is a minimal view of a workspace trust rule.
type TrustRuleRecord struct {
	ToolPattern string
	Tier        string
}

// TrustDefaultRecord is a minimal view of a system trust default.
type TrustDefaultRecord struct {
	ToolPattern string
	Tier        string
	Priority    int
}

// TrustRuleProvider lists workspace-scoped trust rules.
type TrustRuleProvider interface {
	List(ctx context.Context, workspaceID uuid.UUID) ([]TrustRuleRecord, error)
}

// TrustDefaultProvider lists system trust defaults ordered by priority.
type TrustDefaultProvider interface {
	List(ctx context.Context) ([]TrustDefaultRecord, error)
}

// AgentTrustProvider returns agent-level trust overrides.
type AgentTrustProvider interface {
	GetTrustOverrides(ctx context.Context, agentID string) (map[string]string, error)
}

// TrustClassifier evaluates trust tier for tool calls using a 4-level precedence chain.
type TrustClassifier struct {
	rules    TrustRuleProvider
	defaults TrustDefaultProvider
	agents   AgentTrustProvider
}

// NewTrustClassifier creates a new trust classifier.
func NewTrustClassifier(rules TrustRuleProvider, defaults TrustDefaultProvider, agents AgentTrustProvider) *TrustClassifier {
	return &TrustClassifier{
		rules:    rules,
		defaults: defaults,
		agents:   agents,
	}
}

// ClassifyInput contains context for trust classification.
type ClassifyInput struct {
	ToolName    string
	WorkspaceID *uuid.UUID
	AgentID     string
}

// Classify evaluates trust tier using precedence chain:
//  1. Agent trust_overrides (if AgentID provided)
//  2. Workspace trust_rules (if WorkspaceID provided)
//  3. System trust_defaults (ordered by priority)
//  4. Default: TrustAuto
func (tc *TrustClassifier) Classify(ctx context.Context, input ClassifyInput) (TrustTier, error) {
	// Level 1: Agent overrides
	if input.AgentID != "" && tc.agents != nil {
		overrides, err := tc.agents.GetTrustOverrides(ctx, input.AgentID)
		if err != nil {
			return "", err
		}
		for pattern, tier := range overrides {
			if matchGlob(pattern, input.ToolName) {
				return normalizeTrustTier(tier), nil
			}
		}
	}

	// Level 2: Workspace rules
	if input.WorkspaceID != nil && tc.rules != nil {
		rules, err := tc.rules.List(ctx, *input.WorkspaceID)
		if err != nil {
			return "", err
		}
		for _, rule := range rules {
			if matchGlob(rule.ToolPattern, input.ToolName) {
				return normalizeTrustTier(rule.Tier), nil
			}
		}
	}

	// Level 3: System defaults (ordered by priority)
	if tc.defaults != nil {
		defaults, err := tc.defaults.List(ctx)
		if err != nil {
			return "", err
		}
		for _, def := range defaults {
			if matchGlob(def.ToolPattern, input.ToolName) {
				return normalizeTrustTier(def.Tier), nil
			}
		}
	}

	// Level 4: Default to auto
	return TrustAuto, nil
}

// normalizeTrustTier validates and normalizes a trust tier string.
// Returns TrustBlock for invalid/unknown values (safest default).
func normalizeTrustTier(tier string) TrustTier {
	switch TrustTier(tier) {
	case TrustAuto, TrustReview, TrustBlock:
		return TrustTier(tier)
	default:
		// Invalid tier values default to block for safety
		return TrustBlock
	}
}

// matchGlob matches a glob pattern against a name using path.Match.
func matchGlob(pattern, name string) bool {
	matched, err := path.Match(pattern, name)
	return err == nil && matched
}
