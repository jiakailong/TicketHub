#!/usr/bin/env bash
set -euo pipefail

APISIX_ADMIN="${APISIX_ADMIN:-http://127.0.0.1:9180}"
APISIX_ADMIN_KEY="${APISIX_ADMIN_KEY:?APISIX_ADMIN_KEY must be set}"
DEFAULT_BFF_UPSTREAM="host.docker.internal:8080"
if [[ -n "${WSL_DISTRO_NAME:-}" ]]; then
  WSL_HOST_IP="$(hostname -I | awk '{print $1}')"
  if [[ -n "${WSL_HOST_IP}" ]]; then
    DEFAULT_BFF_UPSTREAM="${WSL_HOST_IP}:8080"
  fi
fi
BFF_UPSTREAM="${BFF_UPSTREAM:-${DEFAULT_BFF_UPSTREAM}}"

put_route() {
  local id="$1"
  local uri="$2"
  local node="$3"
  local rate="$4"
  local burst="$5"
  curl -fsS "${APISIX_ADMIN}/apisix/admin/routes/${id}" \
    -H "X-API-KEY: ${APISIX_ADMIN_KEY}" \
    -H "Content-Type: application/json" \
    -X PUT \
    -d "{\"uri\":\"${uri}\",\"plugins\":{\"request-id\":{\"header_name\":\"X-Request-ID\",\"include_in_response\":true},\"cors\":{},\"limit-req\":{\"rate\":${rate},\"burst\":${burst},\"key\":\"remote_addr\",\"key_type\":\"var\",\"policy\":\"redis\",\"redis_host\":\"tickethub-redis\",\"redis_port\":6379,\"redis_database\":0,\"nodelay\":true,\"rejected_code\":429},\"prometheus\":{}},\"upstream\":{\"type\":\"roundrobin\",\"nodes\":{\"${node}\":1}}}"
}

put_route tickethub-bff-api "/api/*" "${BFF_UPSTREAM}" 300 100
put_route tickethub-bff-health "/healthz" "${BFF_UPSTREAM}" 60 20

curl -fsS "${APISIX_ADMIN}/apisix/admin/routes/tickethub-create-order" \
  -H "X-API-KEY: ${APISIX_ADMIN_KEY}" \
  -H "Content-Type: application/json" \
  -X PUT \
  -d "{\"uri\":\"/api/orders\",\"methods\":[\"POST\"],\"priority\":100,\"plugins\":{\"request-id\":{\"header_name\":\"X-Request-ID\",\"include_in_response\":true},\"cors\":{},\"limit-req\":{\"rate\":1000,\"burst\":500,\"key\":\"remote_addr\",\"key_type\":\"var\",\"policy\":\"redis\",\"redis_host\":\"tickethub-redis\",\"redis_port\":6379,\"redis_database\":0,\"nodelay\":true,\"rejected_code\":429},\"prometheus\":{}},\"upstream\":{\"type\":\"roundrobin\",\"nodes\":{\"${BFF_UPSTREAM}\":1}}}"
