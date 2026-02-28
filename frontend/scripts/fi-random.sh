#!/usr/bin/env bash
set -euo pipefail

SCRIPTS_DIR="${SCRIPTS_DIR:-/usr/local/bin}"

# Weighted random:
#  - latency: 30%
#  - http500: 25%
#  - db bad creds: 20%
#  - db lock: 15%
#  - hang: 10%
#
# Adjust weights if you want "hang" rarer.

roll=$(( RANDOM % 100 ))

if [ "$roll" -lt 30 ]; then
  pick="fi-latency.sh"
elif [ "$roll" -lt 55 ]; then
  pick="fi-http500.sh"
elif [ "$roll" -lt 75 ]; then
  pick="fi-db-bad-creds.sh"
elif [ "$roll" -lt 90 ]; then
  pick="fi-db-lock.sh"
else
  pick="fi-hang.sh"
fi

echo "Random pick: ${pick}"
exec "${SCRIPTS_DIR}/${pick}"
