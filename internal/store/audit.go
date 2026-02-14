package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditEntry represents an entry in the audit log.
type AuditEntry struct {
	ID           int64           `json:"id" db:"id"`
	Actor        string          `json:"actor" db:"actor"`
	ActorID      *uuid.UUID      `json:"actor_id" db:"actor_id"`
	Action       string          `json:"action" db:"action"`
	ResourceType string          `json:"resource_type" db:"resource_type"`
	ResourceID   string          `json:"resource_id" db:"resource_id"`
	Details      json.RawMessage `json:"details" db:"details"`
	IPAddress    string          `json:"ip_address" db:"ip_address"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
}

// AuditFilter defines filter criteria for querying the audit log.
type AuditFilter struct {
	Actor        string
	Action       string
	ResourceType string
	ResourceID   string
	Offset       int
	Limit        int
}

// AuditStore handles database operations for the audit log.
type AuditStore struct {
	pool *pgxpool.Pool
}

// NewAuditStore creates a new AuditStore.
func NewAuditStore(pool *pgxpool.Pool) *AuditStore {
	return &AuditStore{pool: pool}
}

// Insert adds a new entry to the audit log.
func (s *AuditStore) Insert(ctx context.Context, entry *AuditEntry) error {
	if entry.Details == nil {
		entry.Details = json.RawMessage("{}")
	}

	query := `
		INSERT INTO audit_log (actor, actor_id, action, resource_type, resource_id, details, ip_address)
		VALUES ($1, $2, $3, $4, $5, $6, $7::inet)
		RETURNING id, created_at`

	err := s.pool.QueryRow(ctx, query,
		entry.Actor, entry.ActorID, entry.Action, entry.ResourceType,
		entry.ResourceID, entry.Details, entry.IPAddress,
	).Scan(&entry.ID, &entry.CreatedAt)
	if err != nil {
		return fmt.Errorf("inserting audit entry: %w", err)
	}
	return nil
}

// List returns paginated audit log entries matching the filter, along with total count.
func (s *AuditStore) List(ctx context.Context, filter AuditFilter) ([]AuditEntry, int, error) {
	// Build WHERE conditions
	where := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if filter.Actor != "" {
		where += fmt.Sprintf(" AND actor = $%d", argIdx)
		args = append(args, filter.Actor)
		argIdx++
	}
	if filter.Action != "" {
		where += fmt.Sprintf(" AND action = $%d", argIdx)
		args = append(args, filter.Action)
		argIdx++
	}
	if filter.ResourceType != "" {
		where += fmt.Sprintf(" AND resource_type = $%d", argIdx)
		args = append(args, filter.ResourceType)
		argIdx++
	}
	if filter.ResourceID != "" {
		where += fmt.Sprintf(" AND resource_id = $%d", argIdx)
		args = append(args, filter.ResourceID)
		argIdx++
	}

	// Count query
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM audit_log %s", where)
	var total int
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting audit entries: %w", err)
	}

	// Data query with pagination
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	dataQuery := fmt.Sprintf(`
		SELECT id, actor, actor_id, action, resource_type, resource_id, details, ip_address, created_at
		FROM audit_log %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)

	args = append(args, limit, filter.Offset)

	rows, err := s.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing audit entries: %w", err)
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(
			&e.ID, &e.Actor, &e.ActorID, &e.Action, &e.ResourceType,
			&e.ResourceID, &e.Details, &e.IPAddress, &e.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning audit entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating audit entries: %w", err)
	}

	return entries, total, nil
}
