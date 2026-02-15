#!/usr/bin/env bash
set -euo pipefail

: "${FAULTS_TOKEN:?FAULTS_TOKEN is not set}"

BASE_URL="${BASE_URL:-http://localhost:8080}"

PERCENT="${1:-50}"
DURATION="${2:-30}"
PATHS_JSON="${3:-[\"/api/entries\"]}"

curl -sS -X POST \
  -H "Authorization: Bearer ${FAULTS_TOKEN}" \
  -H "Content-Type: application/json" \
  "${BASE_URL}/api/admin/faults/http500/enable" \
  -d "{\"percent\":${PERCENT},\"paths\":${PATHS_JSON},\"durationSeconds\":${DURATION}}"

echo
echo "Enabled HTTP500 fault: ${PERCENT}%, duration ${DURATION}s, paths=${PATHS_JSON}"
