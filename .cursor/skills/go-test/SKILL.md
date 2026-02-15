---
name: go-test
description: Run Go tests for the Agentic Registry backend with race detection, coverage, and vet. Use when the user says "run tests", "go test", "test the backend", "check coverage", or "vet".
---

# Go Test Runner

## Default: Run All Tests

```bash
go test -race -count=1 ./...
```

## Specific Package

Map user intent to package:

| User says | Command |
|-----------|---------|
| "test store" / "test db" | `go test -race -count=1 -v ./internal/store/...` |
| "test auth" | `go test -race -count=1 -v ./internal/auth/...` |
| "test api" / "test handlers" | `go test -race -count=1 -v ./internal/api/...` |
| "test notify" / "test webhooks" | `go test -race -count=1 -v ./internal/notify/...` |
| "test config" | `go test -race -count=1 -v ./internal/config/...` |

## Specific Test Function

```bash
go test -race -count=1 -v -run TestAgentCreate ./internal/api/...
```

## Coverage

```bash
go test -race -count=1 -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

Report total percentage. Flag packages below 70%.

## Vet / Static Analysis

```bash
go vet ./...
```

## Post-Test Actions

1. Report pass/fail count and duration
2. If failures: read failing test and source to diagnose
3. If all pass: report total test count
4. If tests need a database: remind that `DATABASE_URL` must be set

## Constraints

- Always use `-race` unless explicitly told not to
- Always use `-count=1` to bypass cache during dev
- Never use `-short` unless asked
