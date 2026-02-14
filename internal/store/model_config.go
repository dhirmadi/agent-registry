package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agent-smit/agentic-registry/internal/errors"
)

// ModelConfig represents a model configuration at a given scope.
type ModelConfig struct {
	ID                     uuid.UUID `json:"id"`
	Scope                  string    `json:"scope"`
	ScopeID                string    `json:"scope_id"`
	DefaultModel           string    `json:"default_model"`
	Temperature            float64   `json:"temperature"`
	MaxTokens              int       `json:"max_tokens"`
	MaxToolRounds          int       `json:"max_tool_rounds"`
	DefaultContextWindow   int       `json:"default_context_window"`
	DefaultMaxOutputTokens int       `json:"default_max_output_tokens"`
	HistoryTokenBudget     int       `json:"history_token_budget"`
	MaxHistoryMessages     int       `json:"max_history_messages"`
	EmbeddingModel         string    `json:"embedding_model"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// ModelConfigStore handles database operations for model configuration.
type ModelConfigStore struct {
	pool *pgxpool.Pool
}

// NewModelConfigStore creates a new ModelConfigStore.
func NewModelConfigStore(pool *pgxpool.Pool) *ModelConfigStore {
	return &ModelConfigStore{pool: pool}
}

var modelConfigColumns = `id, scope, scope_id, default_model, temperature, max_tokens, max_tool_rounds,
	default_context_window, default_max_output_tokens, history_token_budget,
	max_history_messages, embedding_model, updated_at`

func scanModelConfig(row pgx.Row) (*ModelConfig, error) {
	c := &ModelConfig{}
	err := row.Scan(
		&c.ID, &c.Scope, &c.ScopeID, &c.DefaultModel, &c.Temperature,
		&c.MaxTokens, &c.MaxToolRounds, &c.DefaultContextWindow,
		&c.DefaultMaxOutputTokens, &c.HistoryTokenBudget,
		&c.MaxHistoryMessages, &c.EmbeddingModel, &c.UpdatedAt,
	)
	return c, err
}

// GetByScope returns the model config for a specific scope and scope ID.
func (s *ModelConfigStore) GetByScope(ctx context.Context, scope, scopeID string) (*ModelConfig, error) {
	query := fmt.Sprintf(`SELECT %s FROM model_config WHERE scope = $1 AND scope_id = $2`, modelConfigColumns)

	c, err := scanModelConfig(s.pool.QueryRow(ctx, query, scope, scopeID))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("model_config", scope+"/"+scopeID)
		}
		return nil, fmt.Errorf("getting model config by scope: %w", err)
	}
	return c, nil
}

// GetMerged returns a model config that merges global defaults with narrower scope overrides.
// For string fields, empty means not overridden. For numeric fields, 0 means not overridden.
func (s *ModelConfigStore) GetMerged(ctx context.Context, scope, scopeID string) (*ModelConfig, error) {
	// Start with global
	global, err := s.GetByScope(ctx, "global", "")
	if err != nil {
		return nil, fmt.Errorf("getting global model config: %w", err)
	}

	if scope == "global" {
		return global, nil
	}

	// Overlay workspace if applicable
	if scope == "workspace" || scope == "user" {
		ws, err := s.GetByScope(ctx, "workspace", scopeID)
		if err == nil {
			mergeModelConfig(global, ws)
		}
	}

	// Overlay user if applicable
	if scope == "user" {
		user, err := s.GetByScope(ctx, "user", scopeID)
		if err == nil {
			mergeModelConfig(global, user)
		}
	}

	return global, nil
}

// mergeModelConfig overlays non-zero values from overlay onto base.
func mergeModelConfig(base, overlay *ModelConfig) {
	if overlay.DefaultModel != "" {
		base.DefaultModel = overlay.DefaultModel
	}
	if overlay.Temperature != 0.0 {
		base.Temperature = overlay.Temperature
	}
	if overlay.MaxTokens != 0 {
		base.MaxTokens = overlay.MaxTokens
	}
	if overlay.MaxToolRounds != 0 {
		base.MaxToolRounds = overlay.MaxToolRounds
	}
	if overlay.DefaultContextWindow != 0 {
		base.DefaultContextWindow = overlay.DefaultContextWindow
	}
	if overlay.DefaultMaxOutputTokens != 0 {
		base.DefaultMaxOutputTokens = overlay.DefaultMaxOutputTokens
	}
	if overlay.HistoryTokenBudget != 0 {
		base.HistoryTokenBudget = overlay.HistoryTokenBudget
	}
	if overlay.MaxHistoryMessages != 0 {
		base.MaxHistoryMessages = overlay.MaxHistoryMessages
	}
	if overlay.EmbeddingModel != "" {
		base.EmbeddingModel = overlay.EmbeddingModel
	}
}

// Update updates a model config using optimistic concurrency via updated_at.
func (s *ModelConfigStore) Update(ctx context.Context, config *ModelConfig, etag time.Time) error {
	query := `
		UPDATE model_config SET
			default_model = $3,
			temperature = $4,
			max_tokens = $5,
			max_tool_rounds = $6,
			default_context_window = $7,
			default_max_output_tokens = $8,
			history_token_budget = $9,
			max_history_messages = $10,
			embedding_model = $11,
			updated_at = now()
		WHERE scope = $1 AND scope_id = $2 AND updated_at = $12
		RETURNING updated_at`

	err := s.pool.QueryRow(ctx, query,
		config.Scope, config.ScopeID,
		config.DefaultModel, config.Temperature, config.MaxTokens,
		config.MaxToolRounds, config.DefaultContextWindow, config.DefaultMaxOutputTokens,
		config.HistoryTokenBudget, config.MaxHistoryMessages, config.EmbeddingModel,
		etag,
	).Scan(&config.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return errors.Conflict("model config was modified by another request")
		}
		return fmt.Errorf("updating model config: %w", err)
	}
	return nil
}

// Upsert inserts or updates a model config for a given scope.
func (s *ModelConfigStore) Upsert(ctx context.Context, config *ModelConfig) error {
	query := fmt.Sprintf(`
		INSERT INTO model_config (scope, scope_id, default_model, temperature, max_tokens,
			max_tool_rounds, default_context_window, default_max_output_tokens,
			history_token_budget, max_history_messages, embedding_model)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (scope, scope_id) DO UPDATE SET
			default_model = EXCLUDED.default_model,
			temperature = EXCLUDED.temperature,
			max_tokens = EXCLUDED.max_tokens,
			max_tool_rounds = EXCLUDED.max_tool_rounds,
			default_context_window = EXCLUDED.default_context_window,
			default_max_output_tokens = EXCLUDED.default_max_output_tokens,
			history_token_budget = EXCLUDED.history_token_budget,
			max_history_messages = EXCLUDED.max_history_messages,
			embedding_model = EXCLUDED.embedding_model,
			updated_at = now()
		RETURNING %s`, modelConfigColumns)

	_, err := scanModelConfig(s.pool.QueryRow(ctx, query,
		config.Scope, config.ScopeID,
		config.DefaultModel, config.Temperature, config.MaxTokens,
		config.MaxToolRounds, config.DefaultContextWindow, config.DefaultMaxOutputTokens,
		config.HistoryTokenBudget, config.MaxHistoryMessages, config.EmbeddingModel,
	))
	if err != nil {
		return fmt.Errorf("upserting model config: %w", err)
	}
	return nil
}
