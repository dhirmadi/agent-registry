package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/agent-smit/agentic-registry/internal/api"
	internalAuth "github.com/agent-smit/agentic-registry/internal/auth"
	"github.com/agent-smit/agentic-registry/internal/config"
	"github.com/agent-smit/agentic-registry/internal/db"
	"github.com/agent-smit/agentic-registry/internal/notify"
	"github.com/agent-smit/agentic-registry/internal/ratelimit"
	"github.com/agent-smit/agentic-registry/internal/seed"
	"github.com/agent-smit/agentic-registry/internal/store"
	"github.com/agent-smit/agentic-registry/internal/telemetry"
	"github.com/agent-smit/agentic-registry/web"
)

func main() {
	// Parse CLI flags
	var resetAdmin bool
	var newPassword string
	flag.BoolVar(&resetAdmin, "reset-admin", false, "Reset admin account password and auth method")
	flag.StringVar(&newPassword, "new-password", "", "New admin password (used with --reset-admin)")
	flag.Parse()

	if resetAdmin {
		if err := runResetAdmin(newPassword); err != nil {
			log.Fatalf("reset-admin failed: %v", err)
		}
		return
	}

	if err := run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}

func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// Configure cookie security based on EXTERNAL_URL scheme.
	// __Host- cookies require Secure flag, which requires HTTPS.
	if strings.HasPrefix(cfg.ExternalURL, "http://") {
		internalAuth.SetSecureCookies(false)
		log.Println("warning: running in HTTP mode — cookies are NOT secure (dev only)")
	}

	log.Printf("starting agentic-registry on port %s", cfg.Port)

	// Initialize telemetry (stub for now)
	shutdownTelemetry, err := telemetry.Init(ctx, cfg.OTELServiceName, cfg.OTELExporterEndpoint)
	if err != nil {
		return fmt.Errorf("telemetry: %w", err)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := shutdownTelemetry(shutdownCtx); err != nil {
			log.Printf("telemetry shutdown error: %v", err)
		}
	}()

	// Create database pool
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database pool: %w", err)
	}
	defer pool.Close()

	// Run migrations
	sqlDB, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("sql.Open for migrations: %w", err)
	}
	defer sqlDB.Close()

	if err := db.RunMigrations(sqlDB); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	log.Println("database migrations applied")

	// Create stores
	userStore := store.NewUserStore(pool)
	sessionStore := store.NewSessionStore(pool)
	apiKeyStore := store.NewAPIKeyStore(pool)
	auditStore := store.NewAuditStore(pool)
	oauthConnStore := store.NewOAuthConnectionStore(pool)
	agentStore := store.NewAgentStore(pool)
	promptStore := store.NewPromptStore(pool)
	mcpServerStore := store.NewMCPServerStore(pool)
	trustRuleStore := store.NewTrustRuleStore(pool)
	trustDefaultStore := store.NewTrustDefaultStore(pool)
	modelConfigStore := store.NewModelConfigStore(pool)
	webhookStore := store.NewWebhookStore(pool)
	modelEndpointStore := store.NewModelEndpointStore(pool, []byte(cfg.CredentialEncryptionKey))

	// Create webhook dispatcher
	dispatcher := notify.NewDispatcher(&subscriptionLoaderAdapter{store: webhookStore}, notify.Config{
		Workers:    cfg.WebhookWorkers,
		MaxRetries: cfg.WebhookRetries,
		Timeout:    time.Duration(cfg.WebhookTimeoutS) * time.Second,
	})
	dispatcher.Start()
	defer dispatcher.Stop()

	// Seed default admin
	if err := seedDefaultAdmin(ctx, userStore); err != nil {
		log.Printf("warning: failed to seed default admin: %v", err)
	}

	// Seed default agents
	if err := seed.SeedAgents(ctx, agentStore); err != nil {
		log.Printf("warning: failed to seed default agents: %v", err)
	}

	// Session cleanup goroutine
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				deleted, err := sessionStore.DeleteExpired(ctx)
				if err != nil {
					log.Printf("session cleanup error: %v", err)
				} else if deleted > 0 {
					log.Printf("cleaned up %d expired sessions", deleted)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Create auth handler adapters
	sessionLookup := &sessionLookupAdapter{
		sessions: sessionStore,
		users:    userStore,
	}
	apiKeyLookup := &apiKeyLookupAdapter{
		apiKeys: apiKeyStore,
		users:   userStore,
	}

	// Create rate limiter
	rateLimiter := ratelimit.NewRateLimiter()

	// Create auth middleware
	authMW := api.AuthMiddleware(sessionLookup, apiKeyLookup)

	// Create auth handler (wraps store types to match auth package interfaces)
	authUserAdapter := &authUserStoreAdapter{store: userStore}
	authSessionAdapter := &authSessionStoreAdapter{store: sessionStore}
	authAuditAdapter := &authAuditStoreAdapter{store: auditStore}
	authOAuthConnAdapter := &authOAuthConnStoreAdapter{store: oauthConnStore}

	// Build auth handler with or without OAuth
	var authHandler *internalAuth.Handler
	if cfg.GoogleOAuthClientID != "" && cfg.GoogleOAuthClientSecret != "" {
		oauthProvider := internalAuth.NewOAuthProvider(internalAuth.OAuthConfig{
			ClientID:     cfg.GoogleOAuthClientID,
			ClientSecret: cfg.GoogleOAuthClientSecret,
			RedirectURL:  cfg.ExternalURL + "/auth/google/callback",
		})
		encKey := []byte(cfg.CredentialEncryptionKey)
		authHandler = internalAuth.NewHandlerWithOAuth(authUserAdapter, authSessionAdapter, authAuditAdapter, oauthProvider, authOAuthConnAdapter, encKey)
	} else {
		authHandler = internalAuth.NewHandler(authUserAdapter, authSessionAdapter, authAuditAdapter)
	}

	// Create API handlers
	health := &api.HealthHandler{DB: pool}
	usersHandler := api.NewUsersHandler(userStore, oauthConnStore, auditStore)
	apiKeysHandler := api.NewAPIKeysHandler(apiKeyStore, auditStore)
	agentsHandler := api.NewAgentsHandler(agentStore, auditStore, dispatcher)
	promptsHandler := api.NewPromptsHandler(promptStore, agentStore, auditStore, dispatcher)

	// Encryption key for MCP server credentials
	encKey := []byte(cfg.CredentialEncryptionKey)
	mcpServersHandler := api.NewMCPServersHandler(mcpServerStore, auditStore, encKey, dispatcher)
	trustRulesHandler := api.NewTrustRulesHandler(trustRuleStore, auditStore, dispatcher)
	trustDefaultsHandler := api.NewTrustDefaultsHandler(trustDefaultStore, auditStore, dispatcher)
	modelConfigHandler := api.NewModelConfigHandler(modelConfigStore, auditStore, dispatcher)
	webhooksHandler := api.NewWebhooksHandler(webhookStore, auditStore)
	modelEndpointsHandler := api.NewModelEndpointsHandler(modelEndpointStore, auditStore, encKey, dispatcher)
	auditLogHandler := api.NewAuditHandler(auditStore)
	discoveryHandler := api.NewDiscoveryHandler(agentStore, mcpServerStore, trustDefaultStore, modelConfigStore, modelEndpointStore)
	a2aHandler := api.NewA2AHandler(agentStore, cfg.ExternalURL)

	// Create MCP handler (if enabled)
	var mcpHandler *api.MCPHandler
	if cfg.MCPEnabled {
		mcpToolExecutor := api.NewMCPToolExecutor(agentStore, promptStore, mcpServerStore, modelConfigStore, modelEndpointStore, cfg.ExternalURL)
		mcpResourceProvider := api.NewMCPResourceProvider(agentStore, promptStore, modelConfigStore)
		mcpPromptProvider := api.NewMCPPromptProvider(agentStore, promptStore)
		mcpManifestHandler := api.NewMCPManifestHandler(cfg.ExternalURL)
		mcpHandler = api.NewMCPHandler(mcpToolExecutor, mcpResourceProvider, mcpPromptProvider, mcpManifestHandler)
		log.Println("MCP protocol enabled")
	}

	// Set up router
	router := api.NewRouter(api.RouterConfig{
		Health:        health,
		Auth:          authHandler,
		Users:         usersHandler,
		APIKeys:       apiKeysHandler,
		Agents:        agentsHandler,
		Prompts:       promptsHandler,
		MCPServers:    mcpServersHandler,
		TrustRules:    trustRulesHandler,
		TrustDefaults: trustDefaultsHandler,
		ModelConfig:    modelConfigHandler,
		ModelEndpoints: modelEndpointsHandler,
		Webhooks:      webhooksHandler,
		Discovery:     discoveryHandler,
		A2A:           a2aHandler,
		MCP:           mcpHandler,
		AuditLog:      auditLogHandler,
		AuthMW:        authMW,
		UserLookup:    &userLookupAdapter{store: userStore},
		RateLimiter:   rateLimiter,
		WebFS:         web.FS,
	})

	// Create HTTP server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	errCh := make(chan error, 1)
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("received signal %v, shutting down...", sig)
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer shutdownCancel()
		errCh <- srv.Shutdown(shutdownCtx)
	}()

	log.Printf("server listening on :%s", cfg.Port)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("server: %w", err)
	}

	return <-errCh
}

// seedDefaultAdmin creates the default admin user if no admin exists.
func seedDefaultAdmin(ctx context.Context, userStore *store.UserStore) error {
	count, err := userStore.CountAdmins(ctx)
	if err != nil {
		return fmt.Errorf("counting admins: %w", err)
	}
	if count > 0 {
		return nil
	}

	hash, err := internalAuth.HashPassword("admin")
	if err != nil {
		return fmt.Errorf("hashing default admin password: %w", err)
	}

	user := &store.User{
		Username:       "admin",
		Email:          "admin@localhost",
		DisplayName:    "Administrator",
		PasswordHash:   hash,
		Role:           "admin",
		AuthMethod:     "password",
		IsActive:       true,
		MustChangePass: true,
	}

	if err := userStore.Create(ctx, user); err != nil {
		return fmt.Errorf("creating default admin: %w", err)
	}

	log.Println("default admin account created (admin/admin, must change password on first login)")
	return nil
}

// runResetAdmin handles the --reset-admin CLI flag.
func runResetAdmin(newPassword string) error {
	if newPassword == "" {
		return fmt.Errorf("--new-password is required with --reset-admin")
	}

	if err := internalAuth.ValidatePasswordPolicy(newPassword); err != nil {
		return fmt.Errorf("password policy: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer pool.Close()

	// Run migrations first
	sqlDB, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("sql.Open: %w", err)
	}
	defer sqlDB.Close()

	if err := db.RunMigrations(sqlDB); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}

	userStore := store.NewUserStore(pool)
	auditStore := store.NewAuditStore(pool)

	// Find admin user
	admin, err := userStore.GetByUsername(ctx, "admin")
	if err != nil {
		return fmt.Errorf("admin user not found: %w", err)
	}

	// Hash new password
	hash, err := internalAuth.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}

	// Reset admin account
	admin.PasswordHash = hash
	admin.AuthMethod = "password"
	admin.MustChangePass = true
	admin.FailedLogins = 0
	admin.LockedUntil = nil

	if err := userStore.Update(ctx, admin); err != nil {
		return fmt.Errorf("updating admin: %w", err)
	}

	// Delete OAuth connections for admin (if oauth_connections table exists)
	deleteOAuthQuery := `DELETE FROM oauth_connections WHERE user_id = $1`
	pool.Exec(ctx, deleteOAuthQuery, admin.ID)

	// Audit log
	auditStore.Insert(ctx, &store.AuditEntry{
		Actor:        "cli-reset",
		Action:       "admin_reset",
		ResourceType: "user",
		ResourceID:   admin.ID.String(),
		IPAddress:    "127.0.0.1",
	})

	fmt.Println("admin account reset successfully")
	return nil
}

// --- Adapter types to bridge store types to auth/middleware interfaces ---

// sessionLookupAdapter implements api.SessionLookup.
type sessionLookupAdapter struct {
	sessions *store.SessionStore
	users    *store.UserStore
}

func (a *sessionLookupAdapter) GetSessionUser(ctx context.Context, sessionID string) (uuid.UUID, string, string, error) {
	sess, err := a.sessions.GetByID(ctx, sessionID)
	if err != nil {
		return uuid.Nil, "", "", err
	}

	user, err := a.users.GetByID(ctx, sess.UserID)
	if err != nil {
		return uuid.Nil, "", "", err
	}

	return user.ID, user.Role, sess.CSRFToken, nil
}

func (a *sessionLookupAdapter) TouchSession(ctx context.Context, sessionID string) error {
	return a.sessions.UpdateLastSeen(ctx, sessionID)
}

// apiKeyLookupAdapter implements api.APIKeyLookup.
type apiKeyLookupAdapter struct {
	apiKeys *store.APIKeyStore
	users   *store.UserStore
}

func (a *apiKeyLookupAdapter) ValidateAPIKey(ctx context.Context, key string) (uuid.UUID, string, error) {
	hash := internalAuth.HashAPIKey(key)
	apiKey, err := a.apiKeys.GetByHash(ctx, hash)
	if err != nil {
		return uuid.Nil, "", err
	}

	if apiKey.UserID == nil {
		return uuid.Nil, "", fmt.Errorf("api key has no associated user")
	}

	user, err := a.users.GetByID(ctx, *apiKey.UserID)
	if err != nil {
		return uuid.Nil, "", err
	}

	if !user.IsActive {
		return uuid.Nil, "", fmt.Errorf("user is inactive")
	}

	// Update last used timestamp (fire and forget)
	go a.apiKeys.UpdateLastUsed(context.Background(), apiKey.ID)

	return user.ID, user.Role, nil
}

// authUserStoreAdapter bridges store.UserStore to auth.UserForAuth.
type authUserStoreAdapter struct {
	store *store.UserStore
}

func (a *authUserStoreAdapter) GetByUsername(ctx context.Context, username string) (*internalAuth.UserRecord, error) {
	u, err := a.store.GetByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	return storeUserToAuthRecord(u), nil
}

func (a *authUserStoreAdapter) GetByID(ctx context.Context, id uuid.UUID) (*internalAuth.UserRecord, error) {
	u, err := a.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return storeUserToAuthRecord(u), nil
}

func (a *authUserStoreAdapter) IncrementFailedLogins(ctx context.Context, id uuid.UUID) error {
	return a.store.IncrementFailedLogins(ctx, id)
}

func (a *authUserStoreAdapter) ResetFailedLogins(ctx context.Context, id uuid.UUID) error {
	return a.store.ResetFailedLogins(ctx, id)
}

func (a *authUserStoreAdapter) LockAccount(ctx context.Context, id uuid.UUID, until time.Time) error {
	return a.store.LockAccount(ctx, id, until)
}

func (a *authUserStoreAdapter) UpdatePassword(ctx context.Context, id uuid.UUID, hash string) error {
	u, err := a.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	u.PasswordHash = hash
	u.MustChangePass = false
	return a.store.Update(ctx, u)
}

func (a *authUserStoreAdapter) GetByEmail(ctx context.Context, email string) (*internalAuth.UserRecord, error) {
	u, err := a.store.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	return storeUserToAuthRecord(u), nil
}

func (a *authUserStoreAdapter) Create(ctx context.Context, user *internalAuth.UserRecord) error {
	storeUser := &store.User{
		ID:             user.ID,
		Username:       user.Username,
		Email:          user.Email,
		DisplayName:    user.DisplayName,
		PasswordHash:   user.PasswordHash,
		Role:           user.Role,
		AuthMethod:     user.AuthMethod,
		IsActive:       user.IsActive,
		MustChangePass: user.MustChangePass,
	}
	return a.store.Create(ctx, storeUser)
}

func (a *authUserStoreAdapter) UpdateAuthMethod(ctx context.Context, id uuid.UUID, method string, clearPassword bool) error {
	u, err := a.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	u.AuthMethod = method
	if clearPassword {
		u.PasswordHash = ""
	}
	return a.store.Update(ctx, u)
}

func storeUserToAuthRecord(u *store.User) *internalAuth.UserRecord {
	return &internalAuth.UserRecord{
		ID:             u.ID,
		Username:       u.Username,
		Email:          u.Email,
		DisplayName:    u.DisplayName,
		PasswordHash:   u.PasswordHash,
		Role:           u.Role,
		AuthMethod:     u.AuthMethod,
		IsActive:       u.IsActive,
		MustChangePass: u.MustChangePass,
		FailedLogins:   u.FailedLogins,
		LockedUntil:    u.LockedUntil,
		LastLoginAt:    u.LastLoginAt,
	}
}

// authSessionStoreAdapter bridges store.SessionStore to auth.SessionForAuth.
type authSessionStoreAdapter struct {
	store *store.SessionStore
}

func (a *authSessionStoreAdapter) Create(ctx context.Context, sess *internalAuth.SessionRecord) error {
	storeSess := &store.Session{
		ID:        sess.ID,
		UserID:    sess.UserID,
		CSRFToken: sess.CSRFToken,
		IPAddress: sess.IPAddress,
		UserAgent: sess.UserAgent,
		ExpiresAt: sess.ExpiresAt,
	}
	return a.store.Create(ctx, storeSess)
}

func (a *authSessionStoreAdapter) GetByID(ctx context.Context, id string) (*internalAuth.SessionRecord, error) {
	s, err := a.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &internalAuth.SessionRecord{
		ID:        s.ID,
		UserID:    s.UserID,
		CSRFToken: s.CSRFToken,
		IPAddress: s.IPAddress,
		UserAgent: s.UserAgent,
		ExpiresAt: s.ExpiresAt,
	}, nil
}

func (a *authSessionStoreAdapter) Delete(ctx context.Context, id string) error {
	return a.store.Delete(ctx, id)
}

func (a *authSessionStoreAdapter) DeleteByUserID(ctx context.Context, userID uuid.UUID) error {
	return a.store.DeleteByUserID(ctx, userID)
}

func (a *authSessionStoreAdapter) DeleteOthersByUserID(ctx context.Context, userID uuid.UUID, keepSessionID string) error {
	return a.store.DeleteOthersByUserID(ctx, userID, keepSessionID)
}

// authAuditStoreAdapter bridges store.AuditStore to auth.AuditForAuth.
type authAuditStoreAdapter struct {
	store *store.AuditStore
}

func (a *authAuditStoreAdapter) Insert(ctx context.Context, entry *internalAuth.AuditRecord) error {
	storeEntry := &store.AuditEntry{
		Actor:        entry.Actor,
		ActorID:      entry.ActorID,
		Action:       entry.Action,
		ResourceType: entry.ResourceType,
		ResourceID:   entry.ResourceID,
		Details:      entry.Details,
		IPAddress:    entry.IPAddress,
	}
	return a.store.Insert(ctx, storeEntry)
}

// authOAuthConnStoreAdapter bridges store.OAuthConnectionStore to auth.OAuthConnectionForAuth.
type authOAuthConnStoreAdapter struct {
	store *store.OAuthConnectionStore
}

func (a *authOAuthConnStoreAdapter) GetByProviderUID(ctx context.Context, provider, providerUID string) (*internalAuth.OAuthConnectionRecord, error) {
	c, err := a.store.GetByProviderUID(ctx, provider, providerUID)
	if err != nil {
		return nil, err
	}
	return storeOAuthConnToAuthRecord(c), nil
}

func (a *authOAuthConnStoreAdapter) Create(ctx context.Context, conn *internalAuth.OAuthConnectionRecord) error {
	storeConn := &store.OAuthConnection{
		ID:          conn.ID,
		UserID:      conn.UserID,
		Provider:    conn.Provider,
		ProviderUID: conn.ProviderUID,
		Email:       conn.Email,
		DisplayName: conn.DisplayName,
	}
	return a.store.Create(ctx, storeConn)
}

func (a *authOAuthConnStoreAdapter) DeleteByUserID(ctx context.Context, userID uuid.UUID) error {
	return a.store.DeleteByUserID(ctx, userID)
}

func (a *authOAuthConnStoreAdapter) GetByUserID(ctx context.Context, userID uuid.UUID) ([]internalAuth.OAuthConnectionRecord, error) {
	conns, err := a.store.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]internalAuth.OAuthConnectionRecord, len(conns))
	for i, c := range conns {
		result[i] = *storeOAuthConnToAuthRecord(&c)
	}
	return result, nil
}

func storeOAuthConnToAuthRecord(c *store.OAuthConnection) *internalAuth.OAuthConnectionRecord {
	return &internalAuth.OAuthConnectionRecord{
		ID:          c.ID,
		UserID:      c.UserID,
		Provider:    c.Provider,
		ProviderUID: c.ProviderUID,
		Email:       c.Email,
		DisplayName: c.DisplayName,
	}
}

// userLookupAdapter implements api.UserLookup for MustChangePassMiddleware.
type userLookupAdapter struct {
	store *store.UserStore
}

func (a *userLookupAdapter) GetMustChangePass(ctx context.Context, userID uuid.UUID) (bool, error) {
	return a.store.GetMustChangePass(ctx, userID)
}

// subscriptionLoaderAdapter bridges store.WebhookStore to notify.SubscriptionLoader.
type subscriptionLoaderAdapter struct {
	store *store.WebhookStore
}

func (a *subscriptionLoaderAdapter) ListActive(ctx context.Context) ([]notify.Subscription, error) {
	subs, err := a.store.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]notify.Subscription, len(subs))
	for i, s := range subs {
		var events []string
		if len(s.Events) > 0 {
			json.Unmarshal(s.Events, &events)
		}
		result[i] = notify.Subscription{
			ID:     s.ID,
			URL:    s.URL,
			Secret: s.Secret,
			Events: events,
		}
	}
	return result, nil
}
