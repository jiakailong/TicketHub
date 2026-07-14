#!/usr/bin/env bash
set -euo pipefail

ES_URL="${ES_URL:-http://127.0.0.1:9200}"
INDEX="${INDEX:-tickethub_programs_current_v2}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MAPPING_FILE="${ROOT_DIR}/deploy/docker/elasticsearch/programs-index.json"

if curl -fsS "${ES_URL}/${INDEX}" >/dev/null; then
  echo "index ${INDEX} already exists"
  exit 0
fi

curl -fsS \
  -X PUT "${ES_URL}/${INDEX}" \
  -H "Content-Type: application/json" \
  --data-binary "@${MAPPING_FILE}"

echo
echo "index ${INDEX} created"
