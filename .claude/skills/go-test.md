# Skill: go-test

## Description
Runs Go tests for the Agentic Registry backend with the appropriate flags and patterns. Provides shortcuts for common testing workflows.

## Execution Logic

When the user says "run tests", "test this", "go test", or "/go-test":

### Default: Run All Tests
```bash
go test -race -count=1 ./...
```
- Always use `-race` to catch data races
- Always use `-count=1` to bypass test caching during development

### Specific Package
When the user names a package or module (e.g., "test the store", "test auth"):
```bash
go test -race -count=1 -v ./internal/store/...
go test -race -count=1 -v ./internal/auth/...
go test -race -count=1 -v ./internal/api/...
```

### Specific Test
When the user names a test function (e.g., "run TestAgentCreate"):
```bash
go test -race -count=1 -v -run TestAgentCreate ./internal/api/...
```

### Coverage Report
When the user asks for coverage:
```bash
go test -race -count=1 -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```
- Report total coverage percentage
- Highlight any packages below 70% coverage

### Vet and Static Analysis
When the user asks to "check" or "lint":
```bash
go vet ./...
```

### Post-Test Actions
After running tests:
1. Report pass/fail count
2. If failures exist, read the failing test and source to diagnose
3. If all pass, report total test count and duration

## Constraints
- Never skip `-race` flag unless the user explicitly asks
- Never use `-short` flag unless explicitly asked — run full tests
- Report failures clearly with file:line references
- If tests require a database, inform the user that `DATABASE_URL` must be set
