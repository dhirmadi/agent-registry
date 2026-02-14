# PROJECT_AUDIT.md — Repository-Wide QA Audit

**Date:** 2026-02-14
**Auditors:** 4-agent swarm (Security Auditor, Backend Specialist, Frontend Specialist, QA Engineer)
**Lead:** Claude Code (Lead Software Architect)
**Scope:** Full repository — 22,640 lines Go + 10,657 lines TypeScript + 16 SQL migration pairs

---

## Executive Summary

| Severity | Count | Breakdown |
|----------|-------|-----------|
| CRITICAL | 7 | 2 security, 2 test infrastructure, 2 frontend, 1 concurrency |
| HIGH | 13 | 4 security, 3 error handling, 2 data integrity, 4 test gaps |
| MEDIUM | 18 | Mixed: UX, architecture, coverage |
| LOW/INFO | 12 | Code quality, minor UX |
| **Total** | **50** | Deduplicated across all 4 auditors |

**Overall Health: AMBER — fundamentals are strong, but critical integration gaps exist.**

The cryptographic primitives are correct (bcrypt, AES-256-GCM, HMAC-SHA256, crypto/rand). All SQL uses parameterized queries — no injection vectors. RBAC is properly enforced. Security headers are comprehensive. However, a critical security fix from the previous audit was never wired into production, the entire auth test suite is broken (51 tests not compiling), and the store layer has zero direct test coverage.

---

## Cross-Cutting Pattern: `must_change_pass` Enforcement Is Broken End-to-End

The most severe finding spans all four audit domains. The previous security audit (2026-02-14) identified that the default admin (`admin`/`admin`) could access all APIs without changing their password. A fix was written — but **never completed**:

| Layer | Status | Detail |
|-------|--------|--------|
| Backend middleware | Written, never wired | `MustChangePassMiddleware` exists in `middleware.go:126` but `router.go` never applies it |
| Backend store | Missing | `UserLookup.GetMustChangePass()` interface has no store implementation |
| Frontend AuthContext | Broken | `/auth/me` session restore doesn't check `must_change_pass` flag |
| Frontend ProtectedRoute | Bypassable | `mustChangePassword` is only set during `login()`, not on page refresh |
| Auth tests | Not running | Cookie name constants→functions migration broke all 51 auth tests |

**Impact:** Default admin credentials grant full unrestricted API access in production.

---

## 1. Security Findings

### CRITICAL

**S-CRIT-1: MustChangePassMiddleware never wired into router**
- **Found by:** Security Auditor, Backend Specialist, Frontend Specialist
- **Files:** `internal/api/router.go`, `internal/api/middleware.go:126`, `cmd/server/main.go`
- **Description:** The `MustChangePassMiddleware` was written as a fix for the previous audit's top finding but was never integrated into `NewRouter()`. The `RouterConfig` struct has no `UserLookup` field. The middleware is tested in isolation but has zero production effect.
- **Impact:** Default admin (`admin`/`admin`, `must_change_pass=true`) has full API access without changing password.
- **Fix:** See [Diff-Fix S-CRIT-1](#diff-fix-s-crit-1) below.

### HIGH

**S-HIGH-1: CSRF validation bypass via fallback logic**
- **Found by:** Security Auditor, Backend Specialist
- **File:** `internal/api/middleware.go:56-66`
- **Description:** When `auth.ValidateCSRF(r)` fails, the middleware falls through to checking the CSRF cookie value against the stored session token using non-constant-time comparison. This defeats the double-submit cookie pattern — if an attacker can cause the cookie to be sent (e.g., via same-site subdomain), the header is never required.
- **Impact:** CSRF protection can be bypassed; timing side-channel leaks stored token.
- **Fix:** Remove fallback. If `auth.ValidateCSRF(r)` fails, always reject with 403.

**S-HIGH-2: Webhook URL SSRF — no private IP validation**
- **Found by:** Security Auditor, Backend Specialist
- **File:** `internal/api/webhooks.go:63-109`, `internal/notify/dispatcher.go:155`
- **Description:** Webhook creation validates scheme (HTTP/HTTPS) but never calls `isPrivateHost()` or `validateEndpointURL()`. MCP server endpoints get SSRF protection; webhooks do not. The dispatcher makes HTTP POST requests to these URLs.
- **Impact:** Admin can register webhook to `http://169.254.169.254/` (AWS metadata), `http://localhost:5432/`, or internal network, enabling SSRF.
- **Fix:** Reuse `validateEndpointURL()` from `mcp_servers.go` in webhook Create handler.

**S-HIGH-3: CSRF token parsing truncates base64 values**
- **Found by:** Frontend Specialist
- **File:** `web/src/api/client.ts:8`
- **Description:** `getCSRFToken()` uses `match.split('=')[1]` which only gets the first segment after `=`. If the token contains `=` (base64), the value is truncated, causing silent CSRF failures.
- **Fix:** Use `match.substring(match.indexOf('=') + 1)`.

### MEDIUM

**S-MED-1: Agent PATCH type assertion panic — DoS vector**
- **Found by:** Security Auditor, Backend Specialist
- **File:** `internal/store/agents.go:243-274`
- **Description:** Unchecked type assertions (`v.(string)`, `v.(bool)`) on user-controlled PATCH input. Sending `"name": 123` panics the goroutine.
- **Fix:** Use comma-ok pattern for all type assertions.

**S-MED-2: Rate limiter unbounded memory growth**
- **Found by:** Security Auditor, Backend Specialist
- **File:** `internal/ratelimit/limiter.go`
- **Description:** Expired rate limit buckets are never cleaned up. Combined with `middleware.RealIP` trusting `X-Forwarded-For`, an attacker can forge unique IPs to create unlimited buckets.
- **Fix:** Add periodic cleanup goroutine; cap maximum bucket count.

**S-MED-3: Credential encryption silently skipped on wrong key length**
- **Found by:** Security Auditor
- **File:** `internal/api/mcp_servers.go:248,393`, `internal/config/config.go`
- **Description:** If `CREDENTIAL_ENCRYPTION_KEY` is not exactly 32 bytes, credentials are silently dropped (stored as empty string). Config validates non-empty but not length.
- **Fix:** Validate key length at startup: `len(key) != 32` → fatal error.

**S-MED-4: SESSION_SECRET loaded but never used**
- **Found by:** Security Auditor
- **File:** `internal/config/config.go:47-49`
- **Description:** `SESSION_SECRET` is a required env var but is never referenced anywhere. Operators must set it but it provides no security benefit.
- **Fix:** Either remove from required config or use it for HMAC session signing.

### LOW / INFO

**S-LOW-1:** X-Forwarded-For trusted without validation — audit log IP poisoning (`users.go:380`, `handler.go:434`)
**S-LOW-2:** CORS origin comparison doesn't normalize ports (`middleware.go:184`)
**S-LOW-3:** Webhook HMAC secrets stored as plaintext in DB (`store/webhooks.go:39`)

---

## 2. Backend Findings

### CRITICAL

**B-CRIT-1: Webhook dispatcher race condition on shutdown**
- **File:** `internal/notify/dispatcher.go:98-116`
- **Description:** When `d.done` is closed, all workers wake simultaneously and enter a drain loop with a `default` branch that exits when the channel appears empty. Between `close(d.done)` and workers completing, `Dispatch()` can still send events that are never consumed.
- **Impact:** Webhook events silently dropped during graceful shutdown.
- **Fix:** Set atomic flag to stop accepting new events → close `d.done` → close `d.eventCh` → workers range over closed channel to drain.

### HIGH

**B-HIGH-1: Swallowed error on json.Unmarshal in subscriptionLoaderAdapter**
- **File:** `cmd/server/main.go:677`
- **Description:** `json.Unmarshal(s.Events, &events)` error ignored. Malformed JSON in subscription events causes silent mismatch failures.
- **Fix:** Check error; skip subscription with warning log.

**B-HIGH-2: Audit log insertion errors silently swallowed**
- **Files:** All 13 handler files — `agents.go:512`, `prompts.go:240`, `mcp_servers.go:464`, `trust_rules.go:176`, `trust_defaults.go:112`, `trigger_rules.go:329`, `signal_config.go:120`, `model_config.go:180`, `context_config.go:155`, `webhooks.go:134`, `users.go:364`, `api_keys.go:191`, `auth/handler.go:418`
- **Description:** Every `auditLog` helper calls `h.audit.Insert(...)` but never checks the error. Audit failures are invisible.
- **Impact:** Compliance requirement silently violated.
- **Fix:** Log warning on audit insert failure; consider failing the mutation if audit is mandatory.

**B-HIGH-3: Agent/Prompt Get handler maps all store errors to 404**
- **Files:** `agents.go:244-246`, `prompts.go:98-100`, `prompts.go:116-119`
- **Description:** Any error from `GetByID()` returns 404. Database outage appears as "not found" to clients.
- **Fix:** Check `isNotFoundError(err)` first; fall back to `Internal()`.

**B-HIGH-4: No pagination upper bound**
- **Found by:** Backend Specialist, Security Auditor
- **Files:** `agents.go:264-268`, `prompts.go:73-80`, `users.go:92-99`
- **Description:** `limit` parameter has no cap. `?limit=999999999` causes full table scan + massive memory allocation.
- **Fix:** Cap at 200.

### MEDIUM

**B-MED-1:** MCP servers List has no pagination (`mcp_servers.go:298`)
**B-MED-2:** Agent Update handler overwrites `CreatedBy` with updating user (`agents.go:351`)
**B-MED-3:** HandleChangePassword invalidates ALL sessions including current (`auth/handler.go:374`)
**B-MED-4:** Dispatcher uses `context.Background()` for subscription loading (`dispatcher.go:119`)

### LOW

**B-LOW-1:** Duplicated `clientIP` function (`auth/handler.go:433` and `users.go:379`)
**B-LOW-2:** Duplicated `auditLog`/`dispatchEvent` helpers across all 12+ handlers
**B-LOW-3:** Custom `contains`/`searchString` instead of `strings.Contains` (`store/agents.go:435`, `main.go:654`)
**B-LOW-4:** `isStoreNotFound` uses fragile string formatting (`main.go:649`)
**B-LOW-5:** Security headers middleware defined twice (`middleware.go:204` and `router.go:393`)

---

## 3. Frontend Findings

### CRITICAL

**F-CRIT-1: AuthContext does not check `must_change_pass` on session restore**
- **File:** `web/src/auth/AuthContext.tsx:21-31`
- **Description:** The `useEffect` calling `/auth/me` on mount sets `user` but never sets `mustChangePassword`. A user with `must_change_pass=true` and a valid session can refresh the page to bypass the redirect to `/change-password`.
- **Impact:** Combined with S-CRIT-1 (server-side not enforced either), the password change requirement is completely ineffective.
- **Fix:** Have `/auth/me` return `must_change_password` field; set it in AuthContext on restore.

**F-CRIT-2: No global ErrorBoundary — unhandled errors crash the entire app**
- **File:** `web/src/App.tsx`
- **Description:** No React `ErrorBoundary` wraps the application. Any rendering exception white-screens the app with no recovery.
- **Fix:** Add `ErrorBoundary` component wrapping the route tree with "Something went wrong" fallback.

### HIGH

**F-HIGH-1:** ProtectedRoute bypass via page refresh — linked to F-CRIT-1 (`ProtectedRoute.tsx:26`)
**F-HIGH-2:** PromptsPage uses wrong response field names — `data.agents` and `data.prompts` instead of `data.items` (`PromptsPage.tsx:66,90`)
**F-HIGH-3:** 14+ catch blocks silently swallow errors across AgentsPage, TrustPage, TriggersPage, SignalsPage, WebhooksPage, APIKeysPage

### MEDIUM

**F-MED-1:** Webhook toggle doesn't send If-Match/ETag header (`WebhooksPage.tsx:114`)
**F-MED-2:** TriggersPage create form missing `workspace_id` (`TriggersPage.tsx:108`)
**F-MED-3:** Password validation rules duplicated across ChangePasswordPage and MyAccountPage
**F-MED-4:** AgentDetailPage `example_prompts` parsing ambiguous (`AgentDetailPage.tsx:93`)
**F-MED-5:** Navigation active state uses strict equality — child routes not highlighted (`AppLayout.tsx:35`)
**F-MED-6:** SignalsPage poll interval edit uses non-accessible `<span>` without keyboard support (`SignalsPage.tsx:154`)
**F-MED-7:** ToastNotifications has duplicate timeout mechanism (`ToastNotifications.tsx:55,74`)
**F-MED-8:** No loading/submitting indicator on many mutation buttons — double-click risk
**F-MED-9:** AuditLogPage shows "1-50 of 0" when empty (`AuditLogPage.tsx:229`)

### LOW

**F-LOW-1:** JsonEditor uses hardcoded `id="json-editor"` — duplicate ID if multiple instances (`JsonEditor.tsx:55`)
**F-LOW-2:** LoginPage Google OAuth link is plain `<a>` without loading indicator (`LoginPage.tsx:40`)
**F-LOW-3:** `api.delete` doesn't support ETag/If-Match (`client.ts:107`)
**F-LOW-4:** DiffViewer uses array index as React key (`DiffViewer.tsx:95,107`)

---

## 4. Test Coverage Findings

### CRITICAL

**T-CRIT-1: `internal/auth` test suite broken — 51 tests not compiling**
- **File:** `internal/auth/handler_test.go`, `csrf_test.go`, `oauth_test.go`, `session_test.go`
- **Description:** Cookie name identifiers changed from constants to functions (`SessionCookieName()`, `CSRFCookieName()`, `OAuthStateCookieName()`) but test files were NOT updated. 10+ type mismatch compile errors prevent all 51 auth tests from running. The entire authentication layer has zero active test coverage.
- **Fix:** See [Diff-Fix T-CRIT-1](#diff-fix-t-crit-1) below.

**T-CRIT-2: `internal/store/` has ZERO test files**
- **Location:** 15 store files, 0 test files
- **Description:** SQL query correctness, transaction isolation, constraint violations, and NULL handling are entirely untested. All handler tests use in-memory mocks that can diverge from real SQL behavior.

**T-CRIT-3: `internal/db/` has ZERO test files**
- **Location:** `internal/db/` (pool.go, migrate.go)
- **Description:** Connection pool setup and migration runner are untested.

### HIGH

**T-HIGH-1:** No API contract tests — frontend TypeScript types manually maintained with no validation against backend
**T-HIGH-2:** `web/src/api/client.ts` has zero tests — core communication layer untested
**T-HIGH-3:** 7 frontend components have no test files (ChangePasswordPage, ProtectedRoute, App, VersionTimeline, StatusBadge, JsonEditor, ToastNotifications)
**T-HIGH-4:** No cross-resource integration test (create agent → prompt → discover → webhook flow)
**T-HIGH-5:** Mock stores may hide SQL bugs — different behavior for ordering, precision, constraints

### MEDIUM

**T-MED-1:** Webhook concurrent subscription delivery untested
**T-MED-2:** Audit log SQL filtering untested (dynamic WHERE clauses)
**T-MED-3:** Agent seeder doesn't verify all 16 product agents
**T-MED-4:** Rate limiter concurrent access untested
**T-MED-5:** MCP credential encryption/decryption not tested end-to-end (mocks bypass it)
**T-MED-6:** Security test for editor privilege escalation is incomplete (`security_test.go:243`)

### LOW

**T-LOW-1:** Dispatcher retry test uses `time.Sleep(5s)` — slow and flaky (`dispatcher_test.go:213`)
**T-LOW-2:** Frontend tests have `act()` warnings in PromptsPage and MCPServersPage

---

## 5. Recommended Fix Priority

Ordered by severity × blast radius. Items marked with `*` have diff-fixes below.

| Priority | ID | Finding | Effort |
|----------|----|---------|--------|
| **P0** | T-CRIT-1* | Fix broken auth test suite (51 tests) | Small — update function calls |
| **P0** | S-CRIT-1* | Wire MustChangePassMiddleware into router | Medium — store method + router config + main.go |
| **P0** | F-CRIT-1 | AuthContext must_change_pass on session restore | Small — add field check |
| **P1** | S-HIGH-1 | Remove CSRF fallback logic | Small — delete fallback branch |
| **P1** | S-HIGH-2 | Webhook SSRF protection | Small — reuse existing validator |
| **P1** | B-CRIT-1 | Webhook dispatcher shutdown race | Medium — restructure shutdown sequence |
| **P1** | F-CRIT-2 | Add global ErrorBoundary | Small — new component |
| **P2** | B-HIGH-2 | Log audit insertion errors | Small — add error check |
| **P2** | B-HIGH-4 | Cap pagination limit | Small — add upper bound |
| **P2** | S-MED-1 | Fix type assertion panics in Agent PATCH | Small — comma-ok pattern |
| **P2** | S-MED-3 | Validate encryption key length at startup | Small — config check |
| **P2** | F-HIGH-2 | Fix PromptsPage response field names | Small — rename fields |
| **P3** | T-CRIT-2 | Add store-layer integration tests | Large — requires test DB setup |
| **P3** | T-HIGH-4 | Add cross-resource integration test | Medium |
| **P3** | S-HIGH-3 | Fix CSRF token base64 parsing | Small |
| **P3** | S-MED-2 | Rate limiter cleanup goroutine | Medium |
| **P4** | Remaining MEDIUM/LOW findings | Various | Mixed |

---

## 6. Diff-Fixes for CRITICAL Issues

### Diff-Fix T-CRIT-1: Fix Broken Auth Test Suite {#diff-fix-t-crit-1}

All auth test files reference cookie name identifiers as constants instead of function calls.

**Files to update:** `internal/auth/handler_test.go`, `internal/auth/csrf_test.go`, `internal/auth/oauth_test.go`, `internal/auth/session_test.go`

Apply the following replacement pattern across all test files:

```diff
- c.Name == SessionCookieName
+ c.Name == SessionCookieName()

- c.Name == CSRFCookieName
+ c.Name == CSRFCookieName()

- c.Name == OAuthStateCookieName
+ c.Name == OAuthStateCookieName()
```

This is a straightforward find-and-replace that will restore all 51 auth tests.

---

### Diff-Fix S-CRIT-1: Wire MustChangePassMiddleware into Router {#diff-fix-s-crit-1}

**Step 1: Add store method** — `internal/store/users.go`

```diff
+// GetMustChangePass returns whether the user must change their password.
+func (s *UserStore) GetMustChangePass(ctx context.Context, userID uuid.UUID) (bool, error) {
+	var mustChange bool
+	err := s.db.QueryRow(ctx,
+		`SELECT must_change_pass FROM users WHERE id = $1`, userID,
+	).Scan(&mustChange)
+	if err != nil {
+		return false, fmt.Errorf("get must_change_pass: %w", err)
+	}
+	return mustChange, nil
+}
```

**Step 2: Add UserLookup to RouterConfig** — `internal/api/router.go`

```diff
 type RouterConfig struct {
 	// ... existing fields ...
+	UserLookup UserLookup
 }

 func NewRouter(cfg RouterConfig) http.Handler {
 	// ... existing setup ...

 	r.Route("/api/v1", func(r chi.Router) {
 		r.Use(AuthMiddleware(cfg.Sessions, cfg.APIKeys))
+		r.Use(MustChangePassMiddleware(cfg.UserLookup))
 		// ... existing route registration ...
 	})
```

**Step 3: Wire adapter in main.go** — `cmd/server/main.go`

```diff
+// userLookupAdapter adapts UserStore to the api.UserLookup interface.
+type userLookupAdapter struct {
+	store *store.UserStore
+}
+
+func (a *userLookupAdapter) GetMustChangePass(ctx context.Context, userID uuid.UUID) (bool, error) {
+	return a.store.GetMustChangePass(ctx, userID)
+}

 func main() {
 	// ... existing setup ...
 	router := api.NewRouter(api.RouterConfig{
 		// ... existing fields ...
+		UserLookup: &userLookupAdapter{store: userStore},
 	})
```

---

### Diff-Fix S-HIGH-1: Remove CSRF Fallback

**File:** `internal/api/middleware.go`

```diff
 case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
-	if err := auth.ValidateCSRF(r); err != nil {
-		// Fallback: check if CSRF cookie matches stored token
-		csrfCookie, cookieErr := r.Cookie(auth.CSRFCookieName())
-		if cookieErr != nil {
-			RespondError(w, r, apierrors.Forbidden("CSRF validation failed"))
-			return
-		}
-		csrfToken, _ := auth.GetCSRFToken(r.Context())
-		if csrfCookie.Value != csrfToken {
-			RespondError(w, r, apierrors.Forbidden("CSRF validation failed"))
-			return
-		}
-	}
+	if err := auth.ValidateCSRF(r); err != nil {
+		RespondError(w, r, apierrors.Forbidden("CSRF validation failed"))
+		return
+	}
```

---

### Diff-Fix S-HIGH-2: Webhook SSRF Protection

**File:** `internal/api/webhooks.go`

```diff
 func (h *WebhookHandler) Create(w http.ResponseWriter, r *http.Request) {
 	// ... existing validation ...
+	if err := validateEndpointURL(req.URL); err != nil {
+		RespondError(w, r, apierrors.Validation("webhook url: "+err.Error()))
+		return
+	}
 	// ... existing creation logic ...
```

Note: `validateEndpointURL` is currently defined in `mcp_servers.go`. Extract it to a shared location (e.g., `internal/api/validation.go`) so both handlers can use it.

---

### Diff-Fix B-CRIT-1: Webhook Dispatcher Shutdown Race

**File:** `internal/notify/dispatcher.go`

```diff
+// stopped is set atomically to reject new events during shutdown.
+type Dispatcher struct {
 	// ... existing fields ...
+	stopped atomic.Bool
 }

 func (d *Dispatcher) Dispatch(event Event) {
+	if d.stopped.Load() {
+		return // reject events during shutdown
+	}
 	select {
 	case d.eventCh <- event:
 	default:
 		// channel full, drop event
 	}
 }

 func (d *Dispatcher) Shutdown() {
+	d.stopped.Store(true)
 	close(d.done)
+	close(d.eventCh) // workers will drain via range
 	d.wg.Wait()
 }

 func (d *Dispatcher) worker() {
 	defer d.wg.Done()
-	for {
-		select {
-		case <-d.done:
-			// drain
-			for {
-				select {
-				case ev := <-d.eventCh:
-					d.process(ev)
-				default:
-					return
-				}
-			}
-		case ev := <-d.eventCh:
+	for ev := range d.eventCh {
 			d.process(ev)
-		}
 	}
 }
```

---

## 7. Positive Observations

The audit also confirmed many things are done correctly:

- **No SQL injection** — all queries use parameterized `pgx` placeholders
- **Strong cryptography** — bcrypt cost 12, AES-256-GCM with random nonces, HMAC-SHA256, 32-byte crypto/rand tokens
- **No secret leakage** — `password_hash` uses `json:"-"`, API keys stored as SHA-256 hashes
- **Proper RBAC** — all mutation endpoints enforce `RequireRole` middleware
- **Security headers** — HSTS, X-Frame-Options, CSP, X-Content-Type-Options all set
- **No XSS vectors** — no `dangerouslySetInnerHTML` usage in frontend
- **No `any` types** in production TypeScript (only in test mocks)
- **Consistent PF5 usage** — no PF4/PF5 mixing
- **Optimistic concurrency** — If-Match/updated_at correctly implemented on PUT/PATCH
- **Body size limits** — MaxBodySize middleware caps requests at 1MB
- **OAuth state** — encrypted with AES-256-GCM, not just signed

---

## Appendix: Finding Index

| ID | Severity | Title | Agent(s) |
|----|----------|-------|----------|
| S-CRIT-1 | CRITICAL | MustChangePassMiddleware never wired | Security, Backend, Frontend |
| B-CRIT-1 | CRITICAL | Webhook dispatcher shutdown race | Backend |
| F-CRIT-1 | CRITICAL | AuthContext must_change_pass bypass on refresh | Frontend |
| F-CRIT-2 | CRITICAL | No global ErrorBoundary | Frontend |
| T-CRIT-1 | CRITICAL | Auth test suite broken (51 tests) | QA |
| T-CRIT-2 | CRITICAL | Store layer has zero tests | QA |
| T-CRIT-3 | CRITICAL | DB layer has zero tests | QA |
| S-HIGH-1 | HIGH | CSRF validation bypass via fallback | Security, Backend |
| S-HIGH-2 | HIGH | Webhook URL SSRF | Security, Backend |
| S-HIGH-3 | HIGH | CSRF token base64 truncation | Frontend |
| B-HIGH-1 | HIGH | Swallowed json.Unmarshal error | Backend |
| B-HIGH-2 | HIGH | Audit log errors swallowed | Backend |
| B-HIGH-3 | HIGH | Get handler maps all errors to 404 | Backend |
| B-HIGH-4 | HIGH | No pagination upper bound | Backend, Security |
| F-HIGH-1 | HIGH | ProtectedRoute bypass via refresh | Frontend |
| F-HIGH-2 | HIGH | PromptsPage wrong response fields | Frontend |
| F-HIGH-3 | HIGH | 14+ catch blocks silently swallow errors | Frontend |
| T-HIGH-1 | HIGH | No API contract tests | QA |
| T-HIGH-2 | HIGH | api/client.ts has zero tests | QA |
| T-HIGH-3 | HIGH | 7 frontend components untested | QA |
| T-HIGH-4 | HIGH | No cross-resource integration test | QA |
| T-HIGH-5 | HIGH | Mock stores may hide SQL bugs | QA |
| S-MED-1 | MEDIUM | Agent PATCH type assertion panic | Security, Backend |
| S-MED-2 | MEDIUM | Rate limiter unbounded memory | Security, Backend |
| S-MED-3 | MEDIUM | Encryption key length not validated | Security |
| S-MED-4 | MEDIUM | SESSION_SECRET unused | Security |
| B-MED-1 | MEDIUM | MCP servers List has no pagination | Backend |
| B-MED-2 | MEDIUM | Agent Update overwrites CreatedBy | Backend |
| B-MED-3 | MEDIUM | Password change invalidates all sessions | Backend |
| B-MED-4 | MEDIUM | Dispatcher uses context.Background() | Backend |
| F-MED-1 | MEDIUM | Webhook toggle missing ETag | Frontend |
| F-MED-2 | MEDIUM | Trigger create missing workspace_id | Frontend |
| F-MED-3 | MEDIUM | Password validation duplicated | Frontend |
| F-MED-4 | MEDIUM | example_prompts parsing ambiguous | Frontend |
| F-MED-5 | MEDIUM | Nav active state strict equality | Frontend |
| F-MED-6 | MEDIUM | SignalsPage non-accessible span | Frontend |
| F-MED-7 | MEDIUM | Toast duplicate timeout | Frontend |
| F-MED-8 | MEDIUM | No loading state on mutation buttons | Frontend |
| F-MED-9 | MEDIUM | AuditLog "1-50 of 0" when empty | Frontend |
| T-MED-1 | MEDIUM | Webhook concurrent delivery untested | QA |
| T-MED-2 | MEDIUM | Audit log SQL filtering untested | QA |
| T-MED-3 | MEDIUM | Agent seeder incomplete verification | QA |
| T-MED-4 | MEDIUM | Rate limiter concurrency untested | QA |
| T-MED-5 | MEDIUM | MCP encryption not tested E2E | QA |
| T-MED-6 | MEDIUM | Security escalation test incomplete | QA |
| S-LOW-1 | LOW | X-Forwarded-For IP poisoning | Security |
| S-LOW-2 | LOW | CORS port normalization | Security |
| S-LOW-3 | INFO | Webhook secrets stored plaintext | Security |
| B-LOW-1 | LOW | Duplicated clientIP function | Backend |
| B-LOW-2 | LOW | Duplicated auditLog/dispatchEvent | Backend |
| B-LOW-3 | LOW | Custom string search functions | Backend |
| B-LOW-4 | LOW | Fragile isStoreNotFound | Backend |
| B-LOW-5 | LOW | Security headers middleware duplicated | Backend |
| F-LOW-1 | LOW | JsonEditor hardcoded ID | Frontend |
| F-LOW-2 | LOW | OAuth link no loading indicator | Frontend |
| F-LOW-3 | LOW | api.delete no ETag support | Frontend |
| F-LOW-4 | LOW | DiffViewer array index keys | Frontend |
| T-LOW-1 | LOW | Dispatcher test uses time.Sleep | QA |
| T-LOW-2 | LOW | Frontend act() warnings | QA |
