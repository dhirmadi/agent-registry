# Skill: web-dev

## Description
Manages frontend development workflows for the Agentic Registry admin GUI: dependency installation, dev server, testing, and production builds.

## Execution Logic

When the user says "web test", "frontend test", "run web dev", or "/web-dev":

### Install Dependencies
```bash
cd web && npm ci
```
- Use `npm ci` for clean installs (respects lockfile)
- Use `npm install` only when adding new packages

### Run Tests
```bash
cd web && npm test -- --run
```
- `--run` prevents watch mode — runs once and exits
- Report pass/fail count and any failing test details

### Run Dev Server
```bash
cd web && npm run dev
```
- Starts Vite HMR dev server (default port 5173)
- Proxies API calls to Go backend at localhost:8090

### Production Build
```bash
cd web && npm run build
```
- Outputs to `web/dist/`
- Verify the build completes without TypeScript or bundling errors
- Report output size

### Type Check
```bash
cd web && npx tsc --noEmit
```
- Run TypeScript compiler without emitting files
- Report any type errors with file:line references

### Full Check
When asked for a "full check" or "verify frontend":
1. Install deps (`npm ci`)
2. Type check (`tsc --noEmit`)
3. Run tests (`npm test -- --run`)
4. Production build (`npm run build`)
5. Report results for each step

## Constraints
- Always run commands from the `web/` directory
- Use PatternFly 5 components — no Tailwind, no custom CSS frameworks
- Tests use Vitest + @testing-library/react
- State management via React Context only (no Redux, Zustand)
- Do not add dependencies not listed in spec Appendix C.2
