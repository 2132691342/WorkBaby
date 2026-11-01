# minimax-docx 安装脚本（Windows PowerShell）。本产品只支持 Windows。
#Requires -Version 5.1

param(
    [switch]$Minimal,    # 仅装关键依赖，跳过 pandoc / LibreOffice
    [switch]$SkipVerify,
    [switch]$Help
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectDir = Split-Path -Parent $ScriptDir
$DotnetDir = Join-Path $ScriptDir "dotnet"
$LogFile = Join-Path $ProjectDir ".setup.log"

function Log  { Write-Host "[OK]    $args" -ForegroundColor Green }
function Warn { Write-Host "[WARN]  $args" -ForegroundColor Yellow }
function Fail { Write-Host "[FAIL]  $args" -ForegroundColor Red }
function Info { Write-Host "[INFO]  $args" -ForegroundColor Cyan }
function Step { Write-Host "`n=== $args ===" -ForegroundColor Blue }

if ($Help) {
    Write-Host "用法: setup.ps1 [-Minimal] [-SkipVerify] [-Help]"
    exit 0
}

# 本会话强制 UTF-8 输出，避免 CJK 乱码。
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

Write-Host "======== minimax-docx 安装（Windows） ========"
"" | Set-Content $LogFile

# 包管理器探测（仅用于提示，安装 .NET / pandoc / LibreOffice 时优先 winget）。
$HasWinget = $null -ne (Get-Command winget -ErrorAction SilentlyContinue)
$HasChoco  = $null -ne (Get-Command choco -ErrorAction SilentlyContinue)
$HasScoop  = $null -ne (Get-Command scoop -ErrorAction SilentlyContinue)
if ($HasWinget) { Info "包管理器: winget" } elseif ($HasChoco) { Info "包管理器: chocolatey" } elseif ($HasScoop) { Info "包管理器: scoop" }
else { Warn "未检测到 winget / choco / scoop，部分依赖需手动安装" }

# --- .NET 8 SDK ---
Step "检查 .NET SDK"
$dotnetCmd = Get-Command dotnet -ErrorAction SilentlyContinue
if ($dotnetCmd) {
    $ver = & dotnet --version 2>$null
    if ([int]($ver -split '\.')[0] -ge 8) { Log "dotnet $ver 已就绪 (>= 8.0)" }
    else { Warn "dotnet $ver 低于 8.0"; $dotnetCmd = $null }
}
if (-not $dotnetCmd) {
    Info "安装 .NET 8 SDK..."
    if ($HasWinget)    { winget install Microsoft.DotNet.SDK.8 --accept-source-agreements --accept-package-agreements 2>>$LogFile }
    elseif ($HasChoco) { choco install dotnet-sdk -y 2>>$LogFile }
    elseif ($HasScoop) { scoop install dotnet-sdk 2>>$LogFile }
    else { Fail "无法自动安装 .NET SDK，请到 https://dotnet.microsoft.com/download 安装后重跑"; exit 1 }
    $env:Path = [Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [Environment]::GetEnvironmentVariable("Path","User")
    if (-not (Get-Command dotnet -ErrorAction SilentlyContinue)) { Fail "dotnet 安装失败，请重启 PowerShell 重试"; exit 1 }
    Log "dotnet $(dotnet --version) 已安装"
}

# --- 可选：pandoc / LibreOffice ---
if (-not $Minimal) {
    Step "检查 pandoc / LibreOffice（可选）"
    if (Get-Command pandoc -ErrorAction SilentlyContinue) { Log "pandoc 已就绪" }
    else {
        Info "安装 pandoc..."
        if ($HasWinget)    { winget install JohnMacFarlane.Pandoc 2>>$LogFile }
        elseif ($HasChoco) { choco install pandoc -y 2>>$LogFile }
        elseif ($HasScoop) { scoop install pandoc 2>>$LogFile }
        else { Warn "pandoc 未安装（预览回退）" }
    }
    $sofficePaths = @(
        "C:\Program Files\LibreOffice\program\soffice.exe",
        "C:\Program Files (x86)\LibreOffice\program\soffice.exe",
        "${env:LOCALAPPDATA}\Programs\LibreOffice\program\soffice.exe"
    )
    $sofficeOk = (Get-Command soffice -ErrorAction SilentlyContinue) -or ($sofficePaths | Where-Object { Test-Path $_ })
    if ($sofficeOk) { Log "soffice 已就绪（.doc 可转 docx）" }
    else {
        Info "安装 LibreOffice..."
        if ($HasWinget)    { winget install TheDocumentFoundation.LibreOffice 2>>$LogFile }
        elseif ($HasChoco) { choco install libreoffice-fresh -y 2>>$LogFile }
        else { Warn "LibreOffice 未安装（不能转 .doc）" }
    }
}

# --- NuGet 源 ---
Step "检查 NuGet 源"
if (& dotnet nuget list source 2>$null | Select-String "nuget.org") { Log "nuget.org 已配置" }
else { Warn "nuget.org 不在源列表，尝试添加"; & dotnet nuget add source "https://api.nuget.org/v3/index.json" --name "nuget.org" 2>>$LogFile }

# --- 构建工程 ---
Step "构建 minimax-docx 工程"
if (-not (Test-Path $DotnetDir)) { Fail "工程目录缺失：$DotnetDir"; exit 1 }
Push-Location $DotnetDir
$restore = & dotnet restore --verbosity quiet 2>&1
if ($LASTEXITCODE -ne 0) { Fail "NuGet 还原失败："; $restore | ForEach-Object { Fail "  $_" }; Pop-Location; exit 1 }
Log "NuGet 包已还原"
$build = & dotnet build --verbosity quiet --no-restore 2>&1
if ($LASTEXITCODE -ne 0) { Fail "构建失败："; $build | ForEach-Object { Fail "  $_" }; Pop-Location; exit 1 }
Log "工程构建成功"
Pop-Location

# --- 验证 ---
if (-not $SkipVerify) {
    Step "验证"
    $testOutput = Join-Path $env:TEMP "minimax-docx-setup-test-$PID.docx"
    Push-Location $DotnetDir
    $result = & dotnet run --project MiniMaxAIDocx.Cli -- create --type report --output $testOutput --title "Setup Test" 2>&1
    $code = $LASTEXITCODE
    Pop-Location
    if ($code -eq 0 -and (Test-Path $testOutput)) {
        Remove-Item $testOutput -Force
        Log "验证通过 —— minimax-docx 可用"
    } else {
        Fail "验证失败："; $result | ForEach-Object { Fail "  $_" }
    }
}

Step "安装完成"
Write-Host ""
Write-Host "  .NET SDK: $(dotnet --version 2>$null)"
Write-Host "  工程目录: $DotnetDir"
Write-Host "  用法: dotnet run --project $DotnetDir\MiniMaxAIDocx.Cli -- create --type report --output my_report.docx"
Write-Host "  日志: $LogFile"
