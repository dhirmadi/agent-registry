---
name: security-audit
description: Run a security audit on the Agentic Registry. Checks auth, encryption, input validation, headers, and OWASP compliance. Use when the user says "security audit", "check security", "audit auth", or "pen test".
---

# Security Audit

## Audit Scope

### Authentication
- [ ] bcrypt cost 12 with constant-time comparison
- [ ] Password policy enforced (min length, complexity, common-password check)
- [ ] Brute force: lockout after 5 failures, doubling backoff
- [ ] Session cookies: `__Host-` prefix, HttpOnly, Secure, SameSite=Lax
- [ ] Session expiry: 8h absolute, 30min idle
- [ ] OAuth: state parameter validated, PKCE used
- [ ] API keys: SHA-256 hashed, `areg_` prefix, plaintext shown once only
- [ ] `must_change_pass` enforced on first-boot admin

### Authorization
- [ ] Role checks on every mutation (viewer/editor/admin)
- [ ] API key scope validation (read/write/admin)
- [ ] No privilege escalation paths

### Data Protection
- [ ] `password_hash` uses `json:"-"`
- [ ] MCP `auth_credential` encrypted AES-256-GCM at rest
- [ ] No secrets in logs or API responses

### Input Validation
- [ ] Parameterized SQL queries (`$1` placeholders via pgx)
- [ ] JSON body size limits
- [ ] UUID parameters validated before use
- [ ] MCP endpoint SSRF protection (block internal IPs)

### Headers & Transport
- [ ] X-Content-Type-Options, X-Frame-Options, Strict-Transport-Security
- [ ] CORS restrictive (not `*`)
- [ ] CSRF token on all non-GET session endpoints

## Workflow

1. Read implementation of module under audit
2. Cross-reference with spec Sections 3 and 8
3. Document findings: `| Severity | Location | Issue | Fix |`
4. Write failing test exposing each vulnerability
5. Fix, verify test passes, run full suite with `-race`

## Severity Levels

- **CRITICAL**: Auth bypass, data exposure, RCE
- **HIGH**: Privilege escalation, missing encryption
- **MEDIUM**: Missing rate limit, weak validation
- **LOW**: Missing headers, informational
