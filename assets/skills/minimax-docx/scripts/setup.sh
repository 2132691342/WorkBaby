#!/usr/bin/env bash
# minimax-docx 安装脚本：在带 bash 的 Windows 环境（Git Bash / WSL）里跑。
# macOS / Linux 的包管理器分支已去除——本产品只支持 Windows。
# Windows PowerShell 用户请改跑 setup.ps1。
set -euo pipefail

# 强制 dotnet CLI 英文输出。
export DOTNET_CLI_UI_LANGUAGE=en

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DOTNET_DIR="$SCRIPT_DIR/dotnet"
LOG_FILE="$PROJECT_DIR/.setup.log"

log()   { echo -e "\033[0;32m[OK]\033[0m    $*"; }
warn()  { echo -e "\033[1;33m[WARN]\033[0m  $*"; }
fail()  { echo -e "\033[0;31m[FAIL]\033[0m  $*"; }

# 装 .NET 8 SDK：已有则校验版本；没有则下载微软官方安装脚本落本地。
install_dotnet() {
    echo -e "\n=== 检查 .NET SDK ==="
    if command -v dotnet &>/dev/null; then
        local ver major
        ver=$(dotnet --version 2>/dev/null || echo "0")
        major="${ver%%.*}"
        if [ "$major" -ge 8 ] 2>/dev/null; then
            log "dotnet $ver 已就绪 (>= 8.0)"
            return 0
        else
            warn "dotnet $ver 低于 8.0，需升级"
        fi
    fi
    warn "未检测到 .NET 8，请到 https://dotnet.microsoft.com/download 安装后重跑本脚本。"
    return 1
}

# 可选：pandoc / LibreOffice / zip 缺失只告警，不阻断。
install_optional() {
    command -v pandoc &>/dev/null && log "pandoc 已就绪" || warn "pandoc 缺失（预览回退）"
    command -v soffice &>/dev/null && log "soffice 已就绪" || warn "soffice 缺失（不能转 .doc）"
    if command -v zip &>/dev/null && command -v unzip &>/dev/null; then
        log "zip/unzip 已就绪"
    else
        warn "zip/unzip 缺失（可选，.NET 原生处理 DOCX）"
    fi
}

# 还原 + 构建 .NET 工程。
build_project() {
    echo -e "\n=== 构建 minimax-docx 工程 ==="
    [ -d "$DOTNET_DIR" ] || { fail "工程目录缺失：$DOTNET_DIR"; return 1; }
    cd "$DOTNET_DIR"
    dotnet restore --verbosity quiet 2>>"$LOG_FILE" || { fail "NuGet 还原失败，检查 $LOG_FILE"; return 1; }
    log "NuGet 包已还原"
    dotnet build --verbosity quiet --no-restore 2>>"$LOG_FILE" || { fail "构建失败，检查 $LOG_FILE"; return 1; }
    log "工程构建成功"
    cd "$PROJECT_DIR"
}

# 给脚本加可执行位。
fix_permissions() {
    echo -e "\n=== 设置脚本权限 ==="
    for s in "$SCRIPT_DIR"/*.sh; do
        [ -f "$s" ] && chmod +x "$s" && log "chmod +x $(basename "$s")"
    done
}

# 跑一次 create 验证链路通。
verify_installation() {
    echo -e "\n=== 验证 ==="
    local test_output="/tmp/minimax-docx-setup-test-$$.docx"
    if (cd "$DOTNET_DIR" && dotnet run --project MiniMaxAIDocx.Cli -- create \
        --type report --output "$test_output" --title "Setup Test" 2>>"$LOG_FILE"); then
        rm -f "$test_output"
        log "验证通过 —— minimax-docx 可用"
    else
        fail "验证文档创建失败，检查 $LOG_FILE"
        return 1
    fi
    cd "$PROJECT_DIR"
}

main() {
    echo "======== minimax-docx 安装 ========"
    : > "$LOG_FILE"
    install_dotnet && install_optional && build_project && fix_permissions && verify_installation
    echo ""
    echo "用法："
    echo "  dotnet run --project $DOTNET_DIR/MiniMaxAIDocx.Cli -- create --type report --output my_report.docx"
    echo "  bash $SCRIPT_DIR/env_check.sh   # 快速环境检查"
}

main "$@"
