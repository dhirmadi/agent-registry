# Skill: security-audit

## Description
Initializes an Adversarial Agent Team to stress-test the Agentic Registry. Produces a verified "Secure Baseline" after all identified vulnerabilities are fixed and confirmed by tests.

## Execution Logic
When the user says "run security audit", "stress-test security", or "adversarial audit":

### 1. Team Initialization
Spawn a **Team** of 3 agents:

- **Red-Teamer** (`security-auditor` agent): Attempt to bypass `must_change_pass` logic and inject malicious tool definitions. Use Adaptive Thinking (Max) to find non-obvious injection vectors in MCP server configurations, trust rules, agent tools, and auth flows.

- **QA-Engineer** (using `qa-specialist` skill): Provide the Red-Teamer with existing test coverage and immediately write new integration tests to reproduce any found vulnerabilities. If a vulnerability is suspected, spin up a temporary environment and attempt to prove it with a failing test case.

- **Lead Architect**: Coordinate the two, manage the Shared Task List, and resolve disagreements between findings. Ensure fixes don't regress existing functionality.

### 2. Attack Protocol
The Red-Teamer focuses on these vectors (non-exhaustive):

- **Auth bypass**: `must_change_pass` enforcement, session hijacking, CSRF token reuse, API key scope escalation
- **Injection**: MCP endpoint SSRF (internal IPs, AWS metadata), tool_pattern injection (shell, path traversal, regex DoS, null bytes), circuit_breaker JSON injection
- **Validation gaps**: Agent tool source validation, MCP discovery_interval bounds, schema conformance, oversized payloads
- **Privilege escalation**: Role boundary violations (viewer → admin), cross-workspace data access, audit log tampering

### 3. Verification Protocol
For every finding:

1. Red-Teamer documents the attack vector with a proof-of-concept
2. QA-Engineer writes a **failing test** that reproduces the vulnerability
3. Lead Architect reviews and prioritizes the fix
4. Developer (or Lead Architect) implements the fix
5. QA-Engineer confirms the test now **passes**
6. QA-Engineer runs the full suite (`go test -race ./...`) to verify no regressions

### 4. Deliverable: Secure Baseline
The audit is complete only when:

- All identified vulnerabilities have verified fixes
- All new security tests pass
- Full test suite is green and race-clean
- A summary table is produced: `| # | Severity | Finding | Fix Location | Test File |`

## Guidelines
- Severity levels: CRITICAL, HIGH, MEDIUM, LOW
- All fixes must follow existing patterns (no new dependencies)
- New tests use stdlib `testing` package with table-driven sub-tests
- Run with `-race` flag on every verification pass
- Check `go vet ./...` passes after all changes
- Document findings in the team's task list for traceability
