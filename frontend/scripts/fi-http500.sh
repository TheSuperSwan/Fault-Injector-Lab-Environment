#!/usr/bin/env bash
set -euo pipefail
: "${FAULTS_TOKEN:?FAULTS_TOKEN is not set}"

BASE_URL="${BASE_URL:-http://localhost:8080}"

curl -sS -X POST \
  -H "Authorization: Bearer ${FAULTS_TOKEN}" \
  -H "Content-Type: application/json" \
  "${BASE_URL}/api/admin/faults/http500/enable" \
  -d '{"percent":50,"paths":[],"durationSeconds":0}'

echo
echo "Enabled HTTP500: 50% on all /api/* until fi-reset.sh"
