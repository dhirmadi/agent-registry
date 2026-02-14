# Skill: qa-specialist

## Description
Acts as an autonomous Quality Engineer. Responsible for generating, executing, and healing the test suite for the Agentic Registry.

## Execution Logic
When the user says "run QA" or "test this feature":

1. **Scope Discovery**: Scan `internal/api` and `web/src` for new or changed endpoints/components.
2. **Test Generation**:
   - Backend: Create table-driven tests in Go using stdlib `testing` package with `t.Run()` sub-tests.
   - Frontend: Generate Vitest + @testing-library/react tests for user flows (Login, CRUD, role-based visibility).
3. **Stress Testing**: Generate 50+ unique JSON payloads for API endpoints to check edge cases in validation (boundary values, malformed input, injection attempts).
4. **Security Regression**: Verify previously-fixed vulnerabilities (SSRF, tool validation, pattern injection) remain patched.
5. **Self-Correction**: If tests fail, analyze the error logs and provide a `diff-fix` to either the code (if it's a bug) or the test (if it's a change in spec).

## Guidelines
- Follow Go 1.24 standards (use `t.Parallel()` where safe).
- Use stdlib `testing` package — no testify or external test frameworks.
- Frontend tests use Vitest + @testing-library/react — no Playwright.
- Ensure every mutation test checks the **Audit Log** for correctly recorded metadata.
- Run with `-race` flag to catch data races.
- Verify optimistic concurrency (If-Match / updated_at) for all update endpoints.