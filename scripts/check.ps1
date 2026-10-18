<#
验证入口：一个脚本收口全部验证动作。三档按「改了多大」选，不要每次全跑。

  pwsh scripts/check.ps1                 快门：gofmt → 编译 → 全包短测试
  pwsh scripts/check.ps1 -Pkg backend/pi 只验一个包（只碰一个能力域时用，省的是编译）
  pwsh scripts/check.ps1 -Full           全量：+ vet + 依赖边界 + 全量测试 + 前端构建；提交前 / CI

短测试（-short）跳过真 git 子进程链路与归档真解压，这两条只在 -Full 跑到。

耗时大头是编译与链接测试二进制，不是断言：-Pkg 省的就是这一段。
#>
[CmdletBinding()]
param(
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
    $bad = @(gofmt -l . 2>&1 | Where-Object { $_ -and $_ -notmatch '^frontend' })
    foreach ($f in $bad) { Write-Host "      需要格式化：$f" -ForegroundColor Yellow }
    $global:LASTEXITCODE = [int][bool]$bad
}

# -Pkg 档：只回答「这个包坏没坏」，不碰边界与前端。
# 包路径写成 backend/pi 或 ./backend/pi 都行；go 只认后者，这里补上前缀。
if ($Pkg) {
    if ($Pkg -notmatch '^(\.|WorkBaby)') { $Pkg = "./$Pkg" }
    Step "build $Pkg" { go build $Pkg }
    Step "test $Pkg" {
        New-Item -ItemType Directory -Force -Path '.check' | Out-Null
        if ($Full) { go test -count=1 -v $Pkg 2>&1 | Tee-Object $TestLog } else { go test -short -count=1 $Pkg 2>&1 | Tee-Object $TestLog }
        $global:LASTEXITCODE = $LASTEXITCODE
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

# 测试输出落盘：本地回看与 CI 失败摘要共用同一份（scripts/report-failures.ps1 读它）。
$TestLog = '.check/test-output.log'
Step 'test' {
    New-Item -ItemType Directory -Force -Path '.check' | Out-Null
    if ($Full) { go test -count=1 -v ./... 2>&1 | Tee-Object $TestLog } else { go test -short -count=1 ./... 2>&1 | Tee-Object $TestLog }
    # 管道会吃掉 go test 的退出码，这里取 $LASTEXITCODE 手动判。
    $global:LASTEXITCODE = $LASTEXITCODE
}

# 前端只有一档：npm run build 里已串了 vue-tsc，不必再单独跑类型检查。
if ($Full) {
    Step 'frontend build' {
        Push-Location frontend
        try { npm run build } finally { Pop-Location }
    }
}

Write-Host '全部通过。' -ForegroundColor Green