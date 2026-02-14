package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agent-smit/agentic-registry/internal/errors"
)

// TrustRule represents a workspace-scoped trust classification rule.
type TrustRule struct {
	ID          uuid.UUID `json:"id" db:"id"`
	WorkspaceID uuid.UUID `json:"workspace_id" db:"workspace_id"`
	ToolPattern string    `json:"tool_pattern" db:"tool_pattern"`
	Tier        string    `json:"tier" db:"tier"`
	CreatedBy   string    `json:"created_by" db:"created_by"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// TrustRuleStore handles database operations for trust rules.
type TrustRuleStore struct {
	pool *pgxpool.Pool
}

// NewTrustRuleStore creates a new TrustRuleStore.
func NewTrustRuleStore(pool *pgxpool.Pool) *TrustRuleStore {
	return &TrustRuleStore{pool: pool}
}

// List returns all trust rules for a workspace.
func (s *TrustRuleStore) List(ctx context.Context, workspaceID uuid.UUID) ([]TrustRule, error) {
	query := `
		SELECT id, workspace_id, tool_pattern, tier, created_by, created_at, updated_at
		FROM trust_rules
		WHERE workspace_id = $1
		ORDER BY tool_pattern ASC`

	rows, err := s.pool.Query(ctx, query, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("listing trust rules: %w", err)
	}
	defer rows.Close()

	var rules []TrustRule
	for rows.Next() {
		var r TrustRule
		if err := rows.Scan(
			&r.ID, &r.WorkspaceID, &r.ToolPattern, &r.Tier,
			&r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning trust rule: %w", err)
		}
		rules = append(rules, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating trust rules: %w", err)
	}

	return rules, nil
}

// Upsert inserts a trust rule or updates the tier if the workspace_id+tool_pattern already exists.
func (s *TrustRuleStore) Upsert(ctx context.Context, rule *TrustRule) error {
	query := `
		INSERT INTO trust_rules (id, workspace_id, tool_pattern, tier, created_by)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (workspace_id, tool_pattern)
		DO UPDATE SET tier = $4, updated_at = now()
		RETURNING id, created_at, updated_at`

	if rule.ID == uuid.Nil {
		rule.ID = uuid.New()
	}

	err := s.pool.QueryRow(ctx, query,
		rule.ID, rule.WorkspaceID, rule.ToolPattern, rule.Tier, rule.CreatedBy,
	).Scan(&rule.ID, &rule.CreatedAt, &rule.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upserting trust rule: %w", err)
	}
	return nil
}

// Delete hard-deletes a trust rule by ID.
func (s *TrustRuleStore) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM trust_rules WHERE id = $1`
	ct, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("deleting trust rule: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return errors.NotFound("trust_rule", id.String())
	}
	return nil
}
