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

// OAuthConnection represents a linked OAuth provider account.
type OAuthConnection struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	UserID       uuid.UUID  `json:"user_id" db:"user_id"`
	Provider     string     `json:"provider" db:"provider"`
	ProviderUID  string     `json:"-" db:"provider_uid"`
	Email        string     `json:"email" db:"email"`
	DisplayName  string     `json:"display_name" db:"display_name"`
	AccessToken  string     `json:"-" db:"access_token"`
	RefreshToken string     `json:"-" db:"refresh_token"`
	ExpiresAt    *time.Time `json:"-" db:"expires_at"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
}

// OAuthConnectionStore handles database operations for OAuth connections.
type OAuthConnectionStore struct {
	pool *pgxpool.Pool
}

// NewOAuthConnectionStore creates a new OAuthConnectionStore.
func NewOAuthConnectionStore(pool *pgxpool.Pool) *OAuthConnectionStore {
	return &OAuthConnectionStore{pool: pool}
}

// Create inserts a new OAuth connection.
func (s *OAuthConnectionStore) Create(ctx context.Context, conn *OAuthConnection) error {
	query := `
		INSERT INTO oauth_connections (id, user_id, provider, provider_uid, email, display_name, access_token, refresh_token, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at`

	if conn.ID == uuid.Nil {
		conn.ID = uuid.New()
	}

	err := s.pool.QueryRow(ctx, query,
		conn.ID, conn.UserID, conn.Provider, conn.ProviderUID,
		conn.Email, conn.DisplayName, conn.AccessToken, conn.RefreshToken, conn.ExpiresAt,
	).Scan(&conn.CreatedAt)
	if err != nil {
		return fmt.Errorf("creating oauth connection: %w", err)
	}
	return nil
}

// GetByProviderUID retrieves an OAuth connection by provider and provider UID.
func (s *OAuthConnectionStore) GetByProviderUID(ctx context.Context, provider, providerUID string) (*OAuthConnection, error) {
	query := `
		SELECT id, user_id, provider, provider_uid, email, display_name, access_token, refresh_token, expires_at, created_at
		FROM oauth_connections
		WHERE provider = $1 AND provider_uid = $2`

	conn := &OAuthConnection{}
	err := s.pool.QueryRow(ctx, query, provider, providerUID).Scan(
		&conn.ID, &conn.UserID, &conn.Provider, &conn.ProviderUID,
		&conn.Email, &conn.DisplayName, &conn.AccessToken, &conn.RefreshToken,
		&conn.ExpiresAt, &conn.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("oauth_connection", providerUID)
		}
		return nil, fmt.Errorf("getting oauth connection by provider uid: %w", err)
	}
	return conn, nil
}

// GetByUserID retrieves all OAuth connections for a user.
func (s *OAuthConnectionStore) GetByUserID(ctx context.Context, userID uuid.UUID) ([]OAuthConnection, error) {
	query := `
		SELECT id, user_id, provider, provider_uid, email, display_name, access_token, refresh_token, expires_at, created_at
		FROM oauth_connections
		WHERE user_id = $1
		ORDER BY created_at ASC`

	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("listing oauth connections: %w", err)
	}
	defer rows.Close()

	var conns []OAuthConnection
	for rows.Next() {
		var c OAuthConnection
		if err := rows.Scan(
			&c.ID, &c.UserID, &c.Provider, &c.ProviderUID,
			&c.Email, &c.DisplayName, &c.AccessToken, &c.RefreshToken,
			&c.ExpiresAt, &c.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning oauth connection: %w", err)
		}
		conns = append(conns, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating oauth connections: %w", err)
	}

	return conns, nil
}

// DeleteByUserID removes all OAuth connections for a user.
func (s *OAuthConnectionStore) DeleteByUserID(ctx context.Context, userID uuid.UUID) error {
	query := `DELETE FROM oauth_connections WHERE user_id = $1`
	_, err := s.pool.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("deleting oauth connections by user: %w", err)
	}
	return nil
}
