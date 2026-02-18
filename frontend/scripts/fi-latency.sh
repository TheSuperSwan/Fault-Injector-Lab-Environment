#!/usr/bin/env bash
set -euo pipefail
: "${FAULTS_TOKEN:?FAULTS_TOKEN is not set}"

BASE_URL="${BASE_URL:-http://localhost:8080}"

# Always 10 seconds, always on, permanent until reset
curl -sS -X POST \
  -H "Authorization: Bearer ${FAULTS_TOKEN}" \
  -H "Content-Type: application/json" \
  "${BASE_URL}/api/admin/faults/latency/enable" \
  -d '{"ms":10000,"percent":100,"paths":[],"durationSeconds":0}'

echo
echo "Enabled latency: 10s on all /api/* until fi-reset.sh"
