#!/usr/bin/env bash
set -euo pipefail

: "${FAULTS_TOKEN:?FAULTS_TOKEN is not set}"

BASE_URL="${BASE_URL:-http://localhost:8080}"

# Default values (you can override by passing args)
MS="${1:-3000}"
PERCENT="${2:-100}"
DURATION="${3:-60}"
PATHS_JSON="${4:-[\"/api/entries\"]}"

curl -sS -X POST \
  -H "Authorization: Bearer ${FAULTS_TOKEN}" \
  -H "Content-Type: application/json" \
  "${BASE_URL}/api/admin/faults/latency/enable" \
  -d "{\"ms\":${MS},\"percent\":${PERCENT},\"paths\":${PATHS_JSON},\"durationSeconds\":${DURATION}}"

echo
echo "Enabled latency fault: ${MS}ms, ${PERCENT}%, duration ${DURATION}s, paths=${PATHS_JSON}"