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

// TriggerRule represents an event-driven or scheduled trigger rule.
type TriggerRule struct {
	ID               uuid.UUID       `json:"id" db:"id"`
	WorkspaceID      uuid.UUID       `json:"workspace_id" db:"workspace_id"`
	Name             string          `json:"name" db:"name"`
	EventType        string          `json:"event_type" db:"event_type"`
	Condition        json.RawMessage `json:"condition" db:"condition"`
	AgentID          string          `json:"agent_id" db:"agent_id"`
	PromptTemplate   string          `json:"prompt_template" db:"prompt_template"`
	Enabled          bool            `json:"enabled" db:"enabled"`
	RateLimitPerHour int             `json:"rate_limit_per_hour" db:"rate_limit_per_hour"`
	Schedule         string          `json:"schedule" db:"schedule"`
	RunAsUserID      *uuid.UUID      `json:"run_as_user_id" db:"run_as_user_id"`
	CreatedAt        time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at" db:"updated_at"`
}

// TriggerRuleStore handles database operations for trigger rules.
type TriggerRuleStore struct {
	pool *pgxpool.Pool
}

// NewTriggerRuleStore creates a new TriggerRuleStore.
func NewTriggerRuleStore(pool *pgxpool.Pool) *TriggerRuleStore {
	return &TriggerRuleStore{pool: pool}
}

// List returns all trigger rules for a workspace.
func (s *TriggerRuleStore) List(ctx context.Context, workspaceID uuid.UUID) ([]TriggerRule, error) {
	query := `
		SELECT id, workspace_id, name, event_type, condition, agent_id,
		       prompt_template, enabled, rate_limit_per_hour, schedule,
		       run_as_user_id, created_at, updated_at
		FROM trigger_rules
		WHERE workspace_id = $1
		ORDER BY created_at ASC`

	rows, err := s.pool.Query(ctx, query, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("listing trigger rules: %w", err)
	}
	defer rows.Close()

	var rules []TriggerRule
	for rows.Next() {
		var r TriggerRule
		if err := rows.Scan(
			&r.ID, &r.WorkspaceID, &r.Name, &r.EventType, &r.Condition,
			&r.AgentID, &r.PromptTemplate, &r.Enabled, &r.RateLimitPerHour,
			&r.Schedule, &r.RunAsUserID, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning trigger rule: %w", err)
		}
		rules = append(rules, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating trigger rules: %w", err)
	}

	return rules, nil
}

// GetByID retrieves a trigger rule by ID.
func (s *TriggerRuleStore) GetByID(ctx context.Context, id uuid.UUID) (*TriggerRule, error) {
	query := `
		SELECT id, workspace_id, name, event_type, condition, agent_id,
		       prompt_template, enabled, rate_limit_per_hour, schedule,
		       run_as_user_id, created_at, updated_at
		FROM trigger_rules WHERE id = $1`

	r := &TriggerRule{}
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&r.ID, &r.WorkspaceID, &r.Name, &r.EventType, &r.Condition,
		&r.AgentID, &r.PromptTemplate, &r.Enabled, &r.RateLimitPerHour,
		&r.Schedule, &r.RunAsUserID, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("trigger_rule", id.String())
		}
		return nil, fmt.Errorf("getting trigger rule by id: %w", err)
	}
	return r, nil
}

// Create inserts a new trigger rule.
func (s *TriggerRuleStore) Create(ctx context.Context, rule *TriggerRule) error {
	query := `
		INSERT INTO trigger_rules (id, workspace_id, name, event_type, condition, agent_id,
		       prompt_template, enabled, rate_limit_per_hour, schedule, run_as_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING created_at, updated_at`

	if rule.ID == uuid.Nil {
		rule.ID = uuid.New()
	}
	if rule.Condition == nil {
		rule.Condition = json.RawMessage(`{}`)
	}

	err := s.pool.QueryRow(ctx, query,
		rule.ID, rule.WorkspaceID, rule.Name, rule.EventType, rule.Condition,
		rule.AgentID, rule.PromptTemplate, rule.Enabled, rule.RateLimitPerHour,
		rule.Schedule, rule.RunAsUserID,
	).Scan(&rule.CreatedAt, &rule.UpdatedAt)
	if err != nil {
		return fmt.Errorf("creating trigger rule: %w", err)
	}
	return nil
}

// Update updates a trigger rule. Uses updated_at for optimistic concurrency.
func (s *TriggerRuleStore) Update(ctx context.Context, rule *TriggerRule) error {
	query := `
		UPDATE trigger_rules SET
			name = $2, event_type = $3, condition = $4, agent_id = $5,
			prompt_template = $6, enabled = $7, rate_limit_per_hour = $8,
			schedule = $9, run_as_user_id = $10, updated_at = now()
		WHERE id = $1 AND updated_at = $11
		RETURNING updated_at`

	err := s.pool.QueryRow(ctx, query,
		rule.ID, rule.Name, rule.EventType, rule.Condition, rule.AgentID,
		rule.PromptTemplate, rule.Enabled, rule.RateLimitPerHour,
		rule.Schedule, rule.RunAsUserID, rule.UpdatedAt,
	).Scan(&rule.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return errors.Conflict("trigger rule was modified by another request")
		}
		return fmt.Errorf("updating trigger rule: %w", err)
	}
	return nil
}

// Delete hard-deletes a trigger rule by ID.
func (s *TriggerRuleStore) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM trigger_rules WHERE id = $1`
	ct, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("deleting trigger rule: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return errors.NotFound("trigger_rule", id.String())
	}
	return nil
}
