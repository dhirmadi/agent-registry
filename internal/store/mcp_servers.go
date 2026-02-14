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

// MCPServer represents an MCP server configuration in the registry.
type MCPServer struct {
	ID                uuid.UUID       `json:"id" db:"id"`
	Label             string          `json:"label" db:"label"`
	Endpoint          string          `json:"endpoint" db:"endpoint"`
	AuthType          string          `json:"auth_type" db:"auth_type"`
	AuthCredential    string          `json:"-" db:"auth_credential"`
	HealthEndpoint    string          `json:"health_endpoint" db:"health_endpoint"`
	CircuitBreaker    json.RawMessage `json:"circuit_breaker" db:"circuit_breaker"`
	DiscoveryInterval string          `json:"discovery_interval" db:"discovery_interval"`
	IsEnabled         bool            `json:"is_enabled" db:"is_enabled"`
	CreatedAt         time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at" db:"updated_at"`
}

// MCPServerStore handles database operations for MCP servers.
type MCPServerStore struct {
	pool *pgxpool.Pool
}

// NewMCPServerStore creates a new MCPServerStore.
func NewMCPServerStore(pool *pgxpool.Pool) *MCPServerStore {
	return &MCPServerStore{pool: pool}
}

// Create inserts a new MCP server. auth_credential should already be encrypted.
func (s *MCPServerStore) Create(ctx context.Context, server *MCPServer) error {
	query := `
		INSERT INTO mcp_servers (id, label, endpoint, auth_type, auth_credential, health_endpoint, circuit_breaker, discovery_interval, is_enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at, updated_at`

	if server.ID == uuid.Nil {
		server.ID = uuid.New()
	}
	if server.CircuitBreaker == nil {
		server.CircuitBreaker = json.RawMessage(`{"fail_threshold": 5, "open_duration_s": 30}`)
	}

	err := s.pool.QueryRow(ctx, query,
		server.ID, server.Label, server.Endpoint, server.AuthType,
		server.AuthCredential, server.HealthEndpoint, server.CircuitBreaker,
		server.DiscoveryInterval, server.IsEnabled,
	).Scan(&server.CreatedAt, &server.UpdatedAt)
	if err != nil {
		return fmt.Errorf("creating mcp server: %w", err)
	}
	return nil
}

// GetByID retrieves an MCP server by ID.
func (s *MCPServerStore) GetByID(ctx context.Context, id uuid.UUID) (*MCPServer, error) {
	query := `
		SELECT id, label, endpoint, auth_type, auth_credential, health_endpoint,
		       circuit_breaker, discovery_interval, is_enabled, created_at, updated_at
		FROM mcp_servers WHERE id = $1`

	server := &MCPServer{}
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&server.ID, &server.Label, &server.Endpoint, &server.AuthType,
		&server.AuthCredential, &server.HealthEndpoint, &server.CircuitBreaker,
		&server.DiscoveryInterval, &server.IsEnabled, &server.CreatedAt, &server.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("mcp_server", id.String())
		}
		return nil, fmt.Errorf("getting mcp server by id: %w", err)
	}
	return server, nil
}

// GetByLabel retrieves an MCP server by label.
func (s *MCPServerStore) GetByLabel(ctx context.Context, label string) (*MCPServer, error) {
	query := `
		SELECT id, label, endpoint, auth_type, auth_credential, health_endpoint,
		       circuit_breaker, discovery_interval, is_enabled, created_at, updated_at
		FROM mcp_servers WHERE label = $1`

	server := &MCPServer{}
	err := s.pool.QueryRow(ctx, query, label).Scan(
		&server.ID, &server.Label, &server.Endpoint, &server.AuthType,
		&server.AuthCredential, &server.HealthEndpoint, &server.CircuitBreaker,
		&server.DiscoveryInterval, &server.IsEnabled, &server.CreatedAt, &server.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("mcp_server", label)
		}
		return nil, fmt.Errorf("getting mcp server by label: %w", err)
	}
	return server, nil
}

// List returns all MCP servers ordered by label.
func (s *MCPServerStore) List(ctx context.Context) ([]MCPServer, error) {
	query := `
		SELECT id, label, endpoint, auth_type, auth_credential, health_endpoint,
		       circuit_breaker, discovery_interval, is_enabled, created_at, updated_at
		FROM mcp_servers
		ORDER BY label ASC`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing mcp servers: %w", err)
	}
	defer rows.Close()

	var servers []MCPServer
	for rows.Next() {
		var srv MCPServer
		if err := rows.Scan(
			&srv.ID, &srv.Label, &srv.Endpoint, &srv.AuthType,
			&srv.AuthCredential, &srv.HealthEndpoint, &srv.CircuitBreaker,
			&srv.DiscoveryInterval, &srv.IsEnabled, &srv.CreatedAt, &srv.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning mcp server: %w", err)
		}
		servers = append(servers, srv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating mcp servers: %w", err)
	}

	return servers, nil
}

// Update updates an MCP server. Uses updated_at for optimistic concurrency.
func (s *MCPServerStore) Update(ctx context.Context, server *MCPServer) error {
	query := `
		UPDATE mcp_servers SET
			label = $2, endpoint = $3, auth_type = $4, auth_credential = $5,
			health_endpoint = $6, circuit_breaker = $7, discovery_interval = $8,
			is_enabled = $9, updated_at = now()
		WHERE id = $1 AND updated_at = $10
		RETURNING updated_at`

	err := s.pool.QueryRow(ctx, query,
		server.ID, server.Label, server.Endpoint, server.AuthType,
		server.AuthCredential, server.HealthEndpoint, server.CircuitBreaker,
		server.DiscoveryInterval, server.IsEnabled, server.UpdatedAt,
	).Scan(&server.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return errors.Conflict("mcp server was modified by another request")
		}
		return fmt.Errorf("updating mcp server: %w", err)
	}
	return nil
}

// Delete hard-deletes an MCP server by ID.
func (s *MCPServerStore) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM mcp_servers WHERE id = $1`
	ct, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("deleting mcp server: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return errors.NotFound("mcp_server", id.String())
	}
	return nil
}
