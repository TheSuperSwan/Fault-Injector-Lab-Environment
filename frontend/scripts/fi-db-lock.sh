#!/usr/bin/env bash
set -euo pipefail

: "${FAULTS_TOKEN:?FAULTS_TOKEN is not set}"

BASE_URL="${BASE_URL:-http://localhost:8080}"

SECONDS="${1:-20}"

curl -sS -X POST \
  -H "Authorization: Bearer ${FAULTS_TOKEN}" \
  -H "Content-Type: application/json" \
  "${BASE_URL}/api/admin/faults/db_lock/enable" \
  -d "{\"seconds\":${SECONDS}}"

echo
echo "Enabled DB-lock fault for ${SECONDS}s (locks table 'entries')"
