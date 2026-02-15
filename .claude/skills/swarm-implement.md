# Skill: swarm-implement

## Description
Takes a plan document as input and implements it using a coordinated swarm of specialized agents. Handles task decomposition, worker spawning, parallel implementation, and cleanup.

## Execution Logic
When the user says "/swarm-implement <path-to-plan>" or "swarm implement <plan>":

### 1. Plan Ingestion
- Read the plan file provided as argument.
- Extract: acceptance criteria, data model changes, API endpoints, UI requirements, dependencies.
- Classify work domains: backend (Go), frontend (React), migrations (SQL), tests, security.

### 2. Task Decomposition
Create a task list from the plan. Each task must have:
- Clear scope (one file or one logical unit)
- Domain label (backend | frontend | migration | test | security)
- Dependencies (which tasks block which)
- Acceptance criteria derived from the plan

### 3. Team Initialization
Use `TeamCreate` with name derived from the plan (e.g., `impl-009-model-endpoints`).

Spawn workers based on detected domains (skip unneeded roles):

| Role | Agent Definition | When to Spawn |
|------|-----------------|---------------|
| **Leader** | (self) | Always — coordinates, reviews, merges |
| **Backend** | `go-backend-engineer` | Plan has Go handlers, store, middleware, or migrations |
| **Frontend** | `frontend-engineer` | Plan has UI pages, components, or routes |
| **Tester** | `general-purpose` | Plan has >5 acceptance criteria or needs stress testing |
| **Security** | `security-auditor` | Plan touches auth, encryption, or external URLs |

Minimum 2 workers, maximum 4 (plus leader). Spawn via `Task` tool with `team_name` and `name` params.

### 4. Work Allocation
Leader creates tasks via `TaskCreate` and assigns via `TaskUpdate`:
- Assign by domain match (backend tasks → Backend worker, etc.)
- Respect dependency order (migrations before handlers, handlers before UI)
- Balance load — no worker gets >60% of tasks

### 5. Parallel Execution
Workers implement assigned tasks following project conventions:
- **TDD**: Write failing test → implement → verify green
- **Conventional Commits**: Each logical unit gets its own commit
- **Spec compliance**: All work must match `CLAUDE.md` rules

Leader monitors via `TaskList` and `SendMessage`:
- Unblock workers who hit issues
- Reassign tasks if a worker is stuck
- After each major milestone, output status:
  ```json
  {"team_status": [...], "progress": "X%", "next_action": "..."}
  ```

### 6. Verification Gate
After all tasks are marked complete:
1. `go test -race ./...` — all tests pass
2. `go vet ./...` — clean
3. `cd web && npm test -- --run` — frontend tests pass (if frontend work)
4. `cd web && npm run build` — builds clean (if frontend work)
5. Review all new files for spec compliance

If verification fails → create fix tasks, assign, re-verify.

### 7. Cleanup
- Shut down all workers via `SendMessage` with `type: "shutdown_request"`
- Delete team via `TeamDelete`
- Output final summary: tasks completed, tests added, files changed

## Constraints
- Follow all rules in `CLAUDE.md` (TDD, Conventional Commits, dependency discipline, spec is law).
- Never skip verification gate — all tests must pass before declaring done.
- Workers must use existing agent definitions from `.claude/agents/` — no ad-hoc role invention.
- Minimum tokens: workers keep responses under 500 tokens unless implementation requires more.
- All mutations must produce audit log entries.
- No new dependencies beyond what the spec allows.
