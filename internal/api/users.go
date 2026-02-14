package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/auth"
	apierrors "github.com/agent-smit/agentic-registry/internal/errors"
	"github.com/agent-smit/agentic-registry/internal/store"
)

// UserStoreForAPI is the interface the users handler needs from the user store.
type UserStoreForAPI interface {
	List(ctx context.Context, offset, limit int) ([]store.User, int, error)
	Create(ctx context.Context, user *store.User) error
	GetByID(ctx context.Context, id uuid.UUID) (*store.User, error)
	Update(ctx context.Context, user *store.User) error
	CountAdmins(ctx context.Context) (int, error)
}

// OAuthConnStoreForAPI is the interface for deleting OAuth connections during auth reset.
type OAuthConnStoreForAPI interface {
	DeleteByUserID(ctx context.Context, userID uuid.UUID) error
}

// AuditStoreForAPI is the interface for audit logging.
type AuditStoreForAPI interface {
	Insert(ctx context.Context, entry *store.AuditEntry) error
}

// UsersHandler provides HTTP handlers for user management endpoints.
type UsersHandler struct {
	users      UserStoreForAPI
	oauthConns OAuthConnStoreForAPI
	audit      AuditStoreForAPI
}

// NewUsersHandler creates a new UsersHandler.
func NewUsersHandler(users UserStoreForAPI, oauthConns OAuthConnStoreForAPI, audit AuditStoreForAPI) *UsersHandler {
	return &UsersHandler{
		users:      users,
		oauthConns: oauthConns,
		audit:      audit,
	}
}

var validRoles = map[string]bool{
	"admin":  true,
	"editor": true,
	"viewer": true,
}

// userAPIResponse is the JSON representation of a user, never exposing password_hash.
type userAPIResponse struct {
	ID             uuid.UUID  `json:"id"`
	Username       string     `json:"username"`
	Email          string     `json:"email"`
	DisplayName    string     `json:"display_name"`
	Role           string     `json:"role"`
	AuthMethod     string     `json:"auth_method"`
	IsActive       bool       `json:"is_active"`
	MustChangePass bool       `json:"must_change_password"`
	LastLoginAt    *time.Time `json:"last_login_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func toUserAPIResponse(u *store.User) userAPIResponse {
	return userAPIResponse{
		ID:             u.ID,
		Username:       u.Username,
		Email:          u.Email,
		DisplayName:    u.DisplayName,
		Role:           u.Role,
		AuthMethod:     u.AuthMethod,
		IsActive:       u.IsActive,
		MustChangePass: u.MustChangePass,
		LastLoginAt:    u.LastLoginAt,
		CreatedAt:      u.CreatedAt,
		UpdatedAt:      u.UpdatedAt,
	}
}

// List handles GET /api/v1/users.
func (h *UsersHandler) List(w http.ResponseWriter, r *http.Request) {
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	users, total, err := h.users.List(r.Context(), offset, limit)
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to list users"))
		return
	}

	resp := make([]userAPIResponse, 0, len(users))
	for i := range users {
		resp = append(resp, toUserAPIResponse(&users[i]))
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"users": resp,
		"total": total,
	})
}

type createUserRequest struct {
	Username    string `json:"username"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
	Role        string `json:"role"`
}

// Create handles POST /api/v1/users.
func (h *UsersHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	if req.Username == "" {
		RespondError(w, r, apierrors.Validation("username is required"))
		return
	}
	if req.Email == "" {
		RespondError(w, r, apierrors.Validation("email is required"))
		return
	}
	if req.Password == "" {
		RespondError(w, r, apierrors.Validation("password is required"))
		return
	}
	if !validRoles[req.Role] {
		RespondError(w, r, apierrors.Validation("role must be one of: admin, editor, viewer"))
		return
	}

	if err := auth.ValidatePasswordPolicy(req.Password); err != nil {
		RespondError(w, r, apierrors.Validation(err.Error()))
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to hash password"))
		return
	}

	user := &store.User{
		Username:       req.Username,
		Email:          req.Email,
		DisplayName:    req.DisplayName,
		PasswordHash:   hash,
		Role:           req.Role,
		AuthMethod:     "password",
		IsActive:       true,
		MustChangePass: false,
	}

	if err := h.users.Create(r.Context(), user); err != nil {
		RespondError(w, r, apierrors.Internal("failed to create user"))
		return
	}

	h.auditLog(r, "user_create", "user", user.ID.String())

	RespondJSON(w, r, http.StatusCreated, toUserAPIResponse(user))
}

// Get handles GET /api/v1/users/{userId}.
func (h *UsersHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(chi.URLParam(r, "userId"))
	if err != nil {
		RespondError(w, r, apierrors.Validation("invalid user ID"))
		return
	}

	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		RespondError(w, r, apierrors.NotFound("user", userID.String()))
		return
	}

	RespondJSON(w, r, http.StatusOK, toUserAPIResponse(user))
}

type updateUserRequest struct {
	DisplayName *string `json:"display_name"`
	Role        *string `json:"role"`
	IsActive    *bool   `json:"is_active"`
}

// Update handles PUT /api/v1/users/{userId}.
func (h *UsersHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(chi.URLParam(r, "userId"))
	if err != nil {
		RespondError(w, r, apierrors.Validation("invalid user ID"))
		return
	}

	ifMatch := r.Header.Get("If-Match")
	if ifMatch == "" {
		RespondError(w, r, apierrors.Validation("If-Match header is required for updates"))
		return
	}

	etag, err := time.Parse(time.RFC3339Nano, ifMatch)
	if err != nil {
		RespondError(w, r, apierrors.Validation("invalid If-Match value"))
		return
	}

	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		RespondError(w, r, apierrors.NotFound("user", userID.String()))
		return
	}

	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	if req.DisplayName != nil {
		user.DisplayName = *req.DisplayName
	}
	if req.Role != nil {
		if !validRoles[*req.Role] {
			RespondError(w, r, apierrors.Validation("role must be one of: admin, editor, viewer"))
			return
		}
		user.Role = *req.Role
	}
	if req.IsActive != nil {
		user.IsActive = *req.IsActive
	}

	// Set UpdatedAt to the etag for optimistic concurrency
	user.UpdatedAt = etag

	if err := h.users.Update(r.Context(), user); err != nil {
		RespondError(w, r, apierrors.Conflict("user was modified by another request"))
		return
	}

	h.auditLog(r, "user_update", "user", user.ID.String())

	RespondJSON(w, r, http.StatusOK, toUserAPIResponse(user))
}

// Delete handles DELETE /api/v1/users/{userId}.
func (h *UsersHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(chi.URLParam(r, "userId"))
	if err != nil {
		RespondError(w, r, apierrors.Validation("invalid user ID"))
		return
	}

	callerID, _ := auth.UserIDFromContext(r.Context())
	if callerID == userID {
		RespondError(w, r, apierrors.Validation("cannot deactivate yourself"))
		return
	}

	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		RespondError(w, r, apierrors.NotFound("user", userID.String()))
		return
	}

	user.IsActive = false
	if err := h.users.Update(r.Context(), user); err != nil {
		RespondError(w, r, apierrors.Internal("failed to deactivate user"))
		return
	}

	h.auditLog(r, "user_deactivate", "user", user.ID.String())

	RespondNoContent(w)
}

type resetAuthRequest struct {
	NewPassword string `json:"new_password"`
	ForceChange bool   `json:"force_change"`
}

// ResetAuth handles POST /api/v1/users/{userId}/reset-auth.
func (h *UsersHandler) ResetAuth(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(chi.URLParam(r, "userId"))
	if err != nil {
		RespondError(w, r, apierrors.Validation("invalid user ID"))
		return
	}

	callerID, _ := auth.UserIDFromContext(r.Context())
	if callerID == userID {
		RespondError(w, r, apierrors.Validation("cannot reset your own auth"))
		return
	}

	var req resetAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, apierrors.Validation("invalid request body"))
		return
	}

	if req.NewPassword == "" {
		RespondError(w, r, apierrors.Validation("new_password is required"))
		return
	}

	if err := auth.ValidatePasswordPolicy(req.NewPassword); err != nil {
		RespondError(w, r, apierrors.Validation(err.Error()))
		return
	}

	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		RespondError(w, r, apierrors.NotFound("user", userID.String()))
		return
	}

	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to hash password"))
		return
	}

	user.PasswordHash = hash
	user.AuthMethod = "password"
	user.MustChangePass = true
	user.FailedLogins = 0
	user.LockedUntil = nil

	if err := h.users.Update(r.Context(), user); err != nil {
		RespondError(w, r, apierrors.Internal("failed to update user"))
		return
	}

	// Remove all OAuth connections for this user
	if h.oauthConns != nil {
		h.oauthConns.DeleteByUserID(r.Context(), userID)
	}

	h.auditLog(r, "auth_reset", "user", user.ID.String())

	RespondJSON(w, r, http.StatusOK, toUserAPIResponse(user))
}

func (h *UsersHandler) auditLog(r *http.Request, action, resourceType, resourceID string) {
	if h.audit == nil {
		return
	}
	callerID, _ := auth.UserIDFromContext(r.Context())
	if err := h.audit.Insert(r.Context(), &store.AuditEntry{
		Actor:        callerID.String(),
		ActorID:      &callerID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		IPAddress:    clientIPFromRequest(r),
	}); err != nil {
		log.Printf("audit log failed for %s %s/%s: %v", action, resourceType, resourceID, err)
	}
}

func clientIPFromRequest(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	addr := r.RemoteAddr
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
}
