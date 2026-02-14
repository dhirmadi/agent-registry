package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agent-smit/agentic-registry/internal/errors"
)

// WebhookSubscription represents a webhook subscription.
type WebhookSubscription struct {
	ID        uuid.UUID       `json:"id"`
	URL       string          `json:"url"`
	Secret    string          `json:"-"`
	Events    json.RawMessage `json:"events"`
	IsActive  bool            `json:"is_active"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// WebhookStore handles database operations for webhook subscriptions.
type WebhookStore struct {
	pool *pgxpool.Pool
}

// NewWebhookStore creates a new WebhookStore.
func NewWebhookStore(pool *pgxpool.Pool) *WebhookStore {
	return &WebhookStore{pool: pool}
}

// Create inserts a new webhook subscription.
func (s *WebhookStore) Create(ctx context.Context, sub *WebhookSubscription) error {
	query := `
		INSERT INTO webhook_subscriptions (url, secret, events, is_active)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`

	err := s.pool.QueryRow(ctx, query, sub.URL, sub.Secret, sub.Events, sub.IsActive).
		Scan(&sub.ID, &sub.CreatedAt, &sub.UpdatedAt)
	if err != nil {
		return fmt.Errorf("creating webhook subscription: %w", err)
	}
	return nil
}

// List returns all webhook subscriptions (excluding secret).
func (s *WebhookStore) List(ctx context.Context) ([]WebhookSubscription, error) {
	query := `
		SELECT id, url, events, is_active, created_at, updated_at
		FROM webhook_subscriptions
		ORDER BY created_at ASC`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing webhook subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []WebhookSubscription
	for rows.Next() {
		var sub WebhookSubscription
		if err := rows.Scan(&sub.ID, &sub.URL, &sub.Events, &sub.IsActive, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning webhook subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating webhook subscriptions: %w", err)
	}

	return subs, nil
}

// Delete removes a webhook subscription by ID.
func (s *WebhookStore) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM webhook_subscriptions WHERE id = $1`

	tag, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("deleting webhook subscription: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.NotFound("webhook_subscription", id.String())
	}
	return nil
}

// ListActive returns all active webhook subscriptions including their secrets (for dispatcher use).
func (s *WebhookStore) ListActive(ctx context.Context) ([]WebhookSubscription, error) {
	query := `
		SELECT id, url, secret, events, is_active, created_at, updated_at
		FROM webhook_subscriptions
		WHERE is_active = true
		ORDER BY created_at ASC`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing active webhook subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []WebhookSubscription
	for rows.Next() {
		var sub WebhookSubscription
		if err := rows.Scan(&sub.ID, &sub.URL, &sub.Secret, &sub.Events, &sub.IsActive, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning active webhook subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating active webhook subscriptions: %w", err)
	}

	return subs, nil
}
