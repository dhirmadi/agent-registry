# Security Checklist

Verification of spec Section 8 security requirements for Agentic Registry.

Last reviewed: 2026-02-14

---

## Password Policy (spec Section 8.1)

- [x] bcrypt cost 12 -- `internal/auth/password.go` line 11: `const bcryptCost = 12`; used in `HashPassword()`
- [x] Minimum 12 characters enforced -- `internal/auth/password.go` `ValidatePasswordPolicy()` checks `len(password) < 12`
- [x] Complexity requirements (upper, lower, digit, special) -- `internal/auth/password.go` `ValidatePasswordPolicy()` checks all four classes
- [ ] Reject top 10,000 common passwords -- spec Section 8.1 requires an embedded common-passwords list check; **not implemented** in `ValidatePasswordPolicy()`
- [x] Constant-time comparison -- `internal/auth/password.go` `VerifyPassword()` uses `bcrypt.CompareHashAndPassword` (inherently constant-time)
- [x] Plaintext never stored or returned -- `internal/store/users.go` `PasswordHash` field has `json:"-"` tag

## Encryption at Rest (spec Section 8.2)

- [x] AES-256-GCM for MCP server credentials -- `internal/auth/crypto.go` implements `Encrypt()`/`Decrypt()` with AES-256-GCM; called from `internal/api/mcp_servers.go` lines 246, 391
- [x] Unique random 12-byte nonce per encryption -- `internal/auth/crypto.go` line 29: `nonce := make([]byte, gcm.NonceSize())` filled with `crypto/rand`
- [x] 32-byte key requirement enforced -- `internal/auth/crypto.go` lines 16, 42 both validate `len(key) != 32`
- [x] Credential encryption key from env var -- `internal/config/config.go` line 51 reads `CREDENTIAL_ENCRYPTION_KEY`; required or startup fails

## HTTP Security Headers (spec Section 8.3)

- [x] Strict-Transport-Security (HSTS) -- `internal/api/router.go` `securityHeaders` middleware: `max-age=63072000; includeSubDomains`
- [x] Content-Security-Policy -- `internal/api/router.go` `securityHeaders`: `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'`
- [x] X-Frame-Options: DENY -- `internal/api/router.go` `securityHeaders`: `DENY`
- [x] X-Content-Type-Options: nosniff -- `internal/api/router.go` `securityHeaders`: `nosniff`
- [x] Referrer-Policy -- `internal/api/router.go` `securityHeaders`: `strict-origin-when-cross-origin`
- [x] Permissions-Policy -- `internal/api/router.go` `securityHeaders`: `camera=(), microphone=(), geolocation=()`

## CSRF Protection (spec Section 3.11)

- [x] Double-submit cookie pattern -- `internal/auth/csrf.go`: `__Host-csrf` cookie + `X-CSRF-Token` header validated via `ValidateCSRF()`
- [x] Validated on all POST/PUT/PATCH/DELETE for session auth -- `internal/api/middleware.go` `AuthMiddleware()` lines 56-65: checks CSRF for mutation methods when session-authenticated
- [x] Not required for API key auth -- `internal/api/middleware.go` lines 31-43: API key path skips CSRF validation entirely
- [x] Constant-time comparison -- `internal/auth/csrf.go` line 60: `subtle.ConstantTimeCompare`

## Session Management (spec Section 3.5)

- [x] 8-hour session TTL -- `internal/auth/session.go` line 13: `SessionTTL = 8 * time.Hour`
- [x] HttpOnly, Secure, SameSite=Lax cookies -- `internal/auth/handler.go` `setSessionCookie()` lines 395-403: all three flags set
- [x] `__Host-` cookie prefix -- `internal/auth/session.go` line 15: `SessionCookieName = "__Host-session"`
- [x] Session cleanup goroutine -- `cmd/server/main.go` lines 131-148: 10-minute ticker calling `DeleteExpired()`
- [x] Session touch on activity -- `internal/api/middleware.go` line 69: `sessions.TouchSession()` called on each authenticated request
- [x] Session invalidation on password change -- `internal/auth/handler.go` line 374: `DeleteByUserID()` called after password update

## Rate Limiting (spec Section 8.4)

- [x] Login rate limiting present -- `internal/api/router.go` lines 71-74: rate limiter applied to `POST /auth/login`
- [ ] Login: 5 attempts per 15 minutes per IP+username -- **partial**: implementation uses 5 per 1 minute per IP only (spec requires 15-minute window and per-username scoping)
- [x] API rate limiting present -- `internal/api/router.go` lines 101-108: rate limiter applied to `/api/v1/*`
- [ ] API: 60 mutations/min + 300 reads/min per user -- **partial**: implementation uses a blanket 100/min for all API routes (spec requires separate limits for mutations vs reads)
- [ ] Discovery endpoint: 10/min per API key -- **not implemented** as a separate rate limit
- [ ] Google OAuth: 10/15min per IP -- **not implemented**
- [x] 429 response with Retry-After header -- `internal/ratelimit/limiter.go` lines 73-78: sets `Retry-After` header on 429
- [x] X-RateLimit-* headers on all responses -- `internal/ratelimit/limiter.go` lines 68-70: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset` set on every request through the limiter

## Login Lockout (spec Section 3.5 / 8.1)

- [x] Failed login tracking -- `internal/auth/handler.go` line 206: `IncrementFailedLogins()` called on password mismatch
- [x] Lockout after 5 failed attempts -- `internal/auth/handler.go` lines 209-212: threshold of 5
- [x] Escalating lockout duration -- `internal/auth/handler.go` lines 382-392: `lockoutDuration()` returns 15m, 30m, 60m, 24h
- [x] Account lock check on login -- `internal/auth/handler.go` lines 185-189: checks `LockedUntil` before proceeding
- [x] Reset on successful login -- `internal/auth/handler.go` line 221: `ResetFailedLogins()` called on success
- [x] No info leakage -- `internal/auth/handler.go` lines 188, 193, 199: locked/inactive/wrong-method all return generic "invalid credentials"

## Audit Logging (spec Section 8.8)

- [x] All mutations recorded -- audit store calls found in all 12 handler files: `agents.go`, `prompts.go`, `mcp_servers.go`, `trust_rules.go`, `trust_defaults.go`, `trigger_rules.go`, `model_config.go`, `context_config.go`, `signal_config.go`, `webhooks.go`, `api_keys.go`, `users.go`
- [x] Auth events recorded -- `internal/auth/handler.go`: login, login_failed, logout, oauth_login, password_change, google_unlink
- [x] Actor, action, resource_type, resource_id, IP captured -- `internal/auth/handler.go` `AuditRecord` struct and `auditLog()` helper at line 418
- [x] Append-only (no delete API) -- no DELETE endpoint exists for `/api/v1/audit-log`

## Input Validation (spec Section 8.5)

- [x] URL validation on webhook URLs -- `internal/api/webhooks.go` validates URL is non-empty
- [x] JSON body decoding -- all handlers use `json.NewDecoder(r.Body).Decode()` with explicit error handling
- [x] SQL injection prevention via parameterized queries -- all `internal/store/` files use pgx parameterized queries (`$1`, `$2`, etc.)
- [ ] Agent ID regex validation (`/^[a-z][a-z0-9_]{1,49}$/`) -- not verified in handler (spec Section 8.5 requires this pattern)
- [ ] Tool pattern max 100 chars + glob safety -- not verified
- [ ] System prompt max 100KB -- not verified
- [ ] JSON fields max 1MB -- not verified

## Secrets Protection

- [x] `password_hash` uses `json:"-"` -- `internal/store/users.go` line 21: never in API responses
- [x] API key hashes stored as SHA-256 -- `internal/auth/apikey.go` `HashAPIKey()` uses `sha256.Sum256()`
- [x] API key `key_hash` uses `json:"-"` -- `internal/store/api_keys.go` line 21
- [x] Raw API key only shown once at creation -- `internal/api/api_keys.go` `Create()` returns plaintext in response; subsequent List/Get never includes it
- [x] Webhook secret uses `json:"-"` -- `internal/store/webhooks.go` line 19: `Secret string json:"-"`
- [x] MCP auth_credential uses `json:"-"` -- `internal/store/mcp_servers.go` line 22
- [x] CSRF token uses `json:"-"` -- `internal/auth/session.go` line 24 and `internal/store/sessions.go` line 19
- [x] OAuth tokens use `json:"-"` -- `internal/store/oauth_connections.go` lines 23-25

## CORS (spec Section 8.7)

- [ ] CORS middleware configured -- **not implemented**; spec Section 8.7 requires same-origin CORS configuration with specific allowed methods and headers

## Dependency Security (spec Section 8.9)

- [x] Minimal dependencies -- project uses only spec-mandated packages (chi, pgx, bcrypt, golang-migrate)
- [x] No ORM, no external crypto libraries -- verified; stdlib crypto only
- [ ] Non-root container user (UID 1001) -- not verified (requires Dockerfile review)
- [ ] Read-only root filesystem -- not verified (requires Dockerfile review)

---

## Summary

| Category | Passed | Failed | Total |
|----------|--------|--------|-------|
| Password Policy | 5 | 1 | 6 |
| Encryption at Rest | 4 | 0 | 4 |
| HTTP Security Headers | 6 | 0 | 6 |
| CSRF Protection | 4 | 0 | 4 |
| Session Management | 6 | 0 | 6 |
| Rate Limiting | 4 | 4 | 8 |
| Login Lockout | 6 | 0 | 6 |
| Audit Logging | 4 | 0 | 4 |
| Input Validation | 3 | 4 | 7 |
| Secrets Protection | 8 | 0 | 8 |
| CORS | 0 | 1 | 1 |
| Dependency Security | 2 | 2 | 4 |
| **Total** | **52** | **12** | **64** |

### Open Items Requiring Attention

1. **Common password rejection** -- spec Section 8.1 requires rejecting top 10,000 common passwords; `ValidatePasswordPolicy()` does not check against a list.
2. **Rate limiting granularity** -- login window is 1 minute (spec says 15 minutes); no per-username scoping; API mutations and reads share a single 100/min limit (spec splits 60/min mutations and 300/min reads); Google OAuth and discovery have no dedicated limits.
3. **Input validation hardening** -- agent ID regex, tool pattern length, system prompt size, and JSON field size limits are not enforced at the handler level.
4. **CORS middleware** -- not configured; spec Section 8.7 defines same-origin CORS configuration.
5. **Container hardening** -- non-root user and read-only filesystem not verified from source alone.
