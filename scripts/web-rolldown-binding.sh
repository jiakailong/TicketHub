#!/usr/bin/env bash
# 补装 rolldown linux 平台绑定（vite 8 原生模块）
set -uo pipefail
export PATH="$HOME/.node/bin:$PATH"
cd "$(dirname "$0")/../web"
pnpm add -D @rolldown/binding-linux-x64-gnu@1.1.5 --registry=https://registry.npmmirror.com 2>&1 | tail -4
