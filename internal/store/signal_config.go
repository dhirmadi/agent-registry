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

// SignalConfig represents a signal polling configuration entry.
type SignalConfig struct {
	ID           uuid.UUID `json:"id"`
	Source       string    `json:"source"`
	PollInterval string    `json:"poll_interval"`
	IsEnabled    bool      `json:"is_enabled"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// SignalConfigStore handles database operations for signal configuration.
type SignalConfigStore struct {
	pool *pgxpool.Pool
}

// NewSignalConfigStore creates a new SignalConfigStore.
func NewSignalConfigStore(pool *pgxpool.Pool) *SignalConfigStore {
	return &SignalConfigStore{pool: pool}
}

// List returns all signal configs ordered by source ascending.
func (s *SignalConfigStore) List(ctx context.Context) ([]SignalConfig, error) {
	query := `
		SELECT id, source, poll_interval, is_enabled, updated_at
		FROM signal_config
		ORDER BY source ASC`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing signal configs: %w", err)
	}
	defer rows.Close()

	var configs []SignalConfig
	for rows.Next() {
		var c SignalConfig
		if err := rows.Scan(&c.ID, &c.Source, &c.PollInterval, &c.IsEnabled, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning signal config: %w", err)
		}
		configs = append(configs, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating signal configs: %w", err)
	}

	return configs, nil
}

// GetByID retrieves a signal config by ID.
func (s *SignalConfigStore) GetByID(ctx context.Context, id uuid.UUID) (*SignalConfig, error) {
	query := `
		SELECT id, source, poll_interval, is_enabled, updated_at
		FROM signal_config WHERE id = $1`

	c := &SignalConfig{}
	err := s.pool.QueryRow(ctx, query, id).Scan(&c.ID, &c.Source, &c.PollInterval, &c.IsEnabled, &c.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("signal_config", id.String())
		}
		return nil, fmt.Errorf("getting signal config by id: %w", err)
	}
	return c, nil
}

// Update updates a signal config's poll_interval and is_enabled. Uses updated_at for optimistic concurrency.
func (s *SignalConfigStore) Update(ctx context.Context, config *SignalConfig, etag time.Time) error {
	query := `
		UPDATE signal_config SET
			poll_interval = $2, is_enabled = $3, updated_at = now()
		WHERE id = $1 AND updated_at = $4
		RETURNING updated_at`

	err := s.pool.QueryRow(ctx, query, config.ID, config.PollInterval, config.IsEnabled, etag).Scan(&config.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return errors.Conflict("signal config was modified by another request")
		}
		return fmt.Errorf("updating signal config: %w", err)
	}
	return nil
}
