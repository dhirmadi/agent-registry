package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agent-smit/agentic-registry/internal/auth"
	"github.com/agent-smit/agentic-registry/internal/errors"
)

// ModelEndpoint represents a model provider endpoint in the registry.
type ModelEndpoint struct {
	ID            uuid.UUID       `json:"id"`
	Slug          string          `json:"slug"`
	Name          string          `json:"name"`
	Provider      string          `json:"provider"`
	EndpointURL   string          `json:"endpoint_url"`
	IsFixedModel  bool            `json:"is_fixed_model"`
	ModelName     string          `json:"model_name"`
	AllowedModels json.RawMessage `json:"allowed_models"`
	IsActive      bool            `json:"is_active"`
	WorkspaceID   *string         `json:"workspace_id,omitempty"`
	CreatedBy     string          `json:"created_by"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// ModelEndpointVersion represents a versioned configuration snapshot for a model endpoint.
type ModelEndpointVersion struct {
	ID         uuid.UUID       `json:"id"`
	EndpointID uuid.UUID       `json:"endpoint_id"`
	Version    int             `json:"version"`
	Config     json.RawMessage `json:"config"`
	IsActive   bool            `json:"is_active"`
	ChangeNote string          `json:"change_note"`
	CreatedBy  string          `json:"created_by"`
	CreatedAt  time.Time       `json:"created_at"`
}

// ModelEndpointStore handles database operations for model endpoints.
type ModelEndpointStore struct {
	pool   *pgxpool.Pool
	encKey []byte
}

// NewModelEndpointStore creates a new ModelEndpointStore.
func NewModelEndpointStore(pool *pgxpool.Pool, encKey []byte) *ModelEndpointStore {
	return &ModelEndpointStore{pool: pool, encKey: encKey}
}

// Create inserts a new model endpoint and creates version 1 in a transaction.
func (s *ModelEndpointStore) Create(ctx context.Context, endpoint *ModelEndpoint, initialConfig json.RawMessage, changeNote string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if endpoint.AllowedModels == nil {
		endpoint.AllowedModels = json.RawMessage(`[]`)
	}

	endpointQuery := `
		INSERT INTO model_endpoints (slug, name, provider, endpoint_url, is_fixed_model, model_name, allowed_models, is_active, workspace_id, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at, updated_at`

	err = tx.QueryRow(ctx, endpointQuery,
		endpoint.Slug, endpoint.Name, endpoint.Provider, endpoint.EndpointURL,
		endpoint.IsFixedModel, endpoint.ModelName, endpoint.AllowedModels,
		endpoint.IsActive, endpoint.WorkspaceID, endpoint.CreatedBy,
	).Scan(&endpoint.ID, &endpoint.CreatedAt, &endpoint.UpdatedAt)
	if err != nil {
		if isDuplicateKeyError(err) {
			return errors.Conflict(fmt.Sprintf("model endpoint '%s' already exists", endpoint.Slug))
		}
		return fmt.Errorf("inserting model endpoint: %w", err)
	}

	encryptedConfig, err := s.encryptConfigHeaders(initialConfig)
	if err != nil {
		return fmt.Errorf("encrypting config headers: %w", err)
	}

	versionQuery := `
		INSERT INTO model_endpoint_versions (endpoint_id, version, config, is_active, change_note, created_by)
		VALUES ($1, 1, $2, true, $3, $4)`

	_, err = tx.Exec(ctx, versionQuery,
		endpoint.ID, encryptedConfig, changeNote, endpoint.CreatedBy,
	)
	if err != nil {
		return fmt.Errorf("inserting initial version: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}

// GetBySlug retrieves a model endpoint by slug.
func (s *ModelEndpointStore) GetBySlug(ctx context.Context, slug string) (*ModelEndpoint, error) {
	query := `
		SELECT id, slug, name, provider, endpoint_url, is_fixed_model, model_name, allowed_models,
		       is_active, workspace_id, created_by, created_at, updated_at
		FROM model_endpoints WHERE slug = $1`

	ep := &ModelEndpoint{}
	err := s.pool.QueryRow(ctx, query, slug).Scan(
		&ep.ID, &ep.Slug, &ep.Name, &ep.Provider, &ep.EndpointURL,
		&ep.IsFixedModel, &ep.ModelName, &ep.AllowedModels,
		&ep.IsActive, &ep.WorkspaceID, &ep.CreatedBy,
		&ep.CreatedAt, &ep.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("model_endpoint", slug)
		}
		return nil, fmt.Errorf("getting model endpoint by slug: %w", err)
	}
	return ep, nil
}

// List returns a paginated list of model endpoints, optionally filtered by workspace.
func (s *ModelEndpointStore) List(ctx context.Context, workspaceID *string, activeOnly bool, offset, limit int) ([]ModelEndpoint, int, error) {
	where := ""
	args := []interface{}{}
	argIdx := 1

	var conditions []string
	if workspaceID != nil {
		conditions = append(conditions, fmt.Sprintf("workspace_id = $%d", argIdx))
		args = append(args, *workspaceID)
		argIdx++
	}
	if activeOnly {
		conditions = append(conditions, "is_active = true")
	}

	if len(conditions) > 0 {
		where = " WHERE "
		for i, c := range conditions {
			if i > 0 {
				where += " AND "
			}
			where += c
		}
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM model_endpoints%s", where)
	var total int
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting model endpoints: %w", err)
	}

	dataQuery := fmt.Sprintf(`
		SELECT id, slug, name, provider, endpoint_url, is_fixed_model, model_name, allowed_models,
		       is_active, workspace_id, created_by, created_at, updated_at
		FROM model_endpoints%s
		ORDER BY created_at ASC
		LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing model endpoints: %w", err)
	}
	defer rows.Close()

	var endpoints []ModelEndpoint
	for rows.Next() {
		var ep ModelEndpoint
		if err := rows.Scan(
			&ep.ID, &ep.Slug, &ep.Name, &ep.Provider, &ep.EndpointURL,
			&ep.IsFixedModel, &ep.ModelName, &ep.AllowedModels,
			&ep.IsActive, &ep.WorkspaceID, &ep.CreatedBy,
			&ep.CreatedAt, &ep.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning model endpoint: %w", err)
		}
		endpoints = append(endpoints, ep)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating model endpoints: %w", err)
	}

	return endpoints, total, nil
}

// Update performs a full update of a model endpoint with optimistic concurrency.
func (s *ModelEndpointStore) Update(ctx context.Context, endpoint *ModelEndpoint, updatedAt time.Time) error {
	if endpoint.AllowedModels == nil {
		endpoint.AllowedModels = json.RawMessage(`[]`)
	}

	query := `
		UPDATE model_endpoints SET
			name = $2, provider = $3, endpoint_url = $4,
			is_fixed_model = $5, model_name = $6, allowed_models = $7,
			is_active = $8, workspace_id = $9, updated_at = now()
		WHERE slug = $1 AND updated_at = $10
		RETURNING id, updated_at`

	err := s.pool.QueryRow(ctx, query,
		endpoint.Slug, endpoint.Name, endpoint.Provider, endpoint.EndpointURL,
		endpoint.IsFixedModel, endpoint.ModelName, endpoint.AllowedModels,
		endpoint.IsActive, endpoint.WorkspaceID, updatedAt,
	).Scan(&endpoint.ID, &endpoint.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return errors.Conflict("resource was modified by another client")
		}
		return fmt.Errorf("updating model endpoint: %w", err)
	}
	return nil
}

// Delete performs a soft-delete by setting is_active = false.
func (s *ModelEndpointStore) Delete(ctx context.Context, slug string) error {
	query := `UPDATE model_endpoints SET is_active = false, updated_at = now() WHERE slug = $1`
	ct, err := s.pool.Exec(ctx, query, slug)
	if err != nil {
		return fmt.Errorf("soft-deleting model endpoint: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return errors.NotFound("model_endpoint", slug)
	}
	return nil
}

// CreateVersion inserts a new version for a model endpoint, auto-incrementing the version number.
// The new version is NOT automatically activated.
func (s *ModelEndpointStore) CreateVersion(ctx context.Context, endpointID uuid.UUID, config json.RawMessage, changeNote, createdBy string) (*ModelEndpointVersion, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var maxVersion int
	err = tx.QueryRow(ctx,
		"SELECT COALESCE(MAX(version), 0) FROM model_endpoint_versions WHERE endpoint_id = $1",
		endpointID,
	).Scan(&maxVersion)
	if err != nil {
		return nil, fmt.Errorf("finding max version: %w", err)
	}

	encryptedConfig, err := s.encryptConfigHeaders(config)
	if err != nil {
		return nil, fmt.Errorf("encrypting config headers: %w", err)
	}

	version := &ModelEndpointVersion{
		EndpointID: endpointID,
		Version:    maxVersion + 1,
		Config:     encryptedConfig,
		IsActive:   false,
		ChangeNote: changeNote,
		CreatedBy:  createdBy,
	}

	insertQuery := `
		INSERT INTO model_endpoint_versions (endpoint_id, version, config, is_active, change_note, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at`

	err = tx.QueryRow(ctx, insertQuery,
		version.EndpointID, version.Version, version.Config,
		version.IsActive, version.ChangeNote, version.CreatedBy,
	).Scan(&version.ID, &version.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("inserting model endpoint version: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	// Decrypt for the returned version
	version.Config, _ = s.decryptConfigHeaders(version.Config)
	return version, nil
}

// ListVersions returns a paginated list of versions for a model endpoint.
func (s *ModelEndpointStore) ListVersions(ctx context.Context, endpointID uuid.UUID, offset, limit int) ([]ModelEndpointVersion, int, error) {
	countQuery := `SELECT COUNT(*) FROM model_endpoint_versions WHERE endpoint_id = $1`
	var total int
	if err := s.pool.QueryRow(ctx, countQuery, endpointID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting model endpoint versions: %w", err)
	}

	query := `
		SELECT id, endpoint_id, version, config, is_active, change_note, created_by, created_at
		FROM model_endpoint_versions
		WHERE endpoint_id = $1
		ORDER BY version DESC
		LIMIT $2 OFFSET $3`

	rows, err := s.pool.Query(ctx, query, endpointID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing model endpoint versions: %w", err)
	}
	defer rows.Close()

	var versions []ModelEndpointVersion
	for rows.Next() {
		var v ModelEndpointVersion
		if err := rows.Scan(
			&v.ID, &v.EndpointID, &v.Version, &v.Config,
			&v.IsActive, &v.ChangeNote, &v.CreatedBy, &v.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning model endpoint version: %w", err)
		}
		v.Config, _ = s.decryptConfigHeaders(v.Config)
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating model endpoint versions: %w", err)
	}

	return versions, total, nil
}

// GetVersion retrieves a specific version of a model endpoint.
func (s *ModelEndpointStore) GetVersion(ctx context.Context, endpointID uuid.UUID, version int) (*ModelEndpointVersion, error) {
	query := `
		SELECT id, endpoint_id, version, config, is_active, change_note, created_by, created_at
		FROM model_endpoint_versions
		WHERE endpoint_id = $1 AND version = $2`

	v := &ModelEndpointVersion{}
	err := s.pool.QueryRow(ctx, query, endpointID, version).Scan(
		&v.ID, &v.EndpointID, &v.Version, &v.Config,
		&v.IsActive, &v.ChangeNote, &v.CreatedBy, &v.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("model_endpoint_version", fmt.Sprintf("v%d", version))
		}
		return nil, fmt.Errorf("getting model endpoint version: %w", err)
	}
	v.Config, _ = s.decryptConfigHeaders(v.Config)
	return v, nil
}

// GetActiveVersion returns the currently active version for a model endpoint.
func (s *ModelEndpointStore) GetActiveVersion(ctx context.Context, endpointID uuid.UUID) (*ModelEndpointVersion, error) {
	query := `
		SELECT id, endpoint_id, version, config, is_active, change_note, created_by, created_at
		FROM model_endpoint_versions
		WHERE endpoint_id = $1 AND is_active = true
		LIMIT 1`

	v := &ModelEndpointVersion{}
	err := s.pool.QueryRow(ctx, query, endpointID).Scan(
		&v.ID, &v.EndpointID, &v.Version, &v.Config,
		&v.IsActive, &v.ChangeNote, &v.CreatedBy, &v.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("model_endpoint_version", fmt.Sprintf("active for endpoint '%s'", endpointID))
		}
		return nil, fmt.Errorf("getting active model endpoint version: %w", err)
	}
	v.Config, _ = s.decryptConfigHeaders(v.Config)
	return v, nil
}

// ActivateVersion activates a specific version, deactivating the currently active one.
func (s *ModelEndpointStore) ActivateVersion(ctx context.Context, endpointID uuid.UUID, version int) (*ModelEndpointVersion, error) {
	target, err := s.GetVersion(ctx, endpointID, version)
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx,
		"UPDATE model_endpoint_versions SET is_active = false WHERE endpoint_id = $1 AND is_active = true",
		endpointID,
	)
	if err != nil {
		return nil, fmt.Errorf("deactivating current version: %w", err)
	}

	_, err = tx.Exec(ctx,
		"UPDATE model_endpoint_versions SET is_active = true WHERE endpoint_id = $1 AND version = $2",
		endpointID, version,
	)
	if err != nil {
		return nil, fmt.Errorf("activating version: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	target.IsActive = true
	return target, nil
}

// CountAll returns the total number of model endpoints.
func (s *ModelEndpointStore) CountAll(ctx context.Context) (int, error) {
	var count int
	query := "SELECT COUNT(*) FROM model_endpoints"
	if err := s.pool.QueryRow(ctx, query).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting all model endpoints: %w", err)
	}
	return count, nil
}

// encryptConfigHeaders encrypts the "headers" field within a config JSONB if present and encKey is set.
func (s *ModelEndpointStore) encryptConfigHeaders(config json.RawMessage) (json.RawMessage, error) {
	if len(s.encKey) != 32 || config == nil {
		return config, nil
	}

	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(config, &parsed); err != nil {
		return config, nil
	}

	headersRaw, ok := parsed["headers"]
	if !ok || string(headersRaw) == "null" || string(headersRaw) == "{}" {
		return config, nil
	}

	encrypted, err := auth.Encrypt(headersRaw, s.encKey)
	if err != nil {
		return nil, fmt.Errorf("encrypting headers: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(encrypted)
	encodedJSON, _ := json.Marshal(encoded)
	parsed["headers"] = encodedJSON

	return json.Marshal(parsed)
}

// decryptConfigHeaders decrypts the "headers" field within a config JSONB if present and encKey is set.
func (s *ModelEndpointStore) decryptConfigHeaders(config json.RawMessage) (json.RawMessage, error) {
	if len(s.encKey) != 32 || config == nil {
		return config, nil
	}

	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(config, &parsed); err != nil {
		return config, nil
	}

	headersRaw, ok := parsed["headers"]
	if !ok {
		return config, nil
	}

	var encoded string
	if err := json.Unmarshal(headersRaw, &encoded); err != nil {
		return config, nil
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return config, nil
	}

	decrypted, err := auth.Decrypt(ciphertext, s.encKey)
	if err != nil {
		return config, nil
	}

	parsed["headers"] = decrypted
	return json.Marshal(parsed)
}
