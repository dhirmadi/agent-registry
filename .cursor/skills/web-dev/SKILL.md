---
name: web-dev
description: Frontend development workflows for the Agentic Registry admin GUI — install deps, run tests, dev server, build, type check. Use when the user says "web test", "frontend test", "run dev server", "build frontend", "type check", or "full check".
---

# Frontend Dev Workflows

All commands run from the `web/` directory.

## Install Dependencies

```bash
cd web && npm ci
```

Use `npm ci` for clean installs. Use `npm install` only when adding new packages.

## Run Tests

```bash
cd web && npm test -- --run
```

`--run` prevents watch mode. Report pass/fail count.

## Dev Server

```bash
cd web && npm run dev
```

Starts Vite HMR on port 5173. Proxies `/api`, `/auth`, `/healthz` to Go backend at `localhost:8090`.

## Production Build

```bash
cd web && npm run build
```

Output: `web/dist/`. Verify no TypeScript or bundling errors.

## Type Check

```bash
cd web && npx tsc --noEmit
```

## Full Check

Run in sequence, stop on first failure:

1. `npm ci`
2. `npx tsc --noEmit`
3. `npm test -- --run`
4. `npm run build`

Report results for each step.

## Constraints

- PatternFly 5 only — no Tailwind, no custom CSS frameworks
- Vitest + @testing-library/react for tests
- React Context for state — no Redux, no Zustand
- No deps beyond spec Appendix C.2
