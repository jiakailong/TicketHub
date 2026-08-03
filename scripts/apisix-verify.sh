#!/usr/bin/env bash
# 验证 k8s APISIX：admin 路由表 + 对外端口
set -uo pipefail
cd "$(dirname "$0")/.."

KEY=$(grep "APISIX_ADMIN_KEY=" .env | head -1 | cut -d= -f2 | tr -d '\r')
echo "admin key len: ${#KEY}"

echo "=== admin 路由表 (k8s APISIX) ==="
curl -s -H "X-API-KEY: $KEY" http://127.0.0.1:9180/apisix/admin/routes \
  | grep -oE '"id":"[^"]+"'

echo "=== 对外 /healthz (port-forward 19080) ==="
curl -s -o /dev/null -w "HTTP %{http_code}\n" http://127.0.0.1:19080/healthz
