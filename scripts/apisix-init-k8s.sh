#!/usr/bin/env bash
# 给 k8s 里的 APISIX 初始化路由（compose 的 bootstrap 默认指向 9181 即 compose APISIX）
set -uo pipefail
cd "$(dirname "$0")/.."

ENV_KEY=$(grep "APISIX_ADMIN_KEY=" .env | head -1 | cut -d= -f2 | tr -d '\r')
SECRET_KEY=$(kubectl get secret tickethub-secrets -n tickethub -o jsonpath='{.data.APISIX_ADMIN_KEY}' | base64 -d 2>/dev/null | tr -d '\r')

echo "env key:    ${ENV_KEY:0:12}... (len ${#ENV_KEY})"
echo "secret key: ${SECRET_KEY:0:12}... (len ${#SECRET_KEY})"
if [ -n "$ENV_KEY" ] && [ "$ENV_KEY" = "$SECRET_KEY" ]; then
  echo "=> keys MATCH"
else
  echo "=> keys DIFFERENT (Secret 是 k8s APISIX 实际校验的 key)"
fi

echo "=== 写入 k8s APISIX (127.0.0.1:9180) ==="
# 应用已全部入集群：upstream 指向集群内 gateway-bff（node 格式 host:port，无 scheme）
APISIX_ADMIN=http://127.0.0.1:9180 APISIX_ADMIN_KEY="$ENV_KEY" \
  BFF_UPSTREAM="gateway-bff:8080" bash scripts/apisix-bootstrap.sh > /tmp/bootstrap-k8s.log 2>&1
echo "bootstrap exit=$? (log: /tmp/bootstrap-k8s.log)"

echo "=== 验证 k8s APISIX 路由表 ==="
curl -s -H "X-API-KEY: $ENV_KEY" http://127.0.0.1:9180/apisix/admin/routes \
  | grep -oE '"id":"[^"]+"' || echo "(empty)"
