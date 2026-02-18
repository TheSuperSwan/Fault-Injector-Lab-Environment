#!/usr/bin/env bash
set -euo pipefail
: "${FAULTS_TOKEN:?FAULTS_TOKEN is not set}"

BASE_URL="${BASE_URL:-http://localhost:8080}"

# Permanent DB lock until reset (or manual termination of the locking DB session)
curl -sS -X POST \
  -H "Authorization: Bearer ${FAULTS_TOKEN}" \
  -H "Content-Type: application/json" \
  "${BASE_URL}/api/admin/faults/db_lock/enable" \
  -d '{"durationSeconds":0}'

echo
echo "Enabled DB-lock (entries table) until fi-reset.sh"
echo "Non-reset recovery option: find & terminate the locking session in Postgres."
