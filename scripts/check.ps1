<#
验证入口：一个脚本收口全部验证动作。按「改了多大」选档，不要每次全跑。

  pwsh scripts/check.ps1                  快门：格式 → 编译 → 各包短测试
  pwsh scripts/check.ps1 -Pkg backend/pi  只验一个包
  pwsh scripts/check.ps1 -Chain 提问       只跑名字命中的子测试（只编可能命中的包）
  pwsh scripts/check.ps1 -List            列出全部测试入口与所在文件
  pwsh scripts/check.ps1 -Full            全量门禁：+ 前端构建 + vet + 依赖边界 + 全量测试
  加 -Detail 把每个子测试都打出来（默认安静：几百行刷屏对定位没有帮助）

档位怎么选：
  -Pkg 只回答「这个包坏没坏」，不跑 vet / 边界；它与 -Full / -Chain 互斥。
  -short 跳过三条慢链路（真 git 子进程、运行时归档真解压、本机真 PI 回合），
  它们只在 -Full 跑到；CI 跑 -Full，所以本地不跑不等于没人跑。
  耗时大头是编译，不是断言——-Pkg / -Chain 省下的正是编译那一段。

前端必须先于 Go 编译就位：main.go 的 //go:embed all:frontend/dist 在 dist 不存在时
直接编译失败，而 dist 被 .gitignore 排除，全新 checkout（CI）上没有。
-Full 每次重建（门禁验的是「最新前端 + Go」的真实组合）；其余档 dist 已在则跳过。
#>
[CmdletBinding()]
param(
    [switch]$Full,
    [string]$Pkg,
    [string]$Chain,
    [switch]$List,
    [switch]$Detail
)

$ErrorActionPreference = 'Stop'
Set-Location (Split-Path -Parent $PSScriptRoot)

# 有测试文件的包。写死在这里而不是用 go list 现算：go list 要加载整个模块图，
# 单那一步就 3 秒，比整轮短测试还贵。清单漂了由 -Full 档的校验拦住（见 AssertTestPackages）。
# 有测试文件的包。写死在这里而不是用 go list 现算：go list 要加载整个模块图，
# 单那一步就 3 秒，比整轮短测试还贵。清单漂了由 -Full 档的校验拦住（见 AssertTestPackages）。
$TestPackages = @(
    '.'
    './backend/api'
    './backend/git'
    './backend/pi'
    './backend/pi/env'
    './backend/runtime'
    './backend/service'
    './backend/skill'
)

$TestLog = '.check/test-output.log'

# Invoke-Step 跑一步并当场判退出码：失败即退出，且不吞退出码。
# 不靠 $LASTEXITCODE 往外传——管道与脚本块都会让它变得不可靠。
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

# Write-FailureSummary 从测试日志里摘出可定位的片段：子测试名是定位的第一个把手，
# .go:行号 直接指到断言。CI 里同一份内容追加进 job summary，不必下载 artifact。
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
        Add-Content -Path $env:GITHUB_STEP_SUMMARY -Value "## 验证失败摘要`n`n${fence}text`n$body`n$fence" -Encoding utf8
    }
}

# AssertTestPackages 校验上面的清单与磁盘一致：清单漏了一个包，它的测试就永远不会被跑，
# 而「全部通过」的输出看不出这条链根本没跑。只在 -Full 付这次 go list 的 3 秒。
function AssertTestPackages {
    $actual = @(go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./... 2>$null |
        Where-Object { $_ -and $_.Trim() -ne '' } |
        ForEach-Object {
            $rel = $_ -replace '^WorkBaby/?', ''
            if ($rel -eq '') { '.' } else { "./$rel" }
        } | Sort-Object -Unique)
    $listed = @($TestPackages | Sort-Object -Unique)
    $missing = @($actual | Where-Object { $_ -notin $listed })
    $stale = @($listed | Where-Object { $_ -notin $actual })
    if ($missing.Count -gt 0) {
        throw "有测试的包没进清单（它们的测试不会被跑）：$($missing -join ', ')"
    }
    if ($stale.Count -gt 0) {
        throw "清单里的包已经没有测试文件了（该删）：$($stale -join ', ')"
    }
}

if ($List) {
    # 查找靠命名不靠搜代码：入口一律叫 Test<Domain>Chain，子测试名就是一句「坏的现象」。
    Get-ChildItem -Recurse -Include *_test.go -Path . |
        Where-Object { $_.FullName -notmatch '\\node_modules\\' } |
        ForEach-Object {
            $file = $_.FullName.Replace((Get-Location).Path + '\', '')
            $lines = Get-Content $_.FullName
            $entries = @($lines | Select-String -Pattern '^func (Test\w+Chain)\(')
            if (-not $entries.Count) { return }
            Write-Host $file -ForegroundColor Cyan
            foreach ($e in $entries) {
                Write-Host ('  {0}' -f $e.Matches[0].Groups[1].Value) -ForegroundColor Green
                $lines | Select-String -Pattern 't\.Run\("([^"]+)"' |
                    ForEach-Object { '    - {0}' -f $_.Matches[0].Groups[1].Value } |
                    ForEach-Object { Write-Host $_ }
            }
        }
    exit 0
}

if ($Pkg -and ($Full -or $Chain)) {
    throw '-Pkg 只验一个包，不能与 -Full / -Chain 同时用。'
}
if ($Full -and $Chain) {
    throw '-Chain 是定点定位，-Full 是全量门禁，二选一。'
}

# gofmt 必须无输出：.gitattributes 把 *.go 钉成 LF，CRLF 文件会被一律报成未格式化。
Invoke-Step 'gofmt' {
    $bad = @(gofmt -l . 2>&1 | Where-Object { $_ -and $_ -notmatch '^frontend' })
    foreach ($f in $bad) { Write-Host "      需要格式化：$f" -ForegroundColor Yellow }
    $global:LASTEXITCODE = [int][bool]$bad
}

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

# -Pkg 档：只回答「这个包坏没坏」。backend/pi 与 ./backend/pi 两种写法都收。
if ($Pkg) {
    if ($Pkg -notmatch '^(\.|WorkBaby)') { $Pkg = "./$Pkg" }
    Invoke-Step "build $Pkg" { go build $Pkg }
    Invoke-Step "test $Pkg" { go test -short -count=1 $Pkg }
    Write-Host '全部通过。' -ForegroundColor Green
    exit 0
}

# 编译比测试更早暴露类型错误，且覆盖全部包（包括没有测试的那些）。
Invoke-Step 'build' { go build ./... }

if ($Full) {
    Invoke-Step 'vet' { go vet ./... }
    Invoke-Step 'boundaries' { go run ./tools/check-boundaries }
    AssertTestPackages
}

# -Chain 档：子测试名要带入口名前缀才能命中，所以给 -run 拼一个空前缀——
# -run 把参数按 '/' 分段，每段对应名字的一层，空段匹配任意入口名。
# 于是「提问」不必知道它挂在哪个 Chain 下也能直接跑；给了完整路径就原样用。
if ($Chain) {
    # 测试源码里 grep 关键字，只编可能命中的包：编全部测试二进制要 6 秒，grep 是毫秒级。
    # 没命中就退回全量——宁可多跑，不能漏跑。
    $pattern = if ($Chain.Contains('/')) { $Chain } else { "/$Chain" }
    # 关键字按中文标点与空白切段：链名里的「：」「、」不该让 grep 落空。
    $keys = @($Chain -split '[\s：:、,，]+' | Where-Object { $_ -and $_.Length -ge 2 } | Sort-Object -Unique)
    $testFiles = @(Get-ChildItem -Recurse -Include *_test.go -Path . |
        Where-Object { $_.FullName -notmatch '\node_modules\\' })
    $hit = @()
    foreach ($k in $keys) {
        $hit += @($testFiles | Where-Object { Select-String -Path $_.FullName -Pattern ([regex]::Escape($k)) -Quiet })
    }
    $hit = @($hit | Sort-Object -Unique)
    Write-Host "  命中文件: $($hit.Count) 个（关键字: $($keys -join ' / ')）" -ForegroundColor DarkGray
    $pkgs = @()
    $root = (Get-Location).Path
    foreach ($f in $hit) {
        $rel = $f.FullName.Substring($root.Length + 1)
        $dir = ($rel -replace '\\', '/') -replace '/[^/]+$', ''
        $p = if ($dir -eq '') { '.' } else { "./$dir" }
        if ($p -in $TestPackages) { $pkgs += $p }
    }
    $pkgs = @($pkgs | Sort-Object -Unique)
    if (-not $pkgs) {
        $pkgs = $TestPackages
        Write-Host '  关键字没命中任何测试文件，退回全量' -ForegroundColor DarkGray
    } else {
        Write-Host "  只跑: $($pkgs -join ' ')" -ForegroundColor DarkGray
    }
    Invoke-Step "test -run $Chain" {
        Write-Host "  跑的包: $($pkgs -join ' ')" -ForegroundColor DarkGray
        go test -short -count=1 -v -run $pattern @pkgs
    }
    Write-Host '全部通过。' -ForegroundColor Green
    exit 0
}

# 全量档把测试输出落盘（失败摘要靠它）；快门档只求快，不留日志。
$testArgs = if ($Full) { @('test', '-count=1') } else { @('test', '-short', '-count=1') }
if ($Detail) { $testArgs += '-v' }
$testArgs += $TestPackages

if ($Full) {
    New-Item -ItemType Directory -Force -Path '.check' | Out-Null
    Invoke-Step 'test' {
        & go @testArgs 2>&1 | Tee-Object $TestLog
        $global:LASTEXITCODE = $LASTEXITCODE
    }
} else {
    Invoke-Step 'test' {
        & go @testArgs
        $global:LASTEXITCODE = $LASTEXITCODE
    }
}

Write-Host '全部通过。' -ForegroundColor Green
