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

// Session represents a server-side session stored in PostgreSQL.
type Session struct {
	ID        string    `json:"id" db:"id"`
	UserID    uuid.UUID `json:"user_id" db:"user_id"`
	CSRFToken string    `json:"-" db:"csrf_token"`
	IPAddress string    `json:"ip_address" db:"ip_address"`
	UserAgent string    `json:"user_agent" db:"user_agent"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	LastSeen  time.Time `json:"last_seen" db:"last_seen"`
	ExpiresAt time.Time `json:"expires_at" db:"expires_at"`
}

// SessionStore handles database operations for sessions.
type SessionStore struct {
	pool *pgxpool.Pool
}

// NewSessionStore creates a new SessionStore.
func NewSessionStore(pool *pgxpool.Pool) *SessionStore {
	return &SessionStore{pool: pool}
}

// Create inserts a new session.
func (s *SessionStore) Create(ctx context.Context, sess *Session) error {
	query := `
		INSERT INTO sessions (id, user_id, csrf_token, ip_address, user_agent, expires_at)
		VALUES ($1, $2, $3, $4::inet, $5, $6)
		RETURNING created_at, last_seen`

	err := s.pool.QueryRow(ctx, query,
		sess.ID, sess.UserID, sess.CSRFToken, sess.IPAddress, sess.UserAgent, sess.ExpiresAt,
	).Scan(&sess.CreatedAt, &sess.LastSeen)
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	return nil
}

// GetByID retrieves a session by ID. Returns nil if the session is expired or idle-timed-out.
func (s *SessionStore) GetByID(ctx context.Context, id string) (*Session, error) {
	query := `
		SELECT id, user_id, csrf_token, ip_address, user_agent, created_at, last_seen, expires_at
		FROM sessions
		WHERE id = $1
		  AND expires_at > now()
		  AND last_seen > now() - interval '30 minutes'`

	sess := &Session{}
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&sess.ID, &sess.UserID, &sess.CSRFToken, &sess.IPAddress,
		&sess.UserAgent, &sess.CreatedAt, &sess.LastSeen, &sess.ExpiresAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("session", id)
		}
		return nil, fmt.Errorf("getting session: %w", err)
	}
	return sess, nil
}

// UpdateLastSeen updates the last_seen timestamp for a session (sliding window).
func (s *SessionStore) UpdateLastSeen(ctx context.Context, id string) error {
	query := `UPDATE sessions SET last_seen = now() WHERE id = $1`
	ct, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("updating session last_seen: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return errors.NotFound("session", id)
	}
	return nil
}

// Delete removes a session by ID (used on logout).
func (s *SessionStore) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM sessions WHERE id = $1`
	_, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}
	return nil
}

// DeleteByUserID removes all sessions for a given user (e.g., on password change).
func (s *SessionStore) DeleteByUserID(ctx context.Context, userID uuid.UUID) error {
	query := `DELETE FROM sessions WHERE user_id = $1`
	_, err := s.pool.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("deleting sessions by user: %w", err)
	}
	return nil
}

// DeleteExpired removes expired or idle sessions. Returns the number of deleted rows.
func (s *SessionStore) DeleteExpired(ctx context.Context) (int64, error) {
	query := `
		DELETE FROM sessions
		WHERE expires_at <= now()
		   OR last_seen <= now() - interval '30 minutes'`

	ct, err := s.pool.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("deleting expired sessions: %w", err)
	}
	return ct.RowsAffected(), nil
}
