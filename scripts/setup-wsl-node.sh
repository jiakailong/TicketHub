#!/usr/bin/env bash
# 启用 ~/.node 的 node v22 + pnpm（修复 PATH 优先级，避免旧 v12 node 抢占）
set -uo pipefail

if ! grep -q 'HOME/.node/bin' ~/.bashrc 2>/dev/null; then
  echo 'export PATH=$HOME/.node/bin:$PATH' >> ~/.bashrc
  echo "bashrc updated"
fi
export PATH="$HOME/.node/bin:$PATH"
hash -r
echo "node: $(node --version)"
echo "npm:  $(npm --version)"
corepack enable 2>&1 | head -1
# corepack 签名校验在国内网络常失败，用 npm 装真实 pnpm（--force 覆盖 corepack shim）
npm install -g pnpm --force --registry=https://registry.npmmirror.com > /tmp/npm-pnpm.log 2>&1
tail -5 /tmp/npm-pnpm.log
echo "pnpm: $(pnpm --version)"
