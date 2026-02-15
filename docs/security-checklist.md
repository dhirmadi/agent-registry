# Security Checklist

Verification of spec Section 8 security requirements for Agentic Registry.

Last reviewed: 2026-02-14

---

## Password Policy (spec Section 8.1)

- [x] bcrypt cost 12 -- `internal/auth/password.go` line 11: `const bcryptCost = 12`; used in `HashPassword()`
- [x] Minimum 12 characters enforced -- `internal/auth/password.go` `ValidatePasswordPolicy()` checks `len(password) < 12`
- [x] Complexity requirements (upper, lower, digit, special) -- `internal/auth/password.go` `ValidatePasswordPolicy()` checks all four classes
- [x] Reject common passwords -- `internal/auth/common_passwords.go` embeds 1,041 common passwords via `//go:embed`; checked in `ValidatePasswordPolicy()` via `isCommonPassword()`
- [x] Constant-time comparison -- `internal/auth/password.go` `VerifyPassword()` uses `bcrypt.CompareHashAndPassword` (inherently constant-time)
- [x] Plaintext never stored or returned -- `internal/store/users.go` `PasswordHash` field has `json:"-"` tag

## Encryption at Rest (spec Section 8.2)

- [x] AES-256-GCM for MCP server credentials -- `internal/auth/crypto.go` implements `Encrypt()`/`Decrypt()` with AES-256-GCM
- [x] Unique random 12-byte nonce per encryption -- `internal/auth/crypto.go`: `nonce := make([]byte, gcm.NonceSize())` filled with `crypto/rand`
- [x] 32-byte key requirement enforced -- `internal/auth/crypto.go` validates `len(key) != 32`
- [x] Credential encryption key from env var -- `internal/config/config.go` reads `CREDENTIAL_ENCRYPTION_KEY`; required or startup fails

## HTTP Security Headers (spec Section 8.3)

- [x] Strict-Transport-Security (HSTS) -- `internal/api/router.go` `securityHeaders` middleware: `max-age=63072000; includeSubDomains`
- [x] Content-Security-Policy -- `securityHeaders`: `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'`
- [x] X-Frame-Options: DENY -- `securityHeaders`: `DENY`
- [x] X-Content-Type-Options: nosniff -- `securityHeaders`: `nosniff`
- [x] Referrer-Policy -- `securityHeaders`: `strict-origin-when-cross-origin`
- [x] Permissions-Policy -- `securityHeaders`: `camera=(), microphone=(), geolocation=()`

## CSRF Protection (spec Section 3.11)

- [x] Double-submit cookie pattern -- `internal/auth/csrf.go`: `__Host-csrf` cookie + `X-CSRF-Token` header validated via `ValidateCSRF()`
- [x] Validated on all POST/PUT/PATCH/DELETE for session auth -- `internal/api/middleware.go` `AuthMiddleware()`
- [x] Not required for API key auth -- API key path skips CSRF validation entirely
- [x] Constant-time comparison -- `internal/auth/csrf.go`: `subtle.ConstantTimeCompare`

## Session Management (spec Section 3.5)

- [x] 8-hour session TTL -- `internal/auth/session.go`: `SessionTTL = 8 * time.Hour`
- [x] HttpOnly, Secure, SameSite=Lax cookies -- `internal/auth/handler.go` `setSessionCookie()`
- [x] `__Host-` cookie prefix -- `internal/auth/session.go`: `SessionCookieName = "__Host-session"`
- [x] Session cleanup goroutine -- `cmd/server/main.go`: 10-minute ticker calling `DeleteExpired()`
- [x] Session touch on activity -- `internal/api/middleware.go`: `sessions.TouchSession()` called on each authenticated request
- [x] Session invalidation on password change -- `internal/auth/handler.go`: `DeleteByUserID()` called after password update

## Rate Limiting (spec Section 8.4)

- [x] Login: 5 attempts per 15 minutes per IP -- `router.go`: `cfg.RateLimiter.Middleware(5, 15*time.Minute, ...)` on `POST /auth/login`
- [x] Google OAuth: 10 requests per 15 minutes per IP -- `router.go`: `cfg.RateLimiter.Middleware(10, 15*time.Minute, ...)` on `/auth/google/*`
- [x] API mutations: 60/min per user -- `router.go`: `methodAwareRateLimiter()` applies 60/min for POST/PUT/PATCH/DELETE
- [x] API reads: 300/min per user -- `router.go`: `methodAwareRateLimiter()` applies 300/min for GET
- [x] Discovery: 10/min per user/key -- `router.go`: dedicated limiter on `GET /api/v1/discovery`
- [x] 429 response with Retry-After header -- `internal/ratelimit/limiter.go` Middleware
- [x] X-RateLimit-* headers on all rate-limited responses -- `internal/ratelimit/limiter.go` Middleware

## Login Lockout (spec Section 3.5 / 8.1)

- [x] Failed login tracking -- `internal/auth/handler.go`: `IncrementFailedLogins()` called on password mismatch
- [x] Lockout after 5 failed attempts -- `internal/auth/handler.go`: threshold of 5
- [x] Escalating lockout duration -- `internal/auth/handler.go`: `lockoutDuration()` returns 15m, 30m, 60m, 24h
- [x] Account lock check on login -- `internal/auth/handler.go`: checks `LockedUntil` before proceeding
- [x] Reset on successful login -- `internal/auth/handler.go`: `ResetFailedLogins()` called on success
- [x] No info leakage -- locked/inactive/wrong-method all return generic "invalid credentials"

## Audit Logging (spec Section 8.8)

- [x] All mutations recorded -- audit store calls in all 12 handler files
- [x] Auth events recorded -- `internal/auth/handler.go`: login, login_failed, logout, oauth_login, password_change, google_unlink
- [x] Actor, action, resource_type, resource_id, IP captured -- `AuditRecord` struct
- [x] Append-only (no delete API) -- no DELETE endpoint exists for `/api/v1/audit-log`

## Input Validation (spec Section 8.5)

- [x] Agent ID regex `/^[a-z][a-z0-9_]{1,49}$/` -- `internal/api/agents.go`: `agentIDRegex` validated on Create
- [x] Tool name max 100 chars -- `internal/api/agents.go`: `validateAgentTools()` checks `len(t.Name) > 100`
- [x] System prompt max 100KB -- `internal/api/agents.go`: Create and Update check `len(req.SystemPrompt) > 100*1024`; also in `prompts.go`
- [x] JSON body max 1MB -- `internal/api/middleware.go`: `MaxBodySize(1 << 20)` middleware on `/api/v1/*`
- [x] Webhook URL format + max 2000 chars -- `internal/api/webhooks.go`: `url.Parse()` + length check
- [x] MCP server URL max 2000 chars -- `internal/api/mcp_servers.go`: endpoint length validation in handler
- [x] SQL injection prevention -- all `internal/store/` files use pgx parameterized queries (`$1`, `$2`, etc.)

## Secrets Protection

- [x] `password_hash` uses `json:"-"` -- never in API responses
- [x] API key hashes stored as SHA-256 -- `internal/auth/apikey.go` `HashAPIKey()`
- [x] API key `key_hash` uses `json:"-"` -- never in API responses
- [x] Raw API key only shown once at creation
- [x] Webhook secret uses `json:"-"`
- [x] MCP auth_credential uses `json:"-"`
- [x] CSRF token uses `json:"-"`
- [x] OAuth tokens use `json:"-"`

## CORS (spec Section 8.7)

- [x] Same-origin CORS middleware -- `internal/api/middleware.go` `CORSMiddleware()`: compares `Origin` against `r.Host`, sets `Access-Control-Allow-*` headers only for same-origin
- [x] Allowed methods: GET, POST, PUT, PATCH, DELETE
- [x] Allowed headers: Authorization, Content-Type, X-CSRF-Token, If-Match
- [x] Max-Age: 3600
- [x] Preflight OPTIONS returns 204

## Dependency Security (spec Section 8.9)

- [x] Minimal dependencies -- only spec-mandated packages (chi, pgx, bcrypt, golang-migrate)
- [x] No ORM, no external crypto libraries -- stdlib crypto only
- [x] Non-root container user (UID 1001) -- `Dockerfile`: `adduser -D -u 1001 registry` + `USER 1001`
- [x] Read-only root filesystem -- `deployment/compose.yaml`: `read_only: true` + `tmpfs: /tmp:size=10M`

---

## Summary

| Category | Passed | Total |
|----------|--------|-------|
| Password Policy | 6 | 6 |
| Encryption at Rest | 4 | 4 |
| HTTP Security Headers | 6 | 6 |
| CSRF Protection | 4 | 4 |
| Session Management | 6 | 6 |
| Rate Limiting | 7 | 7 |
| Login Lockout | 6 | 6 |
| Audit Logging | 4 | 4 |
| Input Validation | 7 | 7 |
| Secrets Protection | 8 | 8 |
| CORS | 5 | 5 |
| Dependency Security | 4 | 4 |
| **Total** | **67** | **67** |

All spec Section 8 security requirements are now implemented and verified.
