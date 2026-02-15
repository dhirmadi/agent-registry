# Skill: build-registry
## Description
Coordinates the end-to-end implementation of the Agentic Registry Server based on the `agentic_registry_spec.md`. It manages the phase-based roadmap, coordinates sub-agents (Architect, Gopher, Frontend), and ensures security hardening.

## Execution Logic
When the user says "build the registry" or "start phase [number]", follow this protocol:

1. **Analysis Mode (Adaptive Thinking: High)**: 
   - Internalize the `agentic_registry_spec.md`.
   - Audit the current directory to ensure it is clean.
   - Initialize the `CLAUDE.md` file for the new repo with the tech stack: Go 1.25, React/PatternFly 5, PostgreSQL 16.

2. **Swarm Initialization**: 
   - Spawn a **Swarm** of 3 agents:
     - `Registry-Architect`: Oversees DB migrations and API consistency.
     - `Gopher-Lead`: Implements the Go backend, Auth, and OTel.
     - `PF5-Specialist`: Builds the React Admin GUI using PatternFly 5 components.

3. **Phase-Gated Implementation**:
   - Only move to Phase 2 after Phase 1 tests pass.
   - **Phase 1 (Foundation)**: Setup Go module, Auth (bcrypt/OAuth), and Health checks.
   - **Phase 2 (Resources)**: Implement Agent/Prompt CRUD and encryption for credentials.
   - **Phase 3 (System)**: Webhooks, Signal config, and the Discovery endpoint.
   - **Phase 4 (GUI)**: Build and embed the React frontend.

4. **Safety & Security Check**:
   - After every slice, run a security scan on `internal/auth` and `internal/store`.
   - Ensure `CREDENTIAL_ENCRYPTION_KEY` handling is never logged.

## Constraints
- **Strictly Go 1.25** using `chi/v5` and `pgx/v5`.
- **No GORM or heavy frameworks**. Use raw SQL migrations.
- **Strictly PatternFly 5** for the UI—no Tailwind or custom CSS unless necessary.
- **Embedded FS**: The UI must be served via Go's `embed` package.