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

// APIKey represents an API key stored in the database.
type APIKey struct {
	ID         uuid.UUID  `json:"id" db:"id"`
	UserID     *uuid.UUID `json:"user_id" db:"user_id"`
	Name       string     `json:"name" db:"name"`
	KeyPrefix  string     `json:"key_prefix" db:"key_prefix"`
	KeyHash    string     `json:"-" db:"key_hash"`
	Scopes     []string   `json:"scopes" db:"scopes"`
	IsActive   bool       `json:"is_active" db:"is_active"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at" db:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at" db:"last_used_at"`
}

// APIKeyStore handles database operations for API keys.
type APIKeyStore struct {
	pool *pgxpool.Pool
}

// NewAPIKeyStore creates a new APIKeyStore.
func NewAPIKeyStore(pool *pgxpool.Pool) *APIKeyStore {
	return &APIKeyStore{pool: pool}
}

// Create inserts a new API key.
func (s *APIKeyStore) Create(ctx context.Context, key *APIKey) error {
	query := `
		INSERT INTO api_keys (id, user_id, name, key_prefix, key_hash, scopes, is_active, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at`

	if key.ID == uuid.Nil {
		key.ID = uuid.New()
	}

	err := s.pool.QueryRow(ctx, query,
		key.ID, key.UserID, key.Name, key.KeyPrefix, key.KeyHash,
		key.Scopes, key.IsActive, key.ExpiresAt,
	).Scan(&key.CreatedAt)
	if err != nil {
		return fmt.Errorf("creating api key: %w", err)
	}
	return nil
}

// GetByHash retrieves an active, non-expired API key by its SHA-256 hash.
func (s *APIKeyStore) GetByHash(ctx context.Context, hash string) (*APIKey, error) {
	query := `
		SELECT id, user_id, name, key_prefix, key_hash, scopes, is_active, created_at, expires_at, last_used_at
		FROM api_keys
		WHERE key_hash = $1
		  AND is_active = true
		  AND (expires_at IS NULL OR expires_at > now())`

	key := &APIKey{}
	err := s.pool.QueryRow(ctx, query, hash).Scan(
		&key.ID, &key.UserID, &key.Name, &key.KeyPrefix, &key.KeyHash,
		&key.Scopes, &key.IsActive, &key.CreatedAt, &key.ExpiresAt, &key.LastUsedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("api_key", "")
		}
		return nil, fmt.Errorf("getting api key by hash: %w", err)
	}
	return key, nil
}

// List returns API keys, optionally filtered by user ID. Never returns the hash.
func (s *APIKeyStore) List(ctx context.Context, userID *uuid.UUID) ([]APIKey, error) {
	var query string
	var args []interface{}

	if userID != nil {
		query = `
			SELECT id, user_id, name, key_prefix, key_hash, scopes, is_active, created_at, expires_at, last_used_at
			FROM api_keys
			WHERE user_id = $1
			ORDER BY created_at DESC`
		args = append(args, *userID)
	} else {
		query = `
			SELECT id, user_id, name, key_prefix, key_hash, scopes, is_active, created_at, expires_at, last_used_at
			FROM api_keys
			ORDER BY created_at DESC`
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing api keys: %w", err)
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(
			&k.ID, &k.UserID, &k.Name, &k.KeyPrefix, &k.KeyHash,
			&k.Scopes, &k.IsActive, &k.CreatedAt, &k.ExpiresAt, &k.LastUsedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning api key: %w", err)
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating api keys: %w", err)
	}

	return keys, nil
}

// Delete soft-deletes an API key by setting is_active to false.
func (s *APIKeyStore) Delete(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE api_keys SET is_active = false WHERE id = $1`
	ct, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("deleting api key: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return errors.NotFound("api_key", id.String())
	}
	return nil
}

// GetByID retrieves an API key by its ID.
func (s *APIKeyStore) GetByID(ctx context.Context, id uuid.UUID) (*APIKey, error) {
	query := `
		SELECT id, user_id, name, key_prefix, key_hash, scopes, is_active, created_at, expires_at, last_used_at
		FROM api_keys
		WHERE id = $1`

	key := &APIKey{}
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&key.ID, &key.UserID, &key.Name, &key.KeyPrefix, &key.KeyHash,
		&key.Scopes, &key.IsActive, &key.CreatedAt, &key.ExpiresAt, &key.LastUsedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("api_key", id.String())
		}
		return nil, fmt.Errorf("getting api key by id: %w", err)
	}
	return key, nil
}

// UpdateLastUsed updates the last_used_at timestamp for an API key.
func (s *APIKeyStore) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE api_keys SET last_used_at = now() WHERE id = $1`
	_, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("updating api key last_used: %w", err)
	}
	return nil
}
