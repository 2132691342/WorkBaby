#!/usr/bin/env bash
# 全量检查：后端编译 + 后端测试 + 前端类型检查 + 前端构建。
# 用法：bash scripts/check.sh
set -euo pipefail

cd "$(dirname "$0")/.."
ROOT=$(pwd)

echo "==> 后端编译"
cargo build --workspace --all-targets

echo "==> 后端测试"
cargo test --workspace

echo "==> 前端类型检查"
cd "$ROOT/frontend"
npm run typecheck

echo "==> 前端构建"
npm run build

echo "==> 全部通过"
