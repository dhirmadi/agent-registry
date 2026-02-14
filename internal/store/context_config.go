package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agent-smit/agentic-registry/internal/errors"
)

// ContextConfig represents context assembly configuration for an agent scope.
type ContextConfig struct {
	ID             uuid.UUID       `json:"id"`
	Scope          string          `json:"scope"`
	ScopeID        string          `json:"scope_id"`
	MaxTotalTokens int             `json:"max_total_tokens"`
	LayerBudgets   json.RawMessage `json:"layer_budgets"`
	EnabledLayers  json.RawMessage `json:"enabled_layers"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// ContextConfigStore handles database operations for context config.
type ContextConfigStore struct {
	pool *pgxpool.Pool
}

// NewContextConfigStore creates a new ContextConfigStore.
func NewContextConfigStore(pool *pgxpool.Pool) *ContextConfigStore {
	return &ContextConfigStore{pool: pool}
}

// GetByScope retrieves the context config for a specific scope and scope_id.
func (s *ContextConfigStore) GetByScope(ctx context.Context, scope, scopeID string) (*ContextConfig, error) {
	query := `
		SELECT id, scope, scope_id, max_total_tokens, layer_budgets, enabled_layers, updated_at
		FROM context_config
		WHERE scope = $1 AND scope_id = $2`

	c := &ContextConfig{}
	err := s.pool.QueryRow(ctx, query, scope, scopeID).Scan(
		&c.ID, &c.Scope, &c.ScopeID, &c.MaxTotalTokens,
		&c.LayerBudgets, &c.EnabledLayers, &c.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("context_config", scope+"/"+scopeID)
		}
		return nil, fmt.Errorf("getting context config by scope: %w", err)
	}
	return c, nil
}

// GetMerged retrieves the effective context config for a scope by merging global
// defaults with workspace overrides. For int fields, 0 means not overridden.
// For JSONB fields, null/empty means not overridden.
func (s *ContextConfigStore) GetMerged(ctx context.Context, scope, scopeID string) (*ContextConfig, error) {
	// Always start with global config
	global, err := s.GetByScope(ctx, "global", "")
	if err != nil {
		return nil, fmt.Errorf("getting global context config: %w", err)
	}

	if scope == "global" {
		return global, nil
	}

	// Try to get workspace override
	ws, err := s.GetByScope(ctx, scope, scopeID)
	if err != nil {
		// No workspace override, return global as-is
		return global, nil
	}

	// Merge: workspace fields override global when non-zero/non-empty
	merged := &ContextConfig{
		ID:             ws.ID,
		Scope:          ws.Scope,
		ScopeID:        ws.ScopeID,
		MaxTotalTokens: global.MaxTotalTokens,
		LayerBudgets:   global.LayerBudgets,
		EnabledLayers:  global.EnabledLayers,
		UpdatedAt:      ws.UpdatedAt,
	}

	if ws.MaxTotalTokens != 0 {
		merged.MaxTotalTokens = ws.MaxTotalTokens
	}
	if len(ws.LayerBudgets) > 0 && string(ws.LayerBudgets) != "null" {
		merged.LayerBudgets = ws.LayerBudgets
	}
	if len(ws.EnabledLayers) > 0 && string(ws.EnabledLayers) != "null" {
		merged.EnabledLayers = ws.EnabledLayers
	}

	return merged, nil
}

// Update updates a context config using optimistic concurrency via updated_at.
func (s *ContextConfigStore) Update(ctx context.Context, config *ContextConfig, etag time.Time) error {
	query := `
		UPDATE context_config SET
			max_total_tokens = $3,
			layer_budgets = $4,
			enabled_layers = $5,
			updated_at = now()
		WHERE scope = $1 AND scope_id = $2 AND updated_at = $6
		RETURNING updated_at`

	err := s.pool.QueryRow(ctx, query,
		config.Scope, config.ScopeID,
		config.MaxTotalTokens, config.LayerBudgets, config.EnabledLayers,
		etag,
	).Scan(&config.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return errors.Conflict("context config was modified by another request")
		}
		return fmt.Errorf("updating context config: %w", err)
	}
	return nil
}

// Upsert inserts or updates a context config for the given scope.
func (s *ContextConfigStore) Upsert(ctx context.Context, config *ContextConfig) error {
	query := `
		INSERT INTO context_config (scope, scope_id, max_total_tokens, layer_budgets, enabled_layers)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (scope, scope_id) DO UPDATE SET
			max_total_tokens = EXCLUDED.max_total_tokens,
			layer_budgets = EXCLUDED.layer_budgets,
			enabled_layers = EXCLUDED.enabled_layers,
			updated_at = now()
		RETURNING id, updated_at`

	err := s.pool.QueryRow(ctx, query,
		config.Scope, config.ScopeID,
		config.MaxTotalTokens, config.LayerBudgets, config.EnabledLayers,
	).Scan(&config.ID, &config.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upserting context config: %w", err)
	}
	return nil
}
