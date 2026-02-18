#!/usr/bin/env bash
set -euo pipefail

# This script randomly triggers ONE of the fault scripts.
# You can optionally pass a duration/strength, but defaults are used if you don't.

SCRIPTS_DIR="${SCRIPTS_DIR:-/usr/local/bin}"

choices=(
  "fi-latency.sh"
  "fi-http500.sh"
  "fi-db-down.sh"
  "fi-db-lock.sh"
  "fi-hang.sh"
)

# Random pick
idx=$(( RANDOM % ${#choices[@]} ))
pick="${choices[$idx]}"

echo "Random pick: ${pick}"

# Call the chosen script with no args (defaults)
exec "${SCRIPTS_DIR}/${pick}"
