<#
.SYNOPSIS
  分层测试入口。

.DESCRIPTION
  改一个小功能不必跑全量：
    默认            核心后端包 + 前端类型检查与单测
    -Pkg service    只跑指定后端包（逗号分隔，如 service,harness）
    -Run TestXxx    只跑匹配的用例（配合 -Pkg 精确定位，最快闭环）
    -NoFront        跳过前端检查（纯后端改动时用）
    -Full           全量后端 + 依赖方向门禁 + i18n 契约 + 前端
    -Short          透传 -short 给 go test（跳过时序敏感用例，CI 用）

.EXAMPLE
  powershell scripts/test.ps1 -Pkg service -Run TestInboxReviewFlow -NoFront
  powershell scripts/test.ps1 -Pkg harness
  powershell scripts/test.ps1 -Full
#>
param(
  [string]$Pkg = '',
  [string]$Run = '',
  [switch]$NoFront,
  [switch]$Full,
  [switch]$Short
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

# 注：CI 不能用 `go test ./...` —— main 包通过 go:embed 依赖 frontend/dist，
# CI 流水线在跑 wails build 之前没构建前端；编译 main 包会触发 setup failed。
# 用例都在 internal/ 包里，main 是 Wails 入口不含测试，排除即可。
$goArgs = @()
if ($Short) { $goArgs += '-short' }
if ($Run) { $goArgs += @('-run', $Run) }

function Invoke-GoTest([string[]]$targets) {
  Write-Host "== go test $($targets -join ' ') $($goArgs -join ' ') ==" -ForegroundColor Cyan
  go test @targets @goArgs
  if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}

function Invoke-FrontendChecks {
  Write-Host '== 前端类型检查 ==' -ForegroundColor Cyan
  Push-Location frontend
  npx vue-tsc --noEmit -p tsconfig.json
  if ($LASTEXITCODE -ne 0) { Pop-Location; exit $LASTEXITCODE }
  Write-Host '== 前端单测 ==' -ForegroundColor Cyan
  npx vitest run
  $code = $LASTEXITCODE
  Pop-Location
  if ($code -ne 0) { exit $code }
}

# 指定包：最小闭环（-Run 时可精确到单个用例）
if ($Pkg) {
  $targets = @($Pkg.Split(',') | ForEach-Object { "./internal/$($_.Trim())/..." })
  Invoke-GoTest $targets
  if (-not $NoFront) { Invoke-FrontendChecks }
  Write-Host '全部通过' -ForegroundColor Green
  exit 0
}

if ($Full) {
  Invoke-GoTest @('./internal/...')

  Write-Host '== 依赖方向门禁 ==' -ForegroundColor Cyan
  powershell -ExecutionPolicy Bypass -File scripts/check-boundaries.ps1
  if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

  Write-Host '== i18n 双语契约 ==' -ForegroundColor Cyan
  if (Get-Command node -ErrorAction SilentlyContinue) {
    node scripts/i18n-sync.mjs check
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
  } else {
    Write-Host '  (node 未安装，跳过 i18n-sync check)'
  }
} else {
  Invoke-GoTest @('./internal/service/...', './internal/harness/...', './internal/llm/...', './internal/tool/...')
}

if (-not $NoFront) { Invoke-FrontendChecks }
Write-Host '全部通过' -ForegroundColor Green
