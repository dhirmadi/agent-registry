# Authentication & Authorization

> How users and services authenticate with the Agentic Registry, and how permissions work.

---

## Overview

The Agentic Registry supports three authentication methods. They coexist cleanly — a single deployment can use all three simultaneously.

| Method | Transport | Use Case |
|--------|-----------|----------|
| **Username + Password** | Session cookie | Bootstrap, local development, fallback |
| **Google OAuth 2.0** | Session cookie (via PKCE flow) | Production SSO for admin users |
| **API Key** | `Authorization: Bearer` header | Service-to-service (BFF, CI, scripts) |

---

## Password Authentication

### Login

```http
POST /auth/login
Content-Type: application/json

{
  "username": "admin",
  "password": "your-secure-password-123!"
}
```

**Success (200):** Sets a `__Host-session` cookie with the following attributes:
- `HttpOnly` — Not accessible to JavaScript
- `Secure` — Only sent over HTTPS
- `SameSite=Lax` — CSRF mitigation
- `Path=/` — Available to all routes

**Failure (401):** `"Invalid credentials"` — no information leakage about which field was wrong.

**Locked (423):** After 5 consecutive failed attempts, the account locks for 15 minutes. Subsequent failures escalate: 30 min, 60 min, up to 24 hours.

### Default Admin Account

On first boot (when the `users` table is empty), the server creates:

```
Username:        admin
Password:        admin
Role:            admin
Must change:     true
```

The admin GUI forces a password change before any other action when `must_change_password` is `true`.

### Password Policy

All passwords must meet:
- Minimum 12 characters
- At least 1 uppercase letter
- At least 1 lowercase letter
- At least 1 digit
- At least 1 special character
- Not in the common password blocklist (1,041 entries)

### Brute-Force Protection

| Consecutive Failures | Lockout Duration |
|---------------------|-----------------|
| 5 | 15 minutes |
| 6 | 30 minutes |
| 7 | 60 minutes |
| 8+ | 24 hours |

Failed login count resets on successful authentication.

### Session Lifecycle

- **TTL:** 8 hours from creation
- **Touch:** `last_seen` updated on each authenticated request
- **Cleanup:** Background goroutine deletes expired sessions every 10 minutes
- **Invalidation:** All sessions destroyed on password change

### Logout

```http
POST /auth/logout
Cookie: __Host-session=<id>
X-CSRF-Token: <token>
```

Destroys the server-side session and clears the cookie.

### Current User

```http
GET /auth/me
Cookie: __Host-session=<id>
```

Returns the authenticated user's profile (works with both session and API key auth).

---

## Google OAuth 2.0

### Prerequisites

Set two environment variables:
```bash
GOOGLE_OAUTH_CLIENT_ID=your-client-id.apps.googleusercontent.com
GOOGLE_OAUTH_CLIENT_SECRET=your-client-secret
```

If these are not set, the Google login option is hidden in the admin GUI.

### Flow

1. **User clicks "Sign in with Google"** in the admin GUI
2. **Frontend redirects** to `GET /auth/google/start`
3. **Server generates** a PKCE code verifier, encrypts state (AES-256-GCM), sets a state cookie, and redirects to Google
4. **User authenticates** with Google
5. **Google redirects** back to `GET /auth/google/callback` with an authorization code
6. **Server exchanges** the code for tokens, verifies state, and:
   - If the Google email matches an existing user → creates a session (links account)
   - If no match → creates a new user with `auth_method: "google"` and `role: "viewer"`

### Account Linking

- A password user who authenticates via Google gets their account linked automatically (matched by email)
- `POST /auth/unlink-google` removes the Google connection and sets `auth_method: "password"`
- Users can use both methods simultaneously (`auth_method: "both"`)

### Rate Limiting

Google OAuth endpoints are rate-limited to 10 requests per 15 minutes per IP.

---

## API Keys

API keys are designed for service-to-service communication — the BFF reading agent configuration, CI pipelines running health checks, scripts managing resources.

### Create a Key

```http
POST /api/v1/api-keys
Cookie: __Host-session=<id>
X-CSRF-Token: <token>
Content-Type: application/json

{
  "name": "bff-production",
  "scopes": ["read", "write"]
}
```

**Response (201):**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "name": "bff-production",
    "key": "rk_live_abc123...",
    "key_prefix": "rk_live_abc1",
    "scopes": ["read", "write"],
    "created_at": "2026-02-15T00:00:00Z"
  }
}
```

> **The full key is shown only once.** It is hashed with SHA-256 before storage and can never be retrieved again.

### Using a Key

```http
GET /api/v1/agents
Authorization: Bearer rk_live_abc123...
```

No session cookie or CSRF token required. API key auth bypasses CSRF validation entirely.

### Key Format

All keys use the prefix `rk_live_` followed by cryptographically random bytes. The prefix enables quick identification in logs and makes accidental exposure detectable.

### Scopes

| Scope | Permissions |
|-------|-------------|
| `read` | GET endpoints only |
| `write` | GET + POST/PUT/PATCH/DELETE |
| `admin` | All endpoints including user management |

### Management

```http
GET /api/v1/api-keys           # List keys (admin sees all, others see own)
DELETE /api/v1/api-keys/{keyId} # Revoke a key
```

---

## CSRF Protection

All non-GET requests authenticated via session cookies require a CSRF token.

### How It Works

1. On login, the server sets a `__Host-csrf` cookie containing a random token
2. The frontend reads this cookie and includes the value as an `X-CSRF-Token` header on every mutation
3. The server validates that the header matches the cookie using constant-time comparison

### Exemptions

- **API key requests** — CSRF is not applicable (no cookies involved)
- **GET requests** — Safe methods are exempt
- **Login/OAuth** — Public endpoints before authentication

---

## Roles & Permissions

### Role Hierarchy

| Role | Read Resources | Write Resources | Manage Users | Manage All Keys |
|------|---------------|----------------|-------------|----------------|
| `viewer` | Yes | — | — | — |
| `editor` | Yes | Yes | — | — |
| `admin` | Yes | Yes | Yes | Yes |

### Resource-Level Permissions

| Resource | viewer | editor | admin |
|----------|--------|--------|-------|
| Agents | Read | Full CRUD | Full CRUD |
| Prompts | Read | Full CRUD | Full CRUD |
| MCP Servers | — | — | Full CRUD |
| Trust Rules | — | Full CRUD | Full CRUD |
| Trust Defaults | — | — | Full CRUD |
| Model Config | — | — | Full CRUD |
| Webhooks | — | — | Full CRUD |
| Users | — | — | Full CRUD |
| API Keys | Own keys | Own keys | All keys |
| Audit Log | — | — | Read |
| Discovery | Read | Read | Read |

---

## Security Summary

| Control | Implementation |
|---------|---------------|
| Password storage | bcrypt cost 12, `json:"-"` on hash fields |
| API key storage | SHA-256 hash, prefix retained for identification |
| Session cookies | `__Host-` prefix, HttpOnly, Secure, SameSite=Lax |
| CSRF | Double-submit cookie, constant-time comparison |
| Brute-force | Escalating lockout (15 min → 24 hours) |
| OAuth state | AES-256-GCM encrypted, single-use |
| Rate limiting | Per-IP on login/OAuth, per-user on API |
| Secret exposure | MCP credentials AES-256-GCM encrypted at rest |
| Audit trail | Every mutation logged with actor and IP |
