#!/usr/bin/env bash
set -euo pipefail
: "${FAULTS_TOKEN:?FAULTS_TOKEN is not set}"

BASE_URL="${BASE_URL:-http://localhost:8080}"

curl -sS -X POST \
  -H "Authorization: Bearer ${FAULTS_TOKEN}" \
  -H "Content-Type: application/json" \
  "${BASE_URL}/api/admin/faults/db_bad_creds/enable" \
  -d '{"durationSeconds":0}'

echo
echo "Enabled DB bad-credentials fault (persistent)."
echo "Recovery (NO reset):"
echo "  curl -X POST -H \"Authorization: Bearer \$FAULTS_TOKEN\" ${BASE_URL}/api/admin/faults/db_bad_creds/disable"
