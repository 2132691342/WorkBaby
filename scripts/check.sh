#!/usr/bin/env bash
# 全量门禁：格式 → 静态检查 → 测试 → 前端类型检查 → 前端构建。
# 与 CI（.github/workflows/ci.yml）跑同一套。
# 编译警告由 clippy -D warnings 全量拦截、测试构建兜底链接，不再单跑 cargo build。
#
# 用法：
#   bash scripts/check.sh              跑全部门禁
#   bash scripts/check.sh backend      只跑后端三道（格式/静态/测试）
#   bash scripts/check.sh frontend     只跑前端两道（类型检查/构建）
#   bash scripts/check.sh wb-features  只验一个 crate（改哪验哪）
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

run_crate() {
  step "格式检查($1)" cargo fmt -p "$1" -- --check
  step "静态检查($1)" cargo clippy -p "$1" --all-targets -- -D warnings
  step "测试($1)" cargo test -p "$1"
}

run_backend() {
  step "格式检查" cargo fmt --all -- --check
  step "静态检查" cargo clippy --workspace --all-targets -- -D warnings
  step "测试" cargo test --workspace
}

run_frontend() {
  cd "$ROOT/frontend"
  step "前端类型检查" npm run typecheck
  step "前端构建" npm run build
}

case "$SCOPE" in
  all)      run_backend; run_frontend ;;
  backend)  run_backend ;;
  frontend) run_frontend ;;
  wb-api | wb-domain | wb-features | wb-git | wb-kernel | wb-mcp | wb-pkg | wb-provider | wb-repo | wb-service | wb-skills | wb-tools)
    run_crate "$SCOPE"
    ;;
  *) echo "未知范围：$SCOPE（可选 all / backend / frontend 或 crate 名）"; exit 2 ;;
esac

echo ""
echo "==> 全部通过"
