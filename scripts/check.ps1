<#
验证入口：一个脚本收口全部验证动作，按「改了多大」选档，不要每次全跑。

  pwsh scripts/check.ps1                     快门：gofmt → 编译 → 全包 -short 测试
  pwsh scripts/check.ps1 -Pkg backend/pi     只验一个包（gofmt → 编译 → 短测试）
  pwsh scripts/check.ps1 -Chain 提问          只跑命中名字的链路（跨包，grep 式定位）
  pwsh scripts/check.ps1 -List               列出全部测试入口与所在文件
  pwsh scripts/check.ps1 -Full               全量：+ 前端构建 + vet + 依赖边界 + 全量测试

分档规则：
  -Pkg 只回答「这个包坏没坏」，因此不跑 vet / 边界；它与 -Full、-Chain 互斥。
  -short 跳过真 git 子进程链路、运行时归档真解压与本机 PI 真实回合，
  这三条只在 -Full 跑到；CI 跑 -Full，所以本地不跑不等于没人跑。
  耗时大头是编译与链接测试二进制，不是断言——-Pkg / -Chain 省下的正是这一段。
  任何编译前都保证 frontend/dist 就位：它是 //go:embed 的硬依赖且被 .gitignore
  排除，全新 checkout（CI）上没有。-Full 每次重建，其余档 dist 已在则跳过。

失败摘要：
  -Full 档把测试输出同时写到 .check/test-output.log（不进 Git）。失败时自动打印
  「哪条链坏了、错在哪一行」：先抓 --- FAIL / panic / 文件行号，抓不到再退回末尾若干行。
  在 CI 里同一份内容还会追加到 job summary，不必下载 artifact 才知道原因。
#>
[CmdletBinding()]
param(
    [switch]$Full,
    [string]$Pkg,
    [string]$Chain,
    [switch]$List
)

$ErrorActionPreference = 'Stop'
Set-Location (Split-Path -Parent $PSScriptRoot)

if ($List) {
    # 查找靠命名不靠搜代码：入口函数一律叫 Test<Domain>Chain，这里就是唯一索引。
    Get-ChildItem -Recurse -Include *_test.go -Path . |
        Where-Object { $_.FullName -notmatch '\\node_modules\\' } |
        ForEach-Object {
            $file = $_.FullName.Replace((Get-Location).Path + '\', '')
            Select-String -Path $_.FullName -Pattern '^func (Test\w+Chain)\(' |
                ForEach-Object { '{0,-28} {1}' -f $_.Matches[0].Groups[1].Value, $file }
        }
    exit 0
}

if ($Pkg -and ($Full -or $Chain)) {
    throw '-Pkg 只验一个包，不能与 -Full / -Chain 同时用。'
}
if ($Full -and $Chain) {
    throw '-Chain 是定点定位，-Full 是全量门禁，二选一。'
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
        Write-FailureSummary
        exit $LASTEXITCODE
    }
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
        # 围栏用拼接而不是直接写在字符串里：反引号在 PowerShell 里是转义符。
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

# 前端必须在编译前就位：main.go 的 //go:embed all:frontend/dist 在 dist 不存在时直接编译失败，
# 而 dist 被 .gitignore 排除，全新 checkout（CI）上没有。
# -Full 每次重建（门禁验的是「最新前端 + Go」的真实组合）；其余档 dist 已在则跳过，保住快。
# npm run build 里已串了 vue-tsc，不必再单独跑类型检查。
if ($Full -or -not (Test-Path 'frontend/dist/index.html')) {
    if (-not (Test-Path 'frontend/node_modules')) {
        Invoke-Step 'frontend deps' {
            Push-Location frontend
            try { npm ci } finally { Pop-Location }
        }
    }
    Invoke-Step 'frontend build' {
        Push-Location frontend
        try { npm run build } finally { Pop-Location }
    }
}

# -Pkg 档：只回答「这个包坏没坏」。包路径写成 backend/pi 或 ./backend/pi 都行，这里补前缀。
if ($Pkg) {
    if ($Pkg -notmatch '^(\.|WorkBaby)') { $Pkg = "./$Pkg" }
    Invoke-Step "build $Pkg" { go build $Pkg }
    Invoke-Step "test $Pkg" { go test -short -count=1 $Pkg }
    Write-Host '全部通过。' -ForegroundColor Green
    exit 0
}

# 编译比测试更早暴露类型错误，放在测试前。
Invoke-Step 'build' { go build ./... }

if ($Full) {
    Invoke-Step 'vet' { go vet ./... }
    Invoke-Step 'boundaries' { go run ./tools/check-boundaries }
}

if ($Chain) {
    # 定点定位：-run 接受 RE2 子串，子测试名本身就写成一句话，直接拿它当把手。
    Invoke-Step "test -run $Chain" {
        go test -short -count=1 -v -run $Chain ./...
    }
    Write-Host '全部通过。' -ForegroundColor Green
    exit 0
}

# 全量档跑全量测试并落盘（失败摘要靠它）；快门档只求快，不留日志。
if ($Full) {
    New-Item -ItemType Directory -Force -Path '.check' | Out-Null
    Invoke-Step 'test' {
        go test -count=1 -v ./... 2>&1 | Tee-Object $TestLog
        $global:LASTEXITCODE = $LASTEXITCODE
    }
} else {
    Invoke-Step 'test' { go test -short -count=1 ./... }
}

Write-Host '全部通过。' -ForegroundColor Green
