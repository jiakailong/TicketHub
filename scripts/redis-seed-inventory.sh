#!/usr/bin/env bash
set -euo pipefail

REDIS_HOST="${REDIS_HOST:-127.0.0.1}"
REDIS_PORT="${REDIS_PORT:-6379}"
REDIS_CLI="${REDIS_CLI:-redis-cli}"
REDIS_CONTAINER="${REDIS_CONTAINER:-tickethub-redis}"

if command -v "${REDIS_CLI}" >/dev/null 2>&1; then
  redis_cli=("${REDIS_CLI}" -h "${REDIS_HOST}" -p "${REDIS_PORT}")
else
  redis_cli=(docker exec "${REDIS_CONTAINER}" redis-cli)
fi

"${redis_cli[@]}" MSET \
  tickethub:program:inventory:10001:1 1000 \
  tickethub:program:inventory:10001:2 2000 \
  tickethub:program:inventory:10002:3 5000 \
  tickethub:program:inventory:10003:4 800 >/dev/null

echo "TicketHub Redis inventory seeded at ${REDIS_HOST}:${REDIS_PORT}"
