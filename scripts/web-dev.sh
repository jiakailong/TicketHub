#!/usr/bin/env bash
# 前端 dev server（代理 /api -> 127.0.0.1:9080 APISIX）
set -uo pipefail
export PATH="$HOME/.node/bin:$PATH"
cd "$(dirname "$0")/../web"
exec pnpm dev --host 0.0.0.0
