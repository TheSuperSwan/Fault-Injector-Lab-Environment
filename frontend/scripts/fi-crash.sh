#!/usr/bin/env bash
set -euo pipefail

: "${FAULTS_TOKEN:?FAULTS_TOKEN is not set}"

BASE_URL="${BASE_URL:-http://localhost:8080}"

curl -sS -X POST \
  -H "Authorization: Bearer ${FAULTS_TOKEN}" \
  "${BASE_URL}/api/admin/faults/crash"

echo
echo "Triggered backend crash (pod should restart)"
