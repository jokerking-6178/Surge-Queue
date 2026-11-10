#!/bin/bash
set -e

BASE_URL="${1:-http://localhost:3000}"
FAILED=0

echo "=== Smoke Test: $BASE_URL ==="

# Step 1: Join queue
echo -n "Step 1: Join queue... "
JOIN_RESPONSE=$(curl -s -X POST "$BASE_URL/api/queue/enter" -H "Content-Type: application/json" -d '{}')
USER_ID=$(echo "$JOIN_RESPONSE" | python3 -c "import sys, json; print(json.load(sys.stdin).get('user_id', ''))")
if [ -n "$USER_ID" ]; then
  echo "PASS (user_id: $USER_ID)"
else
  echo "FAIL (no user_id)"
  echo "Response: $JOIN_RESPONSE"
  FAILED=1
fi

# Step 2: Poll until admitted
echo -n "Step 2: Poll until admitted... "
ADMITTED=false
MAX_ATTEMPTS=30
ATTEMPT=0
while [ $ATTEMPT -lt $MAX_ATTEMPTS ] && [ "$ADMITTED" = false ]; do
  STATUS_RESPONSE=$(curl -s "$BASE_URL/api/queue/status/$USER_ID")
  STATE=$(echo "$STATUS_RESPONSE" | python3 -c "import sys, json; print(json.load(sys.stdin).get('status', {}).get('state', ''))")
  if [ "$STATE" = "admitted" ]; then
    ADMITTED=true
    ADMIT_TOKEN=$(echo "$STATUS_RESPONSE" | python3 -c "import sys, json; print(json.load(sys.stdin).get('status', {}).get('admit_token', ''))")
  fi
  ATTEMPT=$((ATTEMPT + 1))
  sleep 2
done

if [ "$ADMITTED" = true ]; then
  echo "PASS (got token)"
else
  echo "FAIL (not admitted after $MAX_ATTEMPTS attempts)"
  echo "Last state: $STATE"
  FAILED=1
fi

# Step 3: Register with token
echo -n "Step 3: Register with token... "
if [ "$ADMITTED" = true ]; then
  REGISTER_RESPONSE=$(curl -s -X POST "$BASE_URL/api/register" -H "Content-Type: application/json" -H "Authorization: Bearer $ADMIT_TOKEN" -d '{"name":"Test User","email":"test@example.com"}')
  REG_ID=$(echo "$REGISTER_RESPONSE" | python3 -c "import sys, json; print(json.load(sys.stdin).get('registration_id', ''))")
  if [ -n "$REG_ID" ]; then
    echo "PASS (registration_id: $REG_ID)"
  else
    echo "FAIL (no registration_id)"
    echo "Response: $REGISTER_RESPONSE"
    FAILED=1
  fi
else
  echo "SKIP (no token)"
fi

# Step 4: Register without token
echo -n "Step 4: Register without token (expect 401)... "
NO_TOKEN_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/register" -H "Content-Type: application/json" -d '{"name":"Test User","email":"test@example.com"}')
HTTP_CODE=$(echo "$NO_TOKEN_RESPONSE" | tail -n1)
if [ "$HTTP_CODE" = "401" ]; then
  echo "PASS (got 401)"
else
  echo "FAIL (got $HTTP_CODE, expected 401)"
  FAILED=1
fi

echo ""
if [ $FAILED -eq 0 ]; then
  echo "=== ALL TESTS PASSED ==="
  exit 0
else
  echo "=== SOME TESTS FAILED ==="
  exit 1
fi
