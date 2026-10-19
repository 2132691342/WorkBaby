<#
验证入口：一个脚本收口全部验证动作，按「改了多大」分档，不要每次全跑。

  pwsh scripts/check.ps1                      快门：gofmt → 编译 → 全包 -short 测试
  pwsh scripts/check.ps1 -Pkg backend/pi      只验一个包（只碰一个能力域时用）
  pwsh scripts/check.ps1 -Full                全量：+ vet + 依赖边界 + 全量测试 + 前端构建

分档规则：
  -Pkg 只回答「这个包坏没坏」，因此不跑 vet / 边界 / 前端；它与 -Full 互斥。
  -short 跳过真 git 子进程链路、运行时归档真解压与本机 PI 真实回合，
  这三条只在 -Full 跑到；CI 跑 -Full，所以本地不跑不等于没人跑。
  耗时大头是编译与链接测试二进制，不是断言——所以 -Pkg 省下的正是这一段。

失败摘要：
  -Full 档把测试输出同时写到 .check/test-output.log（不进 Git）。失败时自动打印
  「哪条链坏了、错在哪一行」：先抓 --- FAIL / panic / 文件行号，抓不到再退回末尾若干行。
  在 CI 里同一份内容还会追加到 job summary，不必下载 artifact 才知道原因。
#>
[CmdletBinding()]
param(
    [switch]$Full,
    [string]$Pkg
)

$ErrorActionPreference = 'Stop'
Set-Location (Split-Path -Parent $PSScriptRoot)

if ($Pkg -and $Full) {
    throw '-Pkg 与 -Full 互斥：-Pkg 只验一个包（gofmt + 编译 + 短测试），-Full 才跑 vet / 依赖边界 / 全量测试 / 前端构建。'
}

# 只有 -Full 需要失败摘要：快门档一失败当场就能看到输出。
$TestLog = '.check/test-output.log'

# Invoke-Step 跑一步并当场判退出码：失败即退出，且不吞退出码。
# 不用 $LASTEXITCODE 传递——管道与脚本块都会让它变得不可靠。
function Invoke-Step {
    param([string]$Name, [scriptblock]$Body)

    Write-Host "==> $Name" -ForegroundColor Cyan
    & $Body
    if ($LASTEXITCODE -ne 0) {
        Write-Host "    FAILED: $Name（退出码 $LASTEXITCODE）" -ForegroundColor Red
        # 摘要放在退出前：本地立刻看见是哪条链，CI 则落进 job summary。
        Write-FailureSummary
        exit $LASTEXITCODE
    }
}

# 跑测试并把输出同时送屏幕与日志。go test 的退出码必须手动取：管道会吃掉它。
function Invoke-Test {
    param([string]$Target, [switch]$Verbose, [switch]$ToFile)

    if ($ToFile) {
        New-Item -ItemType Directory -Force -Path '.check' | Out-Null
        $args = @('test', '-count=1')
        if ($Verbose) { $args += '-v' } else { $args += '-short' }
        $args += $Target
        go @args 2>&1 | Tee-Object $TestLog
        $global:LASTEXITCODE = $LASTEXITCODE
        return
    }
    $args = @('test', '-short', '-count=1', $Target)
    go @args
}

# Write-FailureSummary 从测试日志里摘出可定位的片段：子测试名是定位的第一个把手。
function Write-FailureSummary {
    if (-not (Test-Path $TestLog)) { return }
    $log = Get-Content $TestLog -ErrorAction SilentlyContinue
    if (-not $log) { return }

    $marks = $log | Select-String -Pattern '^--- FAIL|^\s+--- FAIL|^FAIL\s|^panic:|\.go:\d+:' -Context 0, 4
    $body = if ($marks) { ($marks | Out-String) } else { ($log | Select-Object -Last 40) -join "`n" }
    if (-not $body.Trim()) { return }

    Write-Host ''
    Write-Host '=== 失败摘要 ===' -ForegroundColor Red
    Write-Host $body
    if ($env:GITHUB_STEP_SUMMARY) {
        # 围栏用拼接而不是直接写在字符串里：反引号在 PowerShell 里是转义符，
        # 连续的奇数个会在收尾处把引号吃掉，字符串就再也闭合不了。
        $fence = ([string][char]96) * 3
        $md = "## 验证失败摘要`n`n${fence}text`n$body`n$fence"
        Add-Content -Path $env:GITHUB_STEP_SUMMARY -Value $md -Encoding utf8
    }
}

# gofmt 必须无输出：.gitattributes 把 *.go 钉成 LF，CRLF 文件会被一律报成未格式化。
Invoke-Step 'gofmt' {
    $bad = @(gofmt -l . 2>&1 | Where-Object { $_ -and $_ -notmatch '^frontend' })
    foreach ($f in $bad) { Write-Host "      需要格式化：$f" -ForegroundColor Yellow }
    $global:LASTEXITCODE = [int][bool]$bad
}

# -Pkg 档：只回答「这个包坏没坏」。包路径写成 backend/pi 或 ./backend/pi 都行，这里补前缀。
if ($Pkg) {
    if ($Pkg -notmatch '^(\.|WorkBaby)') { $Pkg = "./$Pkg" }
    Invoke-Step "build $Pkg" { go build $Pkg }
    Invoke-Step "test $Pkg" { Invoke-Test -Target $Pkg }
    Write-Host '全部通过。' -ForegroundColor Green
    exit 0
}

# 编译比测试更早暴露类型错误，放在测试前。
Invoke-Step 'build' { go build ./... }

if ($Full) {
    Invoke-Step 'vet' { go vet ./... }
    Invoke-Step 'boundaries' { go run ./tools/check-boundaries }
}

# 全量档跑全量测试并落盘（失败摘要靠它）；快门档只求快，不留日志。
Invoke-Step 'test' { Invoke-Test -Target './...' -ToFile:$Full -Verbose:$Full }

# 前端只有一档：npm run build 里已串了 vue-tsc，不必再单独跑类型检查。
if ($Full) {
    Invoke-Step 'frontend build' {
        Push-Location frontend
        try { npm run build } finally { Pop-Location }
    }
}

Write-Host '全部通过。' -ForegroundColor Green
