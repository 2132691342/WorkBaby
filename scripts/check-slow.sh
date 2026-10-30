#!/usr/bin/env bash
# 慢速门禁：跑依赖真实外部环境的测试（真 git 仓库子进程等）。
# 这些用例默认被 #[ignore] 跳过——全量门禁不跑它们，因为它们依赖用户机上的 git。
#
# 用法：bash scripts/check-slow.sh
set -euo pipefail

cd "$(dirname "$0")/.."

echo "==> git 域真实仓库链路"
cargo test -p wb-git -- --ignored --nocapture

echo ""
echo "==> 全部通过"
