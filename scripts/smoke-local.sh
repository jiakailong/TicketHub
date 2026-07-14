#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
TOKEN="${TOKEN:-}"

curl -fsS "${BASE_URL}/healthz" >/dev/null
curl -fsS "${BASE_URL}/api/programs/search?page=1&page_size=5" >/dev/null
curl -fsS "${BASE_URL}/api/base/areas?parent_id=0" >/dev/null

if [[ -n "${TOKEN}" ]]; then
  curl -fsS \
    -H "Authorization: Bearer ${TOKEN}" \
    "${BASE_URL}/api/admin/dashboard" >/dev/null
fi

echo "smoke check passed for ${BASE_URL}"
