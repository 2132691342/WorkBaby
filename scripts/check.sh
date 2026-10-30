#!/usr/bin/env bash
# 全量门禁：格式 → 静态检查 → 编译 → 测试 → 前端类型检查 → 前端构建。
# 与 CI（.github/workflows/ci.yml）跑同一套，提交前本地先过一遍。
#
# 用法：
#   bash scripts/check.sh              跑全部门禁
#   bash scripts/check.sh backend      只跑后端四道（格式/静态/编译/测试）
#   bash scripts/check.sh frontend     只跑前端两道（类型检查/构建）
set -euo pipefail

cd "$(dirname "$0")/.."
ROOT=$(pwd)
SCOPE="${1:-all}"

# 跑一道检查；失败立刻停并指出是哪一道。
step() {
  echo ""
  echo "==> $1"
  shift
  "$@"
}

run_backend() {
  step "格式检查" cargo fmt --all -- --check
  step "静态检查" cargo clippy --workspace --all-targets -- -D warnings
  step "编译" cargo build --workspace --all-targets
  step "测试" cargo test --workspace
}

run_frontend() {
  cd "$ROOT/frontend"
  step "前端类型检查" npm run typecheck
  step "前端构建" npm run build
}

case "$SCOPE" in
  backend)  run_backend ;;
  frontend) run_frontend ;;
  all)      run_backend; run_frontend ;;
  *) echo "未知范围：$SCOPE（可选 all / backend / frontend）"; exit 2 ;;
esac

echo ""
echo "==> 全部通过"
