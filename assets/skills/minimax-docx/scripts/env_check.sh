#!/usr/bin/env bash
# minimax-docx 环境检查：在任何带 bash 的环境里跑（Windows 用 Git Bash / WSL）。
# 正式操作前先跑一遍；关键依赖缺失直接退出非零。
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DOTNET_DIR="$SCRIPT_DIR/dotnet"

# 强制 dotnet CLI 英文输出，便于解析版本号。
export DOTNET_CLI_UI_LANGUAGE=en

STATUS="READY"

# --- .NET SDK ---
if ! command -v dotnet &>/dev/null; then
    printf "[FAIL]    dotnet 未找到 —— 安装 .NET 8 SDK：https://dotnet.microsoft.com/download\n"
    STATUS="NOT READY"
else
    local_ver=$(dotnet --version 2>/dev/null || echo "0.0.0")
    local_major="${local_ver%%.*}"
    if [ "$local_major" -ge 8 ] 2>/dev/null; then
        printf "[OK]      dotnet %s (>= 8.0)\n" "$local_ver"
    else
        printf "[FAIL]    dotnet %s 需要 >= 8.0\n" "$local_ver"
        STATUS="NOT READY"
    fi
fi

# --- .NET 工程（含 NuGet 还原 / 构建） ---
if [ -d "$DOTNET_DIR" ]; then
    if [ -f "$DOTNET_DIR/MiniMaxAIDocx.Cli/bin/Debug/net10.0/MiniMaxAIDocx.Cli.dll" ] || \
       [ -f "$DOTNET_DIR/MiniMaxAIDocx.Cli/bin/Debug/net8.0/MiniMaxAIDocx.Cli.dll" ]; then
        printf "[OK]      project 已构建\n"
    elif dotnet restore "$DOTNET_DIR" --verbosity quiet &>/dev/null && \
         dotnet build "$DOTNET_DIR" --verbosity quiet --no-restore &>/dev/null; then
        printf "[OK]      project 已还原并构建\n"
    else
        printf "[FAIL]    project 还原或构建失败\n"
        STATUS="NOT READY"
    fi
else
    printf "[FAIL]    project 目录缺失：%s\n" "$DOTNET_DIR"
    STATUS="NOT READY"
fi

# --- 可选：pandoc / LibreOffice / zip ---
if command -v pandoc &>/dev/null; then
    printf "[OK]      pandoc（内容预览可用）\n"
else
    printf "[WARN]    pandoc 未找到 —— 预览回退\n"
fi
if command -v soffice &>/dev/null; then
    printf "[OK]      soffice（.doc 可转 docx）\n"
else
    printf "[WARN]    soffice 未找到 —— 不能转 .doc\n"
fi
if command -v zip &>/dev/null && command -v unzip &>/dev/null; then
    printf "[OK]      zip/unzip 可用\n"
else
    printf "[WARN]    zip/unzip 缺失（可选，.NET 原生处理 DOCX）\n"
fi

# --- 区域（CJK 需要 UTF-8） ---
current_lang="${LANG:-}"
if [ -n "$current_lang" ] && echo "$current_lang" | grep -qi "utf-8\|utf8"; then
    printf "[OK]      locale %s\n" "$current_lang"
else
    printf "[WARN]    LANG 未设或非 UTF-8，CJK 可能异常：export LANG=en_US.UTF-8\n"
fi

# --- 脚本可执行权限 ---
perm_issues=0
for s in "$SCRIPT_DIR"/*.sh; do
    [ -f "$s" ] && [ ! -x "$s" ] && perm_issues=$((perm_issues + 1))
done
if [ "$perm_issues" -gt 0 ]; then
    printf "[WARN]    %d 个脚本不可执行：chmod +x scripts/*.sh\n" "$perm_issues"
else
    printf "[OK]      scripts 均可执行\n"
fi

echo ""
if [ "$STATUS" = "READY" ]; then
    echo "Status: READY"
else
    echo "Status: NOT READY —— 跑完整安装：bash scripts/setup.sh 或 powershell scripts/setup.ps1"
    exit 1
fi
