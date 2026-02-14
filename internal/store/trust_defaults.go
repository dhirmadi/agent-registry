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

// TrustDefault represents a global trust classification default.
type TrustDefault struct {
	ID        uuid.UUID       `json:"id" db:"id"`
	Tier      string          `json:"tier" db:"tier"`
	Patterns  json.RawMessage `json:"patterns" db:"patterns"`
	Priority  int             `json:"priority" db:"priority"`
	UpdatedAt time.Time       `json:"updated_at" db:"updated_at"`
}

// TrustDefaultStore handles database operations for trust defaults.
type TrustDefaultStore struct {
	pool *pgxpool.Pool
}

// NewTrustDefaultStore creates a new TrustDefaultStore.
func NewTrustDefaultStore(pool *pgxpool.Pool) *TrustDefaultStore {
	return &TrustDefaultStore{pool: pool}
}

// List returns all trust defaults ordered by priority.
func (s *TrustDefaultStore) List(ctx context.Context) ([]TrustDefault, error) {
	query := `
		SELECT id, tier, patterns, priority, updated_at
		FROM trust_defaults
		ORDER BY priority ASC`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing trust defaults: %w", err)
	}
	defer rows.Close()

	var defaults []TrustDefault
	for rows.Next() {
		var d TrustDefault
		if err := rows.Scan(&d.ID, &d.Tier, &d.Patterns, &d.Priority, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning trust default: %w", err)
		}
		defaults = append(defaults, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating trust defaults: %w", err)
	}

	return defaults, nil
}

// GetByID retrieves a trust default by ID.
func (s *TrustDefaultStore) GetByID(ctx context.Context, id uuid.UUID) (*TrustDefault, error) {
	query := `
		SELECT id, tier, patterns, priority, updated_at
		FROM trust_defaults WHERE id = $1`

	d := &TrustDefault{}
	err := s.pool.QueryRow(ctx, query, id).Scan(&d.ID, &d.Tier, &d.Patterns, &d.Priority, &d.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("trust_default", id.String())
		}
		return nil, fmt.Errorf("getting trust default by id: %w", err)
	}
	return d, nil
}

// Update updates a trust default's patterns. Uses updated_at for optimistic concurrency.
func (s *TrustDefaultStore) Update(ctx context.Context, d *TrustDefault) error {
	query := `
		UPDATE trust_defaults SET
			patterns = $2, updated_at = now()
		WHERE id = $1 AND updated_at = $3
		RETURNING updated_at`

	err := s.pool.QueryRow(ctx, query, d.ID, d.Patterns, d.UpdatedAt).Scan(&d.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return errors.Conflict("trust default was modified by another request")
		}
		return fmt.Errorf("updating trust default: %w", err)
	}
	return nil
}
