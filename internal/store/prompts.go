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

// Prompt represents a prompt version for an agent.
type Prompt struct {
	ID           uuid.UUID       `json:"id" db:"id"`
	AgentID      string          `json:"agent_id" db:"agent_id"`
	Version      int             `json:"version" db:"version"`
	SystemPrompt string          `json:"system_prompt" db:"system_prompt"`
	TemplateVars json.RawMessage `json:"template_vars" db:"template_vars"`
	Mode         string          `json:"mode" db:"mode"`
	IsActive     bool            `json:"is_active" db:"is_active"`
	CreatedBy    string          `json:"created_by" db:"created_by"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
}

// PromptStore handles database operations for prompts.
type PromptStore struct {
	pool *pgxpool.Pool
}

// NewPromptStore creates a new PromptStore.
func NewPromptStore(pool *pgxpool.Pool) *PromptStore {
	return &PromptStore{pool: pool}
}

// List returns a paginated list of prompts for an agent, along with total count.
func (s *PromptStore) List(ctx context.Context, agentID string, activeOnly bool, offset, limit int) ([]Prompt, int, error) {
	where := "WHERE agent_id = $1"
	args := []interface{}{agentID}
	argIdx := 2

	if activeOnly {
		where += " AND is_active = true"
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM prompts %s", where)
	var total int
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting prompts: %w", err)
	}

	dataQuery := fmt.Sprintf(`
		SELECT id, agent_id, version, system_prompt, template_vars, mode, is_active, created_by, created_at
		FROM prompts %s
		ORDER BY version DESC
		LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing prompts: %w", err)
	}
	defer rows.Close()

	var prompts []Prompt
	for rows.Next() {
		var p Prompt
		if err := rows.Scan(
			&p.ID, &p.AgentID, &p.Version, &p.SystemPrompt,
			&p.TemplateVars, &p.Mode, &p.IsActive, &p.CreatedBy, &p.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning prompt: %w", err)
		}
		prompts = append(prompts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating prompts: %w", err)
	}

	return prompts, total, nil
}

// GetActive returns the currently active prompt for an agent.
func (s *PromptStore) GetActive(ctx context.Context, agentID string) (*Prompt, error) {
	query := `
		SELECT id, agent_id, version, system_prompt, template_vars, mode, is_active, created_by, created_at
		FROM prompts
		WHERE agent_id = $1 AND is_active = true
		LIMIT 1`

	p := &Prompt{}
	err := s.pool.QueryRow(ctx, query, agentID).Scan(
		&p.ID, &p.AgentID, &p.Version, &p.SystemPrompt,
		&p.TemplateVars, &p.Mode, &p.IsActive, &p.CreatedBy, &p.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("prompt", fmt.Sprintf("active for agent '%s'", agentID))
		}
		return nil, fmt.Errorf("getting active prompt: %w", err)
	}
	return p, nil
}

// GetByID retrieves a prompt by UUID.
func (s *PromptStore) GetByID(ctx context.Context, id uuid.UUID) (*Prompt, error) {
	query := `
		SELECT id, agent_id, version, system_prompt, template_vars, mode, is_active, created_by, created_at
		FROM prompts
		WHERE id = $1`

	p := &Prompt{}
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&p.ID, &p.AgentID, &p.Version, &p.SystemPrompt,
		&p.TemplateVars, &p.Mode, &p.IsActive, &p.CreatedBy, &p.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("prompt", id.String())
		}
		return nil, fmt.Errorf("getting prompt by id: %w", err)
	}
	return p, nil
}

// Create inserts a new prompt, deactivating any previously active prompt for the same agent.
// The version is auto-assigned as max(version)+1. This is transactional.
func (s *PromptStore) Create(ctx context.Context, prompt *Prompt) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Find the current max version for this agent
	var maxVersion int
	err = tx.QueryRow(ctx,
		"SELECT COALESCE(MAX(version), 0) FROM prompts WHERE agent_id = $1",
		prompt.AgentID,
	).Scan(&maxVersion)
	if err != nil {
		return fmt.Errorf("finding max version: %w", err)
	}

	// Deactivate all currently active prompts for this agent
	_, err = tx.Exec(ctx,
		"UPDATE prompts SET is_active = false WHERE agent_id = $1 AND is_active = true",
		prompt.AgentID,
	)
	if err != nil {
		return fmt.Errorf("deactivating previous prompts: %w", err)
	}

	// Insert new prompt
	prompt.Version = maxVersion + 1
	prompt.IsActive = true

	insertQuery := `
		INSERT INTO prompts (agent_id, version, system_prompt, template_vars, mode, is_active, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at`

	err = tx.QueryRow(ctx, insertQuery,
		prompt.AgentID, prompt.Version, prompt.SystemPrompt,
		prompt.TemplateVars, prompt.Mode, prompt.IsActive, prompt.CreatedBy,
	).Scan(&prompt.ID, &prompt.CreatedAt)
	if err != nil {
		return fmt.Errorf("inserting prompt: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}

// Activate activates a specific prompt, deactivating the currently active prompt for the same agent.
func (s *PromptStore) Activate(ctx context.Context, id uuid.UUID) (*Prompt, error) {
	// Get the target prompt
	prompt, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Deactivate all prompts for this agent
	_, err = tx.Exec(ctx,
		"UPDATE prompts SET is_active = false WHERE agent_id = $1 AND is_active = true",
		prompt.AgentID,
	)
	if err != nil {
		return nil, fmt.Errorf("deactivating current prompt: %w", err)
	}

	// Activate the target prompt
	_, err = tx.Exec(ctx,
		"UPDATE prompts SET is_active = true WHERE id = $1",
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("activating prompt: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	prompt.IsActive = true
	return prompt, nil
}

// Rollback creates a new prompt version with the content of the target version, activating it.
func (s *PromptStore) Rollback(ctx context.Context, agentID string, targetVersion int, actor string) (*Prompt, error) {
	// Get the target version
	targetQuery := `
		SELECT system_prompt, template_vars, mode
		FROM prompts
		WHERE agent_id = $1 AND version = $2`

	var systemPrompt string
	var templateVars json.RawMessage
	var mode string

	err := s.pool.QueryRow(ctx, targetQuery, agentID, targetVersion).Scan(
		&systemPrompt, &templateVars, &mode,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("prompt_version", fmt.Sprintf("%s/v%d", agentID, targetVersion))
		}
		return nil, fmt.Errorf("getting target prompt version: %w", err)
	}

	// Create a new prompt with the target's content
	newPrompt := &Prompt{
		AgentID:      agentID,
		SystemPrompt: systemPrompt,
		TemplateVars: templateVars,
		Mode:         mode,
		CreatedBy:    actor,
	}

	if err := s.Create(ctx, newPrompt); err != nil {
		return nil, fmt.Errorf("creating rollback prompt: %w", err)
	}

	return newPrompt, nil
}

// GetByVersion retrieves a prompt by agent ID and version number.
func (s *PromptStore) GetByVersion(ctx context.Context, agentID string, version int) (*Prompt, error) {
	query := `
		SELECT id, agent_id, version, system_prompt, template_vars, mode, is_active, created_by, created_at
		FROM prompts
		WHERE agent_id = $1 AND version = $2`

	p := &Prompt{}
	err := s.pool.QueryRow(ctx, query, agentID, version).Scan(
		&p.ID, &p.AgentID, &p.Version, &p.SystemPrompt,
		&p.TemplateVars, &p.Mode, &p.IsActive, &p.CreatedBy, &p.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("prompt", fmt.Sprintf("%s/v%d", agentID, version))
		}
		return nil, fmt.Errorf("getting prompt by version: %w", err)
	}
	return p, nil
}
