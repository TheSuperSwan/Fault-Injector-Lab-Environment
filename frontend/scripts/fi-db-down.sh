#!/usr/bin/env bash
set -euo pipefail

: "${FAULTS_TOKEN:?FAULTS_TOKEN is not set}"

BASE_URL="${BASE_URL:-http://localhost:8080}"

DURATION="${1:-45}"

curl -sS -X POST \
  -H "Authorization: Bearer ${FAULTS_TOKEN}" \
  -H "Content-Type: application/json" \
  "${BASE_URL}/api/admin/faults/db_down/enable" \
  -d "{\"durationSeconds\":${DURATION}}"

echo
echo "Enabled DB-down fault for ${DURATION}s"
