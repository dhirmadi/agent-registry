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

// Agent represents an agent in the registry.
type Agent struct {
	ID             string          `json:"id" db:"id"`
	Name           string          `json:"name" db:"name"`
	Description    string          `json:"description" db:"description"`
	SystemPrompt   string          `json:"system_prompt" db:"system_prompt"`
	Tools          json.RawMessage `json:"tools" db:"tools"`
	TrustOverrides json.RawMessage `json:"trust_overrides" db:"trust_overrides"`
	ExamplePrompts json.RawMessage `json:"example_prompts" db:"example_prompts"`
	IsActive       bool            `json:"is_active" db:"is_active"`
	Version        int             `json:"version" db:"version"`
	CreatedBy      string          `json:"created_by" db:"created_by"`
	CreatedAt      time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at" db:"updated_at"`
}

// AgentVersion represents an immutable snapshot of an agent at a point in time.
type AgentVersion struct {
	ID             uuid.UUID       `json:"id" db:"id"`
	AgentID        string          `json:"agent_id" db:"agent_id"`
	Version        int             `json:"version" db:"version"`
	Name           string          `json:"name" db:"name"`
	Description    string          `json:"description" db:"description"`
	SystemPrompt   string          `json:"system_prompt" db:"system_prompt"`
	Tools          json.RawMessage `json:"tools" db:"tools"`
	TrustOverrides json.RawMessage `json:"trust_overrides" db:"trust_overrides"`
	ExamplePrompts json.RawMessage `json:"example_prompts" db:"example_prompts"`
	IsActive       bool            `json:"is_active" db:"is_active"`
	CreatedBy      string          `json:"created_by" db:"created_by"`
	CreatedAt      time.Time       `json:"created_at" db:"created_at"`
}

// AgentStore handles database operations for agents.
type AgentStore struct {
	pool *pgxpool.Pool
}

// NewAgentStore creates a new AgentStore.
func NewAgentStore(pool *pgxpool.Pool) *AgentStore {
	return &AgentStore{pool: pool}
}

// Create inserts a new agent and creates version 1 snapshot in a transaction.
func (s *AgentStore) Create(ctx context.Context, agent *Agent) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	agentQuery := `
		INSERT INTO agents (id, name, description, system_prompt, tools, trust_overrides, example_prompts, is_active, version, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1, $9)
		RETURNING created_at, updated_at`

	err = tx.QueryRow(ctx, agentQuery,
		agent.ID, agent.Name, agent.Description, agent.SystemPrompt,
		agent.Tools, agent.TrustOverrides, agent.ExamplePrompts,
		agent.IsActive, agent.CreatedBy,
	).Scan(&agent.CreatedAt, &agent.UpdatedAt)
	if err != nil {
		if isDuplicateKeyError(err) {
			return errors.Conflict(fmt.Sprintf("agent '%s' already exists", agent.ID))
		}
		return fmt.Errorf("inserting agent: %w", err)
	}
	agent.Version = 1

	versionQuery := `
		INSERT INTO agent_versions (agent_id, version, name, description, system_prompt, tools, trust_overrides, example_prompts, is_active, created_by)
		VALUES ($1, 1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err = tx.Exec(ctx, versionQuery,
		agent.ID, agent.Name, agent.Description, agent.SystemPrompt,
		agent.Tools, agent.TrustOverrides, agent.ExamplePrompts,
		agent.IsActive, agent.CreatedBy,
	)
	if err != nil {
		return fmt.Errorf("inserting agent version: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}

// GetByID retrieves an agent by ID.
func (s *AgentStore) GetByID(ctx context.Context, id string) (*Agent, error) {
	query := `
		SELECT id, name, description, system_prompt, tools, trust_overrides, example_prompts,
		       is_active, version, created_by, created_at, updated_at
		FROM agents WHERE id = $1`

	agent := &Agent{}
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&agent.ID, &agent.Name, &agent.Description, &agent.SystemPrompt,
		&agent.Tools, &agent.TrustOverrides, &agent.ExamplePrompts,
		&agent.IsActive, &agent.Version, &agent.CreatedBy,
		&agent.CreatedAt, &agent.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("agent", id)
		}
		return nil, fmt.Errorf("getting agent by id: %w", err)
	}
	return agent, nil
}

// List returns a paginated list of agents and the total count.
func (s *AgentStore) List(ctx context.Context, activeOnly bool, offset, limit int) ([]Agent, int, error) {
	where := ""
	args := []interface{}{}
	argIdx := 1

	if activeOnly {
		where = " WHERE is_active = true"
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM agents%s", where)
	var total int
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting agents: %w", err)
	}

	dataQuery := fmt.Sprintf(`
		SELECT id, name, description, system_prompt, tools, trust_overrides, example_prompts,
		       is_active, version, created_by, created_at, updated_at
		FROM agents%s
		ORDER BY created_at ASC
		LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing agents: %w", err)
	}
	defer rows.Close()

	var agents []Agent
	for rows.Next() {
		var a Agent
		if err := rows.Scan(
			&a.ID, &a.Name, &a.Description, &a.SystemPrompt,
			&a.Tools, &a.TrustOverrides, &a.ExamplePrompts,
			&a.IsActive, &a.Version, &a.CreatedBy,
			&a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning agent: %w", err)
		}
		agents = append(agents, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating agents: %w", err)
	}

	return agents, total, nil
}

// Update performs a full update of an agent with optimistic concurrency and creates a new version snapshot.
func (s *AgentStore) Update(ctx context.Context, agent *Agent, updatedAt time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	updateQuery := `
		UPDATE agents SET
			name = $2, description = $3, system_prompt = $4,
			tools = $5, trust_overrides = $6, example_prompts = $7,
			is_active = $8, version = version + 1, updated_at = now()
		WHERE id = $1 AND updated_at = $9
		RETURNING version, updated_at`

	err = tx.QueryRow(ctx, updateQuery,
		agent.ID, agent.Name, agent.Description, agent.SystemPrompt,
		agent.Tools, agent.TrustOverrides, agent.ExamplePrompts,
		agent.IsActive, updatedAt,
	).Scan(&agent.Version, &agent.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return errors.Conflict("resource was modified by another client")
		}
		return fmt.Errorf("updating agent: %w", err)
	}

	versionQuery := `
		INSERT INTO agent_versions (agent_id, version, name, description, system_prompt, tools, trust_overrides, example_prompts, is_active, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	_, err = tx.Exec(ctx, versionQuery,
		agent.ID, agent.Version, agent.Name, agent.Description, agent.SystemPrompt,
		agent.Tools, agent.TrustOverrides, agent.ExamplePrompts,
		agent.IsActive, agent.CreatedBy,
	)
	if err != nil {
		return fmt.Errorf("inserting agent version: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}

// Patch performs a partial update. Fields in the map are applied; unspecified fields are left unchanged.
func (s *AgentStore) Patch(ctx context.Context, id string, fields map[string]interface{}, updatedAt time.Time, actor string) (*Agent, error) {
	// First, get the current agent
	agent, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Apply patch fields
	if v, ok := fields["name"]; ok {
		agent.Name = v.(string)
	}
	if v, ok := fields["description"]; ok {
		agent.Description = v.(string)
	}
	if v, ok := fields["system_prompt"]; ok {
		agent.SystemPrompt = v.(string)
	}
	if v, ok := fields["tools"]; ok {
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshaling tools: %w", err)
		}
		agent.Tools = raw
	}
	if v, ok := fields["trust_overrides"]; ok {
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshaling trust_overrides: %w", err)
		}
		agent.TrustOverrides = raw
	}
	if v, ok := fields["example_prompts"]; ok {
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshaling example_prompts: %w", err)
		}
		agent.ExamplePrompts = raw
	}
	if v, ok := fields["is_active"]; ok {
		agent.IsActive = v.(bool)
	}

	agent.CreatedBy = actor

	if err := s.Update(ctx, agent, updatedAt); err != nil {
		return nil, err
	}
	return agent, nil
}

// Delete performs a soft-delete by setting is_active = false.
func (s *AgentStore) Delete(ctx context.Context, id string) error {
	query := `UPDATE agents SET is_active = false, updated_at = now() WHERE id = $1`
	ct, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("soft-deleting agent: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return errors.NotFound("agent", id)
	}
	return nil
}

// ListVersions returns a paginated list of agent version snapshots.
func (s *AgentStore) ListVersions(ctx context.Context, agentID string, offset, limit int) ([]AgentVersion, int, error) {
	countQuery := `SELECT COUNT(*) FROM agent_versions WHERE agent_id = $1`
	var total int
	if err := s.pool.QueryRow(ctx, countQuery, agentID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting agent versions: %w", err)
	}

	query := `
		SELECT id, agent_id, version, name, description, system_prompt, tools, trust_overrides, example_prompts, is_active, created_by, created_at
		FROM agent_versions
		WHERE agent_id = $1
		ORDER BY version DESC
		LIMIT $2 OFFSET $3`

	rows, err := s.pool.Query(ctx, query, agentID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing agent versions: %w", err)
	}
	defer rows.Close()

	var versions []AgentVersion
	for rows.Next() {
		var v AgentVersion
		if err := rows.Scan(
			&v.ID, &v.AgentID, &v.Version, &v.Name, &v.Description,
			&v.SystemPrompt, &v.Tools, &v.TrustOverrides, &v.ExamplePrompts,
			&v.IsActive, &v.CreatedBy, &v.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning agent version: %w", err)
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating agent versions: %w", err)
	}

	return versions, total, nil
}

// GetVersion retrieves a specific agent version.
func (s *AgentStore) GetVersion(ctx context.Context, agentID string, version int) (*AgentVersion, error) {
	query := `
		SELECT id, agent_id, version, name, description, system_prompt, tools, trust_overrides, example_prompts, is_active, created_by, created_at
		FROM agent_versions
		WHERE agent_id = $1 AND version = $2`

	v := &AgentVersion{}
	err := s.pool.QueryRow(ctx, query, agentID, version).Scan(
		&v.ID, &v.AgentID, &v.Version, &v.Name, &v.Description,
		&v.SystemPrompt, &v.Tools, &v.TrustOverrides, &v.ExamplePrompts,
		&v.IsActive, &v.CreatedBy, &v.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("agent_version", fmt.Sprintf("%s/v%d", agentID, version))
		}
		return nil, fmt.Errorf("getting agent version: %w", err)
	}
	return v, nil
}

// Rollback copies data from a target version to create a new current version.
func (s *AgentStore) Rollback(ctx context.Context, agentID string, targetVersion int, actor string) (*Agent, error) {
	// Get the target version data
	target, err := s.GetVersion(ctx, agentID, targetVersion)
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Update agents table with target version data, increment version
	updateQuery := `
		UPDATE agents SET
			name = $2, description = $3, system_prompt = $4,
			tools = $5, trust_overrides = $6, example_prompts = $7,
			is_active = $8, version = version + 1, updated_at = now()
		WHERE id = $1
		RETURNING version, updated_at`

	agent := &Agent{
		ID:             agentID,
		Name:           target.Name,
		Description:    target.Description,
		SystemPrompt:   target.SystemPrompt,
		Tools:          target.Tools,
		TrustOverrides: target.TrustOverrides,
		ExamplePrompts: target.ExamplePrompts,
		IsActive:       target.IsActive,
		CreatedBy:      actor,
	}

	err = tx.QueryRow(ctx, updateQuery,
		agentID, agent.Name, agent.Description, agent.SystemPrompt,
		agent.Tools, agent.TrustOverrides, agent.ExamplePrompts,
		agent.IsActive,
	).Scan(&agent.Version, &agent.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("agent", agentID)
		}
		return nil, fmt.Errorf("updating agent for rollback: %w", err)
	}

	// Insert new version snapshot
	versionQuery := `
		INSERT INTO agent_versions (agent_id, version, name, description, system_prompt, tools, trust_overrides, example_prompts, is_active, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	_, err = tx.Exec(ctx, versionQuery,
		agentID, agent.Version, agent.Name, agent.Description, agent.SystemPrompt,
		agent.Tools, agent.TrustOverrides, agent.ExamplePrompts,
		agent.IsActive, actor,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting rollback version: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing rollback: %w", err)
	}

	// Get the full agent to return (with created_at)
	return s.GetByID(ctx, agentID)
}

// isDuplicateKeyError checks if a pgx error is a unique constraint violation.
func isDuplicateKeyError(err error) bool {
	return err != nil && (fmt.Sprintf("%v", err) != "" &&
		(contains(err.Error(), "duplicate key") || contains(err.Error(), "23505")))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
