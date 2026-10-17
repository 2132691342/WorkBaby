<#
验证入口：一个脚本收口全部验证动作（格式 / 编译 / 依赖边界 / 测试 / 前端）。
按「改了多大」选档，失败即退出且不吞退出码。

  pwsh scripts/check.ps1                 快门：格式 → 编译 → 全包短测试
  pwsh scripts/check.ps1 -Pkg ./backend/pi   只验一个包（改单个能力域时用）
  pwsh scripts/check.ps1 -All            快门 + 前端构建
  pwsh scripts/check.ps1 -Full           全部（vet + 依赖边界 + 全量测试 + 前端），CI 跑这一档

耗时大头是编译而不是断言：测试本体多在亚秒级，26 个包的编译才是分钟级。
所以「只跑一个包」省的是编译——这就是 -Pkg 存在的理由。
短测试（-short）跳过真 git 操作、归档真解压与真 PI 进程；这些各在对应包里留一次全量。
#>
[CmdletBinding()]
param(
    [switch]$All,
    [switch]$Full,
    [string]$Pkg
)

$ErrorActionPreference = 'Stop'
Set-Location (Split-Path -Parent $PSScriptRoot)

function Step($name, $block) {
    Write-Host "==> $name" -ForegroundColor Cyan
    & $block
    if ($LASTEXITCODE -ne 0) {
        Write-Host "    FAILED: $name" -ForegroundColor Red
        exit $LASTEXITCODE
    }
}

# gofmt 必须无输出：.gitattributes 把 *.go 钉成 LF，CRLF 文件会被一律报成未格式化。
Step 'gofmt' {
    $bad = gofmt -l . 2>&1 | Where-Object { $_ -and $_ -notmatch '^frontend' }
    if ($bad) {
        $bad | ForEach-Object { Write-Host "      需要格式化：$_" -ForegroundColor Yellow }
        $global:LASTEXITCODE = 1
    } else { $global:LASTEXITCODE = 0 }
}

if ($Pkg) {
    # 包路径写成 backend/pi 或 ./backend/pi 都行：go 只认后者，这里补上前缀。
    if ($Pkg -notmatch '^(\.|WorkBaby)') { $Pkg = "./$Pkg" }
    Step "build $Pkg" { go build $Pkg }
    Step "test $Pkg" {
        if ($Full) { go test -count=1 $Pkg } else { go test -short -count=1 $Pkg }
    }
    Write-Host '全部通过。' -ForegroundColor Green
    exit 0
}

# 编译比测试更早暴露类型错误，放在测试前。
Step 'build' { go build ./... }

if ($Full) {
    Step 'vet' { go vet ./... }
    Step 'boundaries' { go run ./tools/check-boundaries }
}

Step 'test' {
    if ($Full) { go test -count=1 ./... } else { go test -short -count=1 ./... }
}

# 前端只有构建一档：npm run build 里已经串了 vue-tsc，不必再单独跑一遍类型检查。
if ($All -or $Full) {
    Step 'frontend build' {
        Push-Location frontend
        try { npm run build } finally { Pop-Location }
    }
}

Write-Host '全部通过。' -ForegroundColor Green
