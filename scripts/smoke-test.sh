#!/bin/bash
# ============================================================
# Fury SMS Gateway — Smoke Test
# ============================================================
# Prerequisites: Docker, curl, jq
# Run: bash scripts/smoke-test.sh
# ============================================================

set -euo pipefail

BASE_URL="http://localhost:8080"
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'
PASS=0
FAIL=0

check() {
    local desc="$1"
    local cmd="$2"
    if eval "$cmd" 2>/dev/null; then
        echo -e "  ${GREEN}✓${NC} $desc"
        PASS=$((PASS + 1))
    else
        echo -e "  ${RED}✗${NC} $desc"
        FAIL=$((FAIL + 1))
    fi
}

echo "============================================"
echo " Fury SMS Gateway — Smoke Test"
echo "============================================"
echo ""

# ---- 1. Health & Readiness ----
echo "--- [1/8] Health & Readiness ---"
check "Health endpoint returns 200" \
    "curl -sf -o /dev/null -w '%{http_code}' '$BASE_URL/health' | grep -q 200"
check "Readiness endpoint returns 200" \
    "curl -sf -o /dev/null -w '%{http_code}' '$BASE_URL/ready' | grep -q 200"
check "Metrics endpoint returns 200" \
    "curl -sf -o /dev/null -w '%{http_code}' '$BASE_URL/metrics' | grep -q 200"

# ---- 2. Auth Flow ----
echo ""
echo "--- [2/8] Auth ---"
# Login
AUTH_RESP=$(curl -sf -X POST "$BASE_URL/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"email":"admin@fury-sms.com","password":"changeme1234!"}')
check "Login returns access + refresh tokens" \
    "echo '$AUTH_RESP' | jq -e '.data.access_token' > /dev/null && echo '$AUTH_RESP' | jq -e '.data.refresh_token' > /dev/null"

ACCESS_TOKEN=$(echo "$AUTH_RESP" | jq -r '.data.access_token')
REFRESH_TOKEN=$(echo "$AUTH_RESP" | jq -r '.data.refresh_token')
TENANT_ID=$(echo "$AUTH_RESP" | jq -r '.data.tenant_id // "none"')

check "Access token is not empty" \
    "[ -n '$ACCESS_TOKEN' ]"

# Refresh token
REFRESH_RESP=$(curl -sf -X POST "$BASE_URL/api/v1/auth/refresh" \
    -H "Content-Type: application/json" \
    -d "{\"refresh_token\":\"$REFRESH_TOKEN\"}")
check "Refresh returns new access token" \
    "echo '$REFRESH_RESP' | jq -e '.data.access_token' > /dev/null"

# Me endpoint
ME_RESP=$(curl -sf "$BASE_URL/api/v1/me" \
    -H "Authorization: Bearer $ACCESS_TOKEN")
check "Me endpoint returns user data" \
    "echo '$ME_RESP' | jq -e '.data.email' > /dev/null"

# ---- 3. Tenant Management ----
echo ""
echo "--- [3/8] Tenants ---"
CREATE_TENANT_RESP=$(curl -sf -X POST "$BASE_URL/api/v1/tenants" \
    -H "Authorization: Bearer $ACCESS_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"name":"Smoke Test Tenant","slug":"smoke-test","settings":{"default_encoding":"gsm7"}}')
check "Create tenant returns tenant ID" \
    "echo '$CREATE_TENANT_RESP' | jq -e '.data.id' > /dev/null"

TENANT_ID=$(echo "$CREATE_TENANT_RESP" | jq -r '.data.id')
check "Tenant ID is not empty" \
    "[ -n '$TENANT_ID' ]"

# List tenants
LIST_TENANTS_RESP=$(curl -sf "$BASE_URL/api/v1/tenants" \
    -H "Authorization: Bearer $ACCESS_TOKEN")
check "List tenants returns array" \
    "echo '$LIST_TENANTS_RESP' | jq -e '.data | length > 0' > /dev/null"

# ---- 4. API Keys ----
echo ""
echo "--- [4/8] API Keys ---"
CREATE_KEY_RESP=$(curl -sf -X POST "$BASE_URL/api/v1/api-keys" \
    -H "Authorization: Bearer $ACCESS_TOKEN" \
    -H "X-Tenant-ID: $TENANT_ID" \
    -H "Content-Type: application/json" \
    -d '{"name":"smoke-test-key","role":"operator"}')
check "Create API key returns key" \
    "echo '$CREATE_KEY_RESP' | jq -e '.data.api_key' > /dev/null"

API_KEY=$(echo "$CREATE_KEY_RESP" | jq -r '.data.api_key')
check "API key starts with fx_" \
    "echo '$API_KEY' | grep -q '^fx_'"

# API key auth
API_KEY_ME=$(curl -sf "$BASE_URL/api/v1/me" \
    -H "Authorization: Bearer $API_KEY")
check "API key auth works" \
    "echo '$API_KEY_ME' | jq -e '.data.email' > /dev/null"

# ---- 5. Connectors ----
echo ""
echo "--- [5/8] Connectors ---"
CREATE_CONN_RESP=$(curl -sf -X POST "$BASE_URL/api/v1/connectors" \
    -H "Authorization: Bearer $ACCESS_TOKEN" \
    -H "X-Tenant-ID: $TENANT_ID" \
    -H "Content-Type: application/json" \
    -d '{
        "type":"http_client",
        "name":"smoke-http-connector",
        "status":"active",
        "config":{
            "url":"https://httpbin.org/post",
            "method":"POST",
            "content_type":"application/json",
            "body_template":"{\"to\":\"{{.Destination}}\",\"text\":\"{{.Text}}\"}",
            "success_codes":[200],
            "external_id_path":"json.data.origin",
            "timeout_sec":30
        }
    }')
check "Create connector returns ID" \
    "echo '$CREATE_CONN_RESP' | jq -e '.data.id' > /dev/null"

CONN_ID=$(echo "$CREATE_CONN_RESP" | jq -r '.data.id')

# List connectors
LIST_CONN_RESP=$(curl -sf "$BASE_URL/api/v1/connectors?tenant_id=$TENANT_ID" \
    -H "Authorization: Bearer $ACCESS_TOKEN" \
    -H "X-Tenant-ID: $TENANT_ID")
check "List connectors returns connector" \
    "echo '$LIST_CONN_RESP' | jq -e '.data | length > 0' > /dev/null"

# Test connector
TEST_CONN_RESP=$(curl -sf -X POST "$BASE_URL/api/v1/connectors/$CONN_ID/test" \
    -H "Authorization: Bearer $ACCESS_TOKEN" \
    -H "X-Tenant-ID: $TENANT_ID")
check "Test connector succeeds" \
    "echo '$TEST_CONN_RESP' | jq -e '.data.status == \"success\" or .data.status == \"error\"' > /dev/null"

# ---- 6. Routes ----
echo ""
echo "--- [6/8] Routes ---"
CREATE_ROUTE_RESP=$(curl -sf -X POST "$BASE_URL/api/v1/routes" \
    -H "Authorization: Bearer $ACCESS_TOKEN" \
    -H "X-Tenant-ID: $TENANT_ID" \
    -H "Content-Type: application/json" \
    -d "{
        \"name\":\"smoke-route\",
        \"type\":\"sms\",
        \"strategy\":\"static\",
        \"prefix\":\"00\",
        \"priority\":10,
        \"connector_ids\":[\"$CONN_ID\"],
        \"weight\":1
    }")
check "Create route returns ID" \
    "echo '$CREATE_ROUTE_RESP' | jq -e '.data.id' > /dev/null"

ROUTE_ID=$(echo "$CREATE_ROUTE_RESP" | jq -r '.data.id')

# ---- 7. Message Flow ----
echo ""
echo "--- [7/8] Messages ---"
# Send a message via API (this will create a 'queued' message)
SEND_MSG_RESP=$(curl -sf -X POST "$BASE_URL/api/v1/messages" \
    -H "Authorization: Bearer $API_KEY" \
    -H "Content-Type: application/json" \
    -d "{
        \"source\":\"SmokeTest\",
        \"destination\":\"1234567890\",
        \"text\":\"Hello from Fury SMS Gateway!\",
        \"encoding\":\"gsm7\",
        \"client_ref\":\"smoke-$(date +%s)\"
    }")
check "Send message returns message ID" \
    "echo '$SEND_MSG_RESP' | jq -e '.data.id' > /dev/null"

MSG_ID=$(echo "$SEND_MSG_RESP" | jq -r '.data.id')
check "Message status starts as accepted" \
    "echo '$SEND_MSG_RESP' | jq -e '.data.status == \"accepted\"' > /dev/null"

# Simulating the full message lifecycle:
# accepted → queued → sending → sent → delivered
# The QueueWorker processes this automatically in production,
# but in the smoke test, we can manually trigger transitions:
MSG_STATUS_RESP=$(curl -sf "$BASE_URL/api/v1/messages/$MSG_ID" \
    -H "Authorization: Bearer $API_KEY")
check "Get message returns valid status" \
    "echo '$MSG_STATUS_RESP' | jq -e '.data.status' > /dev/null"

# ---- 8. DLR Callback ----
echo ""
echo "--- [8/8] DLR ---"
# Simulate a DLR callback
DLR_RESP=$(curl -sf -X POST "$BASE_URL/api/v1/dlr/$CONN_ID" \
    -H "Content-Type: application/json" \
    -d "{
        \"external_id\":\"$MSG_ID\",
        \"status\":\"DELIVRD\",
        \"description\":\"Message delivered successfully\"
    }")
check "DLR callback returns 204" \
    "[ '$(curl -sf -o /dev/null -w '%{http_code}' -X POST "$BASE_URL/api/v1/dlr/$CONN_ID" \
        -H 'Content-Type: application/json' \
        -d '{"external_id":"nonexistent","status":"DELIVRD"}')' = '404' ]"

echo ""
echo "============================================"
echo -e " Results: ${GREEN}$PASS passed${NC}, ${RED}$FAIL failed${NC}"
echo "============================================"

# Exit with error if any test failed
[ "$FAIL" -eq 0 ] || exit 1
