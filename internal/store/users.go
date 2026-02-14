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

// User represents a user account in the registry.
type User struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	Username       string     `json:"username" db:"username"`
	Email          string     `json:"email" db:"email"`
	DisplayName    string     `json:"display_name" db:"display_name"`
	PasswordHash   string     `json:"-" db:"password_hash"`
	Role           string     `json:"role" db:"role"`
	AuthMethod     string     `json:"auth_method" db:"auth_method"`
	IsActive       bool       `json:"is_active" db:"is_active"`
	MustChangePass bool       `json:"must_change_password" db:"must_change_pass"`
	FailedLogins   int        `json:"-" db:"failed_logins"`
	LockedUntil    *time.Time `json:"-" db:"locked_until"`
	LastLoginAt    *time.Time `json:"last_login_at" db:"last_login_at"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}

// UserStore handles database operations for users.
type UserStore struct {
	pool *pgxpool.Pool
}

// NewUserStore creates a new UserStore.
func NewUserStore(pool *pgxpool.Pool) *UserStore {
	return &UserStore{pool: pool}
}

// Create inserts a new user.
func (s *UserStore) Create(ctx context.Context, user *User) error {
	query := `
		INSERT INTO users (id, username, email, display_name, password_hash, role, auth_method, is_active, must_change_pass)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at, updated_at`

	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}

	err := s.pool.QueryRow(ctx, query,
		user.ID, user.Username, user.Email, user.DisplayName,
		user.PasswordHash, user.Role, user.AuthMethod, user.IsActive, user.MustChangePass,
	).Scan(&user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return fmt.Errorf("creating user: %w", err)
	}
	return nil
}

// GetByID retrieves a user by ID.
func (s *UserStore) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	query := `
		SELECT id, username, email, display_name, password_hash, role, auth_method,
		       is_active, must_change_pass, failed_logins, locked_until, last_login_at,
		       created_at, updated_at
		FROM users WHERE id = $1`

	user := &User{}
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&user.ID, &user.Username, &user.Email, &user.DisplayName,
		&user.PasswordHash, &user.Role, &user.AuthMethod,
		&user.IsActive, &user.MustChangePass, &user.FailedLogins,
		&user.LockedUntil, &user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("user", id.String())
		}
		return nil, fmt.Errorf("getting user by id: %w", err)
	}
	return user, nil
}

// GetByUsername retrieves a user by username.
func (s *UserStore) GetByUsername(ctx context.Context, username string) (*User, error) {
	query := `
		SELECT id, username, email, display_name, password_hash, role, auth_method,
		       is_active, must_change_pass, failed_logins, locked_until, last_login_at,
		       created_at, updated_at
		FROM users WHERE username = $1`

	user := &User{}
	err := s.pool.QueryRow(ctx, query, username).Scan(
		&user.ID, &user.Username, &user.Email, &user.DisplayName,
		&user.PasswordHash, &user.Role, &user.AuthMethod,
		&user.IsActive, &user.MustChangePass, &user.FailedLogins,
		&user.LockedUntil, &user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("user", username)
		}
		return nil, fmt.Errorf("getting user by username: %w", err)
	}
	return user, nil
}

// GetByEmail retrieves a user by email.
func (s *UserStore) GetByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT id, username, email, display_name, password_hash, role, auth_method,
		       is_active, must_change_pass, failed_logins, locked_until, last_login_at,
		       created_at, updated_at
		FROM users WHERE email = $1`

	user := &User{}
	err := s.pool.QueryRow(ctx, query, email).Scan(
		&user.ID, &user.Username, &user.Email, &user.DisplayName,
		&user.PasswordHash, &user.Role, &user.AuthMethod,
		&user.IsActive, &user.MustChangePass, &user.FailedLogins,
		&user.LockedUntil, &user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("user", email)
		}
		return nil, fmt.Errorf("getting user by email: %w", err)
	}
	return user, nil
}

// Update updates a user's mutable fields. Uses updated_at for optimistic concurrency.
func (s *UserStore) Update(ctx context.Context, user *User) error {
	query := `
		UPDATE users SET
			username = $2, email = $3, display_name = $4, password_hash = $5,
			role = $6, auth_method = $7, is_active = $8, must_change_pass = $9,
			updated_at = now()
		WHERE id = $1 AND updated_at = $10
		RETURNING updated_at`

	err := s.pool.QueryRow(ctx, query,
		user.ID, user.Username, user.Email, user.DisplayName, user.PasswordHash,
		user.Role, user.AuthMethod, user.IsActive, user.MustChangePass, user.UpdatedAt,
	).Scan(&user.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return errors.Conflict("user was modified by another request")
		}
		return fmt.Errorf("updating user: %w", err)
	}
	return nil
}

// IncrementFailedLogins increments the failed login counter for a user.
func (s *UserStore) IncrementFailedLogins(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE users SET failed_logins = failed_logins + 1, updated_at = now() WHERE id = $1`
	ct, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("incrementing failed logins: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return errors.NotFound("user", id.String())
	}
	return nil
}

// ResetFailedLogins resets the failed login counter and updates last_login_at.
func (s *UserStore) ResetFailedLogins(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE users SET failed_logins = 0, locked_until = NULL, last_login_at = now(), updated_at = now() WHERE id = $1`
	ct, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("resetting failed logins: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return errors.NotFound("user", id.String())
	}
	return nil
}

// LockAccount locks a user's account until the specified time.
func (s *UserStore) LockAccount(ctx context.Context, id uuid.UUID, until time.Time) error {
	query := `UPDATE users SET locked_until = $2, updated_at = now() WHERE id = $1`
	ct, err := s.pool.Exec(ctx, query, id, until)
	if err != nil {
		return fmt.Errorf("locking account: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return errors.NotFound("user", id.String())
	}
	return nil
}

// List returns a paginated list of users and the total count.
func (s *UserStore) List(ctx context.Context, offset, limit int) ([]User, int, error) {
	countQuery := `SELECT COUNT(*) FROM users`
	var total int
	if err := s.pool.QueryRow(ctx, countQuery).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting users: %w", err)
	}

	query := `
		SELECT id, username, email, display_name, password_hash, role, auth_method,
		       is_active, must_change_pass, failed_logins, locked_until, last_login_at,
		       created_at, updated_at
		FROM users
		ORDER BY created_at ASC
		LIMIT $1 OFFSET $2`

	rows, err := s.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(
			&u.ID, &u.Username, &u.Email, &u.DisplayName,
			&u.PasswordHash, &u.Role, &u.AuthMethod,
			&u.IsActive, &u.MustChangePass, &u.FailedLogins,
			&u.LockedUntil, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning user: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating users: %w", err)
	}

	return users, total, nil
}

// UpdatePassword updates just the password hash and clears must_change_pass.
func (s *UserStore) UpdatePassword(ctx context.Context, id uuid.UUID, hash string) error {
	query := `UPDATE users SET password_hash = $2, must_change_pass = false, updated_at = now() WHERE id = $1`
	ct, err := s.pool.Exec(ctx, query, id, hash)
	if err != nil {
		return fmt.Errorf("updating password: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return errors.NotFound("user", id.String())
	}
	return nil
}

// UpdateAuthMethod changes the auth_method and optionally clears password_hash.
func (s *UserStore) UpdateAuthMethod(ctx context.Context, id uuid.UUID, method string, clearPassword bool) error {
	var query string
	if clearPassword {
		query = `UPDATE users SET auth_method = $2, password_hash = '', updated_at = now() WHERE id = $1`
	} else {
		query = `UPDATE users SET auth_method = $2, updated_at = now() WHERE id = $1`
	}
	ct, err := s.pool.Exec(ctx, query, id, method)
	if err != nil {
		return fmt.Errorf("updating auth method: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return errors.NotFound("user", id.String())
	}
	return nil
}

// ResetAuth resets a user's authentication: sets password, sets auth_method to "password",
// and sets must_change_pass to true.
func (s *UserStore) ResetAuth(ctx context.Context, id uuid.UUID, passwordHash string) error {
	query := `
		UPDATE users SET
			password_hash = $2,
			auth_method = 'password',
			must_change_pass = true,
			updated_at = now()
		WHERE id = $1`
	ct, err := s.pool.Exec(ctx, query, id, passwordHash)
	if err != nil {
		return fmt.Errorf("resetting auth: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return errors.NotFound("user", id.String())
	}
	return nil
}

// GetMustChangePass returns whether a user must change their password.
func (s *UserStore) GetMustChangePass(ctx context.Context, userID uuid.UUID) (bool, error) {
	var mustChange bool
	err := s.pool.QueryRow(ctx, "SELECT must_change_pass FROM users WHERE id = $1", userID).Scan(&mustChange)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, errors.NotFound("user", userID.String())
		}
		return false, fmt.Errorf("getting must_change_pass: %w", err)
	}
	return mustChange, nil
}

// CountAdmins returns the number of active admin users.
func (s *UserStore) CountAdmins(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM users WHERE role = 'admin' AND is_active = true`
	var count int
	if err := s.pool.QueryRow(ctx, query).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting admins: %w", err)
	}
	return count, nil
}
