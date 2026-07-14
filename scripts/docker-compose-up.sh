#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${TICKETHUB_HOST_IP:-}" && -n "${WSL_DISTRO_NAME:-}" ]]; then
  TICKETHUB_HOST_IP="$(hostname -I | awk '{print $1}')"
  export TICKETHUB_HOST_IP
fi

docker compose -p tickethub up -d
