#!/usr/bin/env bash
# 慢速检查：跑依赖真实外部环境的测试（真 git 子进程等）。
# 用法：bash scripts/check-slow.sh
set -euo pipefail

cd "$(dirname "$0")/.."

echo "==> git 域真实仓库链路"
cargo test -p wb-git -- --ignored

echo "==> 全部通过"
