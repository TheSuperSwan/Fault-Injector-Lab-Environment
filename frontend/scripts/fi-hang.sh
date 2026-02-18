#!/usr/bin/env bash
set -euo pipefail
: "${FAULTS_TOKEN:?FAULTS_TOKEN is not set}"

BASE_URL="${BASE_URL:-http://localhost:8080}"

curl -sS -X POST \
  -H "Authorization: Bearer ${FAULTS_TOKEN}" \
  -H "Content-Type: application/json" \
  "${BASE_URL}/api/admin/faults/hang/enable" \
  -d '{"durationSeconds":0}'

echo
echo "Enabled BACKEND HANG (API will stop responding) until fi-reset.sh"
echo "Non-reset recovery: oc rollout restart deployment/file-backend"
