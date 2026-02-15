#!/usr/bin/env bash
# =============================================================================
# Agentic Registry — Full API Verification Script
# =============================================================================
# Tests every API endpoint for correct status codes, response structure,
# CRUD lifecycle, auth enforcement, and confirms removed features are gone.
#
# Usage:
#   ./scripts/api-test.sh                     # defaults to http://localhost:8090
#   ./scripts/api-test.sh https://my-host:443
#
# Prerequisites: curl, jq
# =============================================================================

set -uo pipefail

BASE_URL="${1:-http://localhost:8090}"
COOKIE_JAR=$(mktemp)
CSRF=""
PASS=0
FAIL=0
SKIP=0
ERRORS=()
# Unique suffix for test data to avoid collisions across runs
UNIQ=$(date +%s)

# ── Colours ──────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

cleanup() {
  rm -f "$COOKIE_JAR"
}
trap cleanup EXIT

# ── Helpers ──────────────────────────────────────────────────────────────────

api() {
  local method="$1" path="$2" body="${3:-}"
  local headers=(-H "Content-Type: application/json")

  if [[ -n "$CSRF" ]]; then
    headers+=(-H "X-CSRF-Token: $CSRF")
  fi

  if [[ -n "$body" ]]; then
    curl -s -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
      -X "$method" "${BASE_URL}${path}" \
      "${headers[@]}" -d "$body"
  else
    curl -s -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
      -X "$method" "${BASE_URL}${path}" \
      "${headers[@]}"
  fi
}

api_status() {
  local method="$1" path="$2" body="${3:-}"
  local headers=(-H "Content-Type: application/json")

  if [[ -n "$CSRF" ]]; then
    headers+=(-H "X-CSRF-Token: $CSRF")
  fi

  if [[ -n "$body" ]]; then
    curl -s -o /dev/null -w "%{http_code}" -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
      -X "$method" "${BASE_URL}${path}" \
      "${headers[@]}" -d "$body"
  else
    curl -s -o /dev/null -w "%{http_code}" -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
      -X "$method" "${BASE_URL}${path}" \
      "${headers[@]}"
  fi
}

api_with_etag() {
  local method="$1" path="$2" etag="$3" body="${4:-}"
  curl -s -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
    -X "$method" "${BASE_URL}${path}" \
    -H "Content-Type: application/json" \
    -H "X-CSRF-Token: $CSRF" \
    -H "If-Match: $etag" \
    -d "$body"
}

assert_status() {
  local test_name="$1" expected="$2" actual="$3"
  if [[ "$actual" == "$expected" ]]; then
    echo -e "  ${GREEN}PASS${NC} $test_name (${actual})"
    ((PASS++))
  else
    echo -e "  ${RED}FAIL${NC} $test_name — expected ${expected}, got ${actual}"
    ((FAIL++))
    ERRORS+=("$test_name: expected $expected, got $actual")
  fi
}

assert_json() {
  local test_name="$1" json="$2" jq_expr="$3" expected="$4"
  local actual
  actual=$(echo "$json" | jq -r "$jq_expr" 2>/dev/null || echo "JQ_ERROR")
  if [[ "$actual" == "$expected" ]]; then
    echo -e "  ${GREEN}PASS${NC} $test_name"
    ((PASS++))
  else
    echo -e "  ${RED}FAIL${NC} $test_name — expected '${expected}', got '${actual}'"
    ((FAIL++))
    ERRORS+=("$test_name: expected '$expected', got '$actual'")
  fi
}

assert_json_exists() {
  local test_name="$1" json="$2" jq_expr="$3"
  local actual
  actual=$(echo "$json" | jq -r "$jq_expr" 2>/dev/null || echo "null")
  if [[ "$actual" != "null" && "$actual" != "" ]]; then
    echo -e "  ${GREEN}PASS${NC} $test_name"
    ((PASS++))
  else
    echo -e "  ${RED}FAIL${NC} $test_name — field missing or null"
    ((FAIL++))
    ERRORS+=("$test_name: field missing or null")
  fi
}

section() {
  echo ""
  echo -e "${CYAN}${BOLD}━━━ $1 ━━━${NC}"
}

# =============================================================================
# TEST EXECUTION
# =============================================================================

echo -e "${BOLD}Agentic Registry — API Test Suite${NC}"
echo -e "Target: ${CYAN}${BASE_URL}${NC}"
echo -e "Time:   $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo ""

# ─────────────────────────────────────────────────────────────────────────────
section "1. Health Checks"
# ─────────────────────────────────────────────────────────────────────────────

RESP=$(api GET /healthz)
assert_json "GET /healthz returns success" "$RESP" ".success" "true"
assert_json "GET /healthz status is ok"    "$RESP" ".data.status" "ok"

RESP=$(api GET /readyz)
assert_json "GET /readyz returns success" "$RESP" ".success" "true"

# ─────────────────────────────────────────────────────────────────────────────
section "2. Auth — Unauthenticated Access Blocked"
# ─────────────────────────────────────────────────────────────────────────────

STATUS=$(api_status GET /api/v1/agents)
assert_status "GET /api/v1/agents without auth returns 401" "401" "$STATUS"

STATUS=$(api_status GET /api/v1/discovery)
assert_status "GET /api/v1/discovery without auth returns 401" "401" "$STATUS"

# ─────────────────────────────────────────────────────────────────────────────
section "3. Auth — Login"
# ─────────────────────────────────────────────────────────────────────────────

RESP=$(api POST /auth/login '{"username":"admin","password":"admin"}')
assert_json "POST /auth/login succeeds"         "$RESP" ".success" "true"
assert_json "Login returns admin role"           "$RESP" ".data.user.role" "admin"
assert_json "Login returns correct username"     "$RESP" ".data.user.username" "admin"
assert_json_exists "Login returns user id"       "$RESP" ".data.user.id"

# Extract CSRF token from cookie jar
CSRF=$(grep -i csrf "$COOKIE_JAR" | awk '{print $NF}' || echo "")
if [[ -z "$CSRF" ]]; then
  echo -e "  ${YELLOW}WARN${NC} No CSRF cookie found — mutations may fail"
fi

# ─────────────────────────────────────────────────────────────────────────────
section "4. Auth — Session (GET /auth/me)"
# ─────────────────────────────────────────────────────────────────────────────

RESP=$(api GET /auth/me)
assert_json "GET /auth/me returns success"     "$RESP" ".success" "true"
assert_json "GET /auth/me returns admin user"  "$RESP" ".data.username" "admin"
assert_json "GET /auth/me returns admin role"  "$RESP" ".data.role" "admin"

# ─────────────────────────────────────────────────────────────────────────────
section "5. Agents CRUD"
# ─────────────────────────────────────────────────────────────────────────────

# List agents
RESP=$(api GET /api/v1/agents)
assert_json "GET /agents returns success" "$RESP" ".success" "true"
assert_json_exists "GET /agents has agents array" "$RESP" ".data.agents"

# Create agent
AGENT_TEST_ID="test_api_agent_${UNIQ}"
RESP=$(api POST /api/v1/agents "{
  \"id\": \"${AGENT_TEST_ID}\",
  \"name\": \"API Test Agent ${UNIQ}\",
  \"description\": \"Created by API test script\",
  \"is_active\": true
}")
assert_json "POST /agents creates agent"         "$RESP" ".success" "true"
assert_json "Created agent has correct id"        "$RESP" ".data.id" "$AGENT_TEST_ID"
assert_json "Created agent has correct name"      "$RESP" ".data.name" "API Test Agent ${UNIQ}"
AGENT_UPDATED_AT=$(echo "$RESP" | jq -r '.data.updated_at')

# Get single agent
RESP=$(api GET "/api/v1/agents/$AGENT_TEST_ID")
assert_json "GET /agents/{id} returns success"    "$RESP" ".success" "true"
assert_json "GET /agents/{id} correct id"         "$RESP" ".data.id" "$AGENT_TEST_ID"

# Update agent (PUT with If-Match)
RESP=$(api_with_etag PUT "/api/v1/agents/$AGENT_TEST_ID" "$AGENT_UPDATED_AT" "{
  \"id\": \"${AGENT_TEST_ID}\",
  \"name\": \"API Test Agent Updated\",
  \"description\": \"Updated by API test\",
  \"is_active\": true
}")
assert_json "PUT /agents/{id} updates agent"      "$RESP" ".success" "true"
assert_json "Updated agent name"                  "$RESP" ".data.name" "API Test Agent Updated"

# Patch agent
AGENT_UPDATED_AT=$(echo "$RESP" | jq -r '.data.updated_at')
RESP=$(api_with_etag PATCH "/api/v1/agents/$AGENT_TEST_ID" "$AGENT_UPDATED_AT" '{
  "description": "Patched description"
}')
assert_json "PATCH /agents/{id} patches agent"    "$RESP" ".success" "true"
assert_json "Patched description"                 "$RESP" ".data.description" "Patched description"

# List versions
RESP=$(api GET "/api/v1/agents/$AGENT_TEST_ID/versions")
assert_json "GET /agents/{id}/versions success"   "$RESP" ".success" "true"

# Delete agent
STATUS=$(api_status DELETE "/api/v1/agents/$AGENT_TEST_ID")
assert_status "DELETE /agents/{id} returns 204" "204" "$STATUS"

# Confirm soft-deleted (is_active=false)
RESP=$(api GET "/api/v1/agents/$AGENT_TEST_ID")
assert_json "GET soft-deleted agent succeeds"     "$RESP" ".success" "true"
assert_json "Soft-deleted agent is inactive"      "$RESP" ".data.is_active" "false"

# ─────────────────────────────────────────────────────────────────────────────
section "6. Prompts CRUD"
# ─────────────────────────────────────────────────────────────────────────────

# Create a temp agent for prompts
PROMPT_AGENT_ID="prompt_test_${UNIQ}"
api POST /api/v1/agents "{\"id\":\"${PROMPT_AGENT_ID}\",\"name\":\"Prompt Agent ${UNIQ}\",\"is_active\":true}" > /dev/null

# Create prompt
RESP=$(api POST "/api/v1/agents/$PROMPT_AGENT_ID/prompts" '{
  "system_prompt": "You are a helpful test assistant.",
  "mode": "toolcalling_safe"
}')
assert_json "POST /prompts creates prompt"        "$RESP" ".success" "true"
PROMPT_ID=$(echo "$RESP" | jq -r '.data.id')
assert_json_exists "Created prompt has id"        "$RESP" ".data.id"

# List prompts
RESP=$(api GET "/api/v1/agents/$PROMPT_AGENT_ID/prompts")
assert_json "GET /prompts lists prompts"          "$RESP" ".success" "true"

# Get prompt by ID
RESP=$(api GET "/api/v1/agents/$PROMPT_AGENT_ID/prompts/$PROMPT_ID")
assert_json "GET /prompts/{id} returns prompt"    "$RESP" ".success" "true"

# Activate prompt
RESP=$(api POST "/api/v1/agents/$PROMPT_AGENT_ID/prompts/$PROMPT_ID/activate")
assert_json "POST /prompts/{id}/activate works"   "$RESP" ".success" "true"

# Get active prompt
RESP=$(api GET "/api/v1/agents/$PROMPT_AGENT_ID/prompts/active")
assert_json "GET /prompts/active returns prompt"  "$RESP" ".success" "true"
assert_json "Active prompt matches"               "$RESP" ".data.id" "$PROMPT_ID"

# Clean up
api DELETE "/api/v1/agents/$PROMPT_AGENT_ID" > /dev/null 2>&1 || true

# ─────────────────────────────────────────────────────────────────────────────
section "7. MCP Servers CRUD"
# ─────────────────────────────────────────────────────────────────────────────

# Create
RESP=$(api POST /api/v1/mcp-servers '{
  "label": "test-mcp-server",
  "endpoint": "https://mcp.test.example.com/api",
  "auth_type": "none",
  "is_enabled": true
}')
assert_json "POST /mcp-servers creates server"    "$RESP" ".success" "true"
MCP_ID=$(echo "$RESP" | jq -r '.data.id')
MCP_ETAG=$(echo "$RESP" | jq -r '.data.updated_at')
assert_json_exists "Created MCP server has id"    "$RESP" ".data.id"

# List
RESP=$(api GET /api/v1/mcp-servers)
assert_json "GET /mcp-servers lists servers"      "$RESP" ".success" "true"

# Get by ID
RESP=$(api GET "/api/v1/mcp-servers/$MCP_ID")
assert_json "GET /mcp-servers/{id} returns server" "$RESP" ".success" "true"
assert_json "MCP server label correct"            "$RESP" ".data.label" "test-mcp-server"

# Update
RESP=$(api_with_etag PUT "/api/v1/mcp-servers/$MCP_ID" "$MCP_ETAG" '{
  "label": "test-mcp-server",
  "endpoint": "https://mcp.test.example.com/v2/api",
  "auth_type": "bearer",
  "credential": "my-secret-token",
  "is_enabled": true
}')
assert_json "PUT /mcp-servers/{id} updates server" "$RESP" ".success" "true"

# Delete
STATUS=$(api_status DELETE "/api/v1/mcp-servers/$MCP_ID")
assert_status "DELETE /mcp-servers/{id} returns 204" "204" "$STATUS"

# ─────────────────────────────────────────────────────────────────────────────
section "8. Trust Defaults"
# ─────────────────────────────────────────────────────────────────────────────

RESP=$(api GET /api/v1/trust-defaults)
assert_json "GET /trust-defaults lists defaults"  "$RESP" ".success" "true"

# If there are defaults, try updating one
FIRST_ID=$(echo "$RESP" | jq -r '.data.defaults[0].id // empty')
FIRST_ETAG=$(echo "$RESP" | jq -r '.data.defaults[0].updated_at // empty')
if [[ -n "$FIRST_ID" && -n "$FIRST_ETAG" ]]; then
  RESP=$(api_with_etag PUT "/api/v1/trust-defaults/$FIRST_ID" "$FIRST_ETAG" '{
    "tier": "auto",
    "patterns": ["*"],
    "priority": 1
  }')
  assert_json "PUT /trust-defaults/{id} updates"  "$RESP" ".success" "true"
else
  echo -e "  ${YELLOW}SKIP${NC} No trust defaults to update"
  ((SKIP++))
fi

# ─────────────────────────────────────────────────────────────────────────────
section "9. Trust Rules (workspace-scoped)"
# ─────────────────────────────────────────────────────────────────────────────

WS_ID="00000000-0000-0000-0000-000000000001"

# Create
RESP=$(api POST "/api/v1/workspaces/$WS_ID/trust-rules" '{
  "tool_pattern": "git_*",
  "tier": "auto"
}')
assert_json "POST /trust-rules creates rule"      "$RESP" ".success" "true"
RULE_ID=$(echo "$RESP" | jq -r '.data.id')

# List
RESP=$(api GET "/api/v1/workspaces/$WS_ID/trust-rules")
assert_json "GET /trust-rules lists rules"        "$RESP" ".success" "true"

# Delete
STATUS=$(api_status DELETE "/api/v1/workspaces/$WS_ID/trust-rules/$RULE_ID")
assert_status "DELETE /trust-rules/{id} returns 204" "204" "$STATUS"

# ─────────────────────────────────────────────────────────────────────────────
section "10. Model Config"
# ─────────────────────────────────────────────────────────────────────────────

# Get global
RESP=$(api GET /api/v1/model-config)
assert_json "GET /model-config returns config"    "$RESP" ".success" "true"
MC_ETAG=$(echo "$RESP" | jq -r '.data.updated_at // empty')

if [[ -n "$MC_ETAG" ]]; then
  RESP=$(api_with_etag PUT /api/v1/model-config "$MC_ETAG" '{
    "default_model": "gpt-4o",
    "temperature": 0.7,
    "max_tokens": 4096
  }')
  assert_json "PUT /model-config updates config"  "$RESP" ".success" "true"
else
  # Upsert (first time)
  RESP=$(api_with_etag PUT /api/v1/model-config "1970-01-01T00:00:00Z" '{
    "default_model": "gpt-4o",
    "temperature": 0.7,
    "max_tokens": 4096
  }')
  assert_json "PUT /model-config upserts config"  "$RESP" ".success" "true"
fi

# ─────────────────────────────────────────────────────────────────────────────
section "11. Webhooks CRUD"
# ─────────────────────────────────────────────────────────────────────────────

# Create
RESP=$(api POST /api/v1/webhooks '{
  "url": "https://hooks.test.example.com/callback",
  "events": ["agent.created", "agent.updated"],
  "secret": "webhook-test-secret"
}')
assert_json "POST /webhooks creates webhook"      "$RESP" ".success" "true"
WH_ID=$(echo "$RESP" | jq -r '.data.id')

# List
RESP=$(api GET /api/v1/webhooks)
assert_json "GET /webhooks lists webhooks"        "$RESP" ".success" "true"

# Delete
STATUS=$(api_status DELETE "/api/v1/webhooks/$WH_ID")
assert_status "DELETE /webhooks/{id} returns 204" "204" "$STATUS"

# ─────────────────────────────────────────────────────────────────────────────
section "12. API Keys CRUD"
# ─────────────────────────────────────────────────────────────────────────────

# Create
RESP=$(api POST /api/v1/api-keys '{
  "name": "API Test Key",
  "scopes": ["read"]
}')
assert_json "POST /api-keys creates key"          "$RESP" ".success" "true"
KEY_ID=$(echo "$RESP" | jq -r '.data.id')
KEY_PLAINTEXT=$(echo "$RESP" | jq -r '.data.key')
assert_json_exists "Created API key has plaintext" "$RESP" ".data.key"

# List
RESP=$(api GET /api/v1/api-keys)
assert_json "GET /api-keys lists keys"            "$RESP" ".success" "true"

# Test API key auth
RESP=$(curl -s "${BASE_URL}/api/v1/agents" -H "Authorization: Bearer $KEY_PLAINTEXT")
assert_json "Bearer token auth works"             "$RESP" ".success" "true"

# Revoke
STATUS=$(api_status DELETE "/api/v1/api-keys/$KEY_ID")
assert_status "DELETE /api-keys/{id} returns 204" "204" "$STATUS"

# Revoked key should fail
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/api/v1/agents" -H "Authorization: Bearer $KEY_PLAINTEXT")
assert_status "Revoked API key returns 401" "401" "$STATUS"

# ─────────────────────────────────────────────────────────────────────────────
section "13. Users CRUD (admin only)"
# ─────────────────────────────────────────────────────────────────────────────

# Create user
TEST_USERNAME="testuser_${UNIQ}"
TEST_EMAIL="testuser_${UNIQ}@example.com"
RESP=$(api POST /api/v1/users "{
  \"username\": \"${TEST_USERNAME}\",
  \"email\": \"${TEST_EMAIL}\",
  \"password\": \"SecurePass123!\",
  \"role\": \"viewer\"
}")
assert_json "POST /users creates user"            "$RESP" ".success" "true"
USER_ID=$(echo "$RESP" | jq -r '.data.id')
assert_json "Created user has viewer role"        "$RESP" ".data.role" "viewer"

# List users
RESP=$(api GET /api/v1/users)
assert_json "GET /users lists users"              "$RESP" ".success" "true"

# Get user
RESP=$(api GET "/api/v1/users/$USER_ID")
assert_json "GET /users/{id} returns user"        "$RESP" ".success" "true"
assert_json "User username matches"               "$RESP" ".data.username" "$TEST_USERNAME"
USER_ETAG=$(echo "$RESP" | jq -r '.data.updated_at')

# Update user
RESP=$(api_with_etag PUT "/api/v1/users/$USER_ID" "$USER_ETAG" "{
  \"username\": \"${TEST_USERNAME}\",
  \"email\": \"${TEST_EMAIL}\",
  \"display_name\": \"Updated Test User\",
  \"role\": \"editor\",
  \"is_active\": true
}")
assert_json "PUT /users/{id} updates user"        "$RESP" ".success" "true"
assert_json "Updated user role"                   "$RESP" ".data.role" "editor"

# Delete user
STATUS=$(api_status DELETE "/api/v1/users/$USER_ID")
assert_status "DELETE /users/{id} returns 204" "204" "$STATUS"

# ─────────────────────────────────────────────────────────────────────────────
section "14. Audit Log"
# ─────────────────────────────────────────────────────────────────────────────

RESP=$(api GET /api/v1/audit-log)
assert_json "GET /audit-log returns success"      "$RESP" ".success" "true"
assert_json_exists "Audit log has items"          "$RESP" ".data.items"
AUDIT_COUNT=$(echo "$RESP" | jq '.data.items | length')
echo -e "  ${CYAN}INFO${NC} Audit log contains ${AUDIT_COUNT} entries"

# ─────────────────────────────────────────────────────────────────────────────
section "15. Discovery Endpoint"
# ─────────────────────────────────────────────────────────────────────────────

RESP=$(api GET /api/v1/discovery)
assert_json "GET /discovery returns success"      "$RESP" ".success" "true"
assert_json_exists "Discovery has agents"         "$RESP" ".data.agents"
assert_json_exists "Discovery has mcp_servers"    "$RESP" ".data.mcp_servers"
assert_json_exists "Discovery has trust_defaults" "$RESP" ".data.trust_defaults"
assert_json_exists "Discovery has model_config"   "$RESP" ".data.model_config"
assert_json_exists "Discovery has fetched_at"     "$RESP" ".data.fetched_at"

# Verify removed fields are NOT present
CTX_CFG=$(echo "$RESP" | jq -r '.data.context_config // "ABSENT"')
SIG_CFG=$(echo "$RESP" | jq -r '.data.signal_config // "ABSENT"')

if [[ "$CTX_CFG" == "ABSENT" ]]; then
  echo -e "  ${GREEN}PASS${NC} Discovery does NOT contain context_config (removed)"
  ((PASS++))
else
  echo -e "  ${RED}FAIL${NC} Discovery still contains context_config (should be removed)"
  ((FAIL++))
  ERRORS+=("Discovery still contains context_config")
fi

if [[ "$SIG_CFG" == "ABSENT" ]]; then
  echo -e "  ${GREEN}PASS${NC} Discovery does NOT contain signal_config (removed)"
  ((PASS++))
else
  echo -e "  ${RED}FAIL${NC} Discovery still contains signal_config (should be removed)"
  ((FAIL++))
  ERRORS+=("Discovery still contains signal_config")
fi

# ─────────────────────────────────────────────────────────────────────────────
section "16. Removed Features — Confirm Gone"
# ─────────────────────────────────────────────────────────────────────────────

STATUS=$(api_status GET /api/v1/signal-config)
assert_status "GET /signal-config returns 404"    "404" "$STATUS"

STATUS=$(api_status GET /api/v1/context-config)
assert_status "GET /context-config returns 404"   "404" "$STATUS"

STATUS=$(api_status GET "/api/v1/workspaces/$WS_ID/trigger-rules")
assert_status "GET /trigger-rules returns 404"    "404" "$STATUS"

STATUS=$(api_status POST "/api/v1/workspaces/$WS_ID/trigger-rules" '{"name":"test","event_type":"push","agent_id":"x"}')
assert_status "POST /trigger-rules returns 404"   "404" "$STATUS"

STATUS=$(api_status PUT "/api/v1/signal-config/00000000-0000-0000-0000-000000000000" '{"poll_interval":"30s"}')
assert_status "PUT /signal-config returns 404"    "404" "$STATUS"

STATUS=$(api_status PUT /api/v1/context-config '{"max_total_tokens":4096}')
assert_status "PUT /context-config returns 404"   "404" "$STATUS"

# ─────────────────────────────────────────────────────────────────────────────
section "17. Role-Based Access Control"
# ─────────────────────────────────────────────────────────────────────────────

# Create a viewer user, login as them, verify restrictions
RBAC_USER="rbac_${UNIQ}"
RBAC_EMAIL="rbac_${UNIQ}@example.com"
api POST /api/v1/users "{
  \"username\": \"${RBAC_USER}\",
  \"email\": \"${RBAC_EMAIL}\",
  \"password\": \"ViewerPass123!\",
  \"role\": \"viewer\"
}" > /dev/null

# Save admin cookies, login as viewer
cp "$COOKIE_JAR" "${COOKIE_JAR}.admin"
ADMIN_CSRF="$CSRF"
rm -f "$COOKIE_JAR"

RESP=$(api POST /auth/login "{\"username\":\"${RBAC_USER}\",\"password\":\"ViewerPass123!\"}")
assert_json "Viewer login succeeds"               "$RESP" ".success" "true"
CSRF=$(grep -i csrf "$COOKIE_JAR" | awk '{print $NF}' || echo "")

# Viewer can read agents
RESP=$(api GET /api/v1/agents)
assert_json "Viewer can read agents"              "$RESP" ".success" "true"

# Viewer cannot create agents
STATUS=$(api_status POST /api/v1/agents '{"id":"viewer_agent","name":"Nope"}')
assert_status "Viewer cannot create agents (403)" "403" "$STATUS"

# Viewer cannot access admin routes
STATUS=$(api_status GET /api/v1/users)
assert_status "Viewer cannot list users (403)"    "403" "$STATUS"

STATUS=$(api_status GET /api/v1/mcp-servers)
assert_status "Viewer cannot list MCP servers (403)" "403" "$STATUS"

# Restore admin session
cp "${COOKIE_JAR}.admin" "$COOKIE_JAR"
CSRF="$ADMIN_CSRF"
rm -f "${COOKIE_JAR}.admin"

# Clean up viewer user
VIEWER_ID=$(api GET /api/v1/users | jq -r ".data.users[] | select(.username==\"${RBAC_USER}\") | .id")
if [[ -n "$VIEWER_ID" ]]; then
  api_status DELETE "/api/v1/users/$VIEWER_ID" > /dev/null 2>&1 || true
fi

# ─────────────────────────────────────────────────────────────────────────────
section "18. Auth — Logout"
# ─────────────────────────────────────────────────────────────────────────────

STATUS=$(api_status POST /auth/logout)
assert_status "POST /auth/logout returns 200" "200" "$STATUS"

# Verify session is dead
STATUS=$(api_status GET /auth/me)
assert_status "GET /auth/me after logout returns 401" "401" "$STATUS"

# =============================================================================
# SUMMARY
# =============================================================================

echo ""
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}RESULTS${NC}"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "  ${GREEN}Passed:${NC}  $PASS"
echo -e "  ${RED}Failed:${NC}  $FAIL"
echo -e "  ${YELLOW}Skipped:${NC} $SKIP"
TOTAL=$((PASS + FAIL + SKIP))
echo -e "  Total:   $TOTAL"
echo ""

if [[ $FAIL -gt 0 ]]; then
  echo -e "${RED}${BOLD}FAILURES:${NC}"
  for err in "${ERRORS[@]}"; do
    echo -e "  ${RED}•${NC} $err"
  done
  echo ""
  exit 1
else
  echo -e "${GREEN}${BOLD}All tests passed!${NC}"
  exit 0
fi
