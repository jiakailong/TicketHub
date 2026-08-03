#!/usr/bin/env bash
# 在 minikube 节点内构建业务镜像。
# 背景：宿主机网络 Docker Hub 直连受限，而 minikube 节点内网络可访问 Docker Hub/gcr.io，
#       故用 minikube docker-env 把 docker CLI 指向节点内 daemon，在节点内完成构建。
# 用法: ./scripts/build-k8s-images.sh [svc ...]，默认构建 deploy/k8s/tickethub-services.yaml 中的 4 个服务
set -euo pipefail

cd "$(dirname "$0")/.."

if [ "$#" -gt 0 ]; then
  SERVICES=("$@")
else
  SERVICES=(gateway-bff user-service program-service order-service pay-service \
    base-data-service customize-service admin-service migrate-service)
fi

# 指向 minikube 节点内 docker daemon
eval "$(minikube docker-env)"

for svc in "${SERVICES[@]}"; do
  echo "=== building $svc ==="
  docker build \
    -f deploy/docker/service.Dockerfile \
    --build-arg SERVICE="$svc" \
    -t "tickethub/$svc:local" .
done

echo "=== built images in minikube node ==="
docker images | grep tickethub
