#!/usr/bin/env bash
set -euo pipefail

# Cage quickstart — create a sandbox, run a command, clean up.
# Requires: CAGE_SERVER (default http://localhost:8080), CAGE_API_KEY

SERVER="${CAGE_SERVER:-http://localhost:8080}"
if [ -z "${CAGE_API_KEY:-}" ]; then
  echo "Error: set CAGE_API_KEY (generate one with 'make genkey' on the server)" >&2
  exit 1
fi

echo "→ Creating a Python sandbox..."
SANDBOX=$(curl -sf -X POST "$SERVER/sandboxes" \
  -H "Authorization: Bearer $CAGE_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"template": "python-3.12"}')

SANDBOX_ID=$(echo "$SANDBOX" | grep -o '"id":"[^"]*"' | cut -d'"' -f4)
echo "  created: $SANDBOX_ID"

echo "→ Running a command inside it..."
curl -sf -X POST "$SERVER/sandboxes/$SANDBOX_ID/exec" \
  -H "Authorization: Bearer $CAGE_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"cmd": ["python3", "-c", "print(2 + 2)"]}'

echo
echo "→ Cleaning up..."
curl -sf -X DELETE "$SERVER/sandboxes/$SANDBOX_ID" \
  -H "Authorization: Bearer $CAGE_API_KEY"

echo "Done."