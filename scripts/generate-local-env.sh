#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ROOT_DIR}/.env"

if [[ -e "${ENV_FILE}" ]]; then
  echo "${ENV_FILE} already exists; refusing to overwrite it"
  exit 1
fi

command -v openssl >/dev/null 2>&1 || {
  echo "openssl is required to generate local secrets"
  exit 1
}

random_hex() {
  openssl rand -hex "$1"
}

random_base64_key() {
  openssl rand -base64 32 | tr -d '\n'
}

cat >"${ENV_FILE}" <<EOF
TICKETHUB_MYSQL_HOST=127.0.0.1:3306
TICKETHUB_MYSQL_USER=tickethub
TICKETHUB_MYSQL_PASSWORD=$(random_hex 24)
TICKETHUB_MYSQL_ROOT_PASSWORD=$(random_hex 24)

TICKETHUB_JWT_SECRET=$(random_hex 32)
TICKETHUB_ADMIN_MOBILES=
TICKETHUB_PRIVACY_ACTIVE_KEY_VERSION=v1
TICKETHUB_PRIVACY_ENCRYPTION_KEYS=v1:$(random_base64_key)
TICKETHUB_PRIVACY_LOOKUP_KEY=$(random_base64_key)
TICKETHUB_REGISTER_PROTECTION_HMAC_SECRET=$(random_base64_key)

APISIX_ADMIN_KEY=$(random_hex 32)
TICKETHUB_GRAFANA_ADMIN_USER=admin
TICKETHUB_GRAFANA_ADMIN_PASSWORD=$(random_hex 24)
EOF

chmod 600 "${ENV_FILE}"
echo "Created ${ENV_FILE} with random local-development secrets"
