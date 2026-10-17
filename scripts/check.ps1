<#
验证入口：一个脚本收口全部验证动作（格式 / 编译 / 依赖边界 / 测试 / 前端）。
分档按「改了多大」选，不要每次全跑；失败即退出且不吞退出码。

  pwsh scripts/check.ps1            日常快门：格式 → 编译 → 边界 → 全包短测试（约 10s）
  pwsh scripts/check.ps1 -All       同上，测试换成全量，再加前端构建
  pwsh scripts/check.ps1 -Full      再加 vet 与前端类型检查（CI 跑这一档）
  pwsh scripts/check.ps1 -Pkg ./backend/pi   只跑这一个包
#>
[CmdletBinding()]
param(
    [switch]$All,
    [switch]$Full,
    [string]$Pkg
)

$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
Set-Location $repo

$failed = @()
function Step($name, $block) {
    Write-Host "==> $name" -ForegroundColor Cyan
    & $block
    if ($LASTEXITCODE -ne 0) {
        $script:failed += $name
        Write-Host "    FAILED: $name" -ForegroundColor Red
        exit $LASTEXITCODE
    }
}

# 1) 格式：gofmt 必须无输出。行尾由 .gitattributes 统一成 LF，否则 CRLF 文件一律被报未格式化。
Step 'gofmt' {
    $bad = gofmt -l . 2>&1 | Where-Object { $_ -and $_ -notmatch '^frontend' }
    if ($bad) {
        Write-Host "    需要格式化：" -ForegroundColor Yellow
        $bad | ForEach-Object { Write-Host "      $_" }
        $global:LASTEXITCODE = 1
    } else {
        $global:LASTEXITCODE = 0
    }
}

# 2) 编译：比测试更早暴露类型错误。
Step 'build' { go build ./... }

# 3) 依赖方向门禁。
Step 'boundaries' { go run ./tools/check-boundaries }

# 4) 测试：-Full 跑全量，其余跑 -short（跳过真 git 与归档解压）。
if ($Pkg) {
    Step "test $Pkg" {
        if ($Full) { go test -count=1 $Pkg } else { go test -short -count=1 $Pkg }
    }
} elseif ($All -or $Full) {
    Step 'test (all)' { go test -count=1 ./... }
} else {
    Step 'test (short)' { go test -short -count=1 ./... }
}

if ($Full) {
    Step 'vet' { go vet ./... }
}

# 5) 前端：-All 与 -Full 才跑（改前端或准备提交）。
if ($All -or $Full) {
    Step 'frontend build' {
        Push-Location frontend
        try { npm run build } finally { Pop-Location }
    }
}
if ($Full) {
    Step 'frontend typecheck' {
        Push-Location frontend
        try { npm run typecheck } finally { Pop-Location }
    }
}

Write-Host ""
Write-Host "全部通过。" -ForegroundColor Green
