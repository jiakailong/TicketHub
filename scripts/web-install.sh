#!/usr/bin/env bash
# 前端依赖安装（npmmirror 加速）
set -uo pipefail
export PATH="$HOME/.node/bin:$PATH"
export CI=true    # 无 TTY 时自动确认删除旧 node_modules
cd "$(dirname "$0")/../web"
echo "pnpm: $(pnpm --version)"
pnpm install --registry=https://registry.npmmirror.com 2>&1 | tail -6
