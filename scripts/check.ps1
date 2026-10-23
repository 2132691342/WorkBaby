<#
验证入口：一个脚本收口全部验证动作，按改动面选档。

  pwsh scripts/check.ps1                    快门（约 10s）：格式 → 注释 → 编译 → 全部短测试
  pwsh scripts/check.ps1 -Pkg backend/pi/env 只验一个包（约 5s，不建前端）
  pwsh scripts/check.ps1 -Run 冲突           只跑名字命中的子测试（只编译含测试的包）
  pwsh scripts/check.ps1 -Full              门禁（约 60s）：+ 前端构建 + vet + 依赖边界 + 全量测试
  pwsh scripts/check.ps1 -List              列出测试入口与子测试，当索引用

短测试（-short）跳过三条慢链路：真 git 子进程、运行时归档真解压、本机真 PI 回合，只在 -Full 跑。
测试组织：一包一个入口 Test<Domain>Chain，子测试名就是一句「坏的现象」；定位靠 -List 与 -Run。

前端只在需要时构建：main 包的 //go:embed all:frontend/dist 是硬依赖，其余包都不依赖它。
#>
[CmdletBinding()]
param(
    [switch]$Full,
    [string]$Pkg,
    [string]$Run,
    [switch]$List
)

$ErrorActionPreference = 'Stop'
Set-Location (Split-Path -Parent $PSScriptRoot)
$TestLog = '.check/test-output.log'

# Invoke-Step 跑一步并当场判退出码：失败即退出，不吞状态。
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

# Write-FailureSummary 摘出失败片段（子测试名 + .go 行号），同时进 CI 的 job summary。
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
        # 围栏用拼接而不是写在字符串里：反引号在 PowerShell 里是转义符。
        $fence = ([string][char]96) * 3
        Add-Content -Path $env:GITHUB_STEP_SUMMARY -Value "## 验证失败摘要`n`n${fence}text`n$body`n$fence" -Encoding utf8
    }
}

# Test-PkgList 列出真正含测试文件的包目录（``./backend/pi/env`` 这类写法）。
function Test-PkgList {
    Get-ChildItem -Recurse -Include *_test.go -Path . |
        Where-Object { $_.FullName -notmatch '\\node_modules\\' } |
        ForEach-Object { './' + $_.DirectoryName.Replace((Get-Location).Path + '\', '').Replace('\', '/') } |
        Sort-Object -Unique
}

if ($List) {
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

# gofmt 必须无输出：.gitattributes 把 *.go 钉成 LF，CRLF 文件会被一律报成未格式化。
Invoke-Step 'gofmt' {
    $bad = @(gofmt -l . 2>&1 | Where-Object { $_ -and $_ -notmatch '^frontend' })
    foreach ($f in $bad) { Write-Host "      需要格式化：$f" -ForegroundColor Yellow }
    $global:LASTEXITCODE = [int][bool]$bad
}

# 注释规范：连续 3 行以上的 // 块一律报出来（AGENTS §2.6），把纸面规则变成门禁。
Invoke-Step 'comments' {
    $root = (Get-Location).Path
    $bad = @()
    Get-ChildItem -Recurse -Filter *.go -Path . |
        Where-Object { $_.FullName -notmatch '\\node_modules\\' } |
        ForEach-Object {
            $file = $_.FullName.Replace($root + '\', '')
            $lines = Get-Content $_.FullName -Encoding utf8
            $run = 0
            for ($i = 0; $i -lt $lines.Count; $i++) {
                # gofmt 会在 doc 注释与 //go: 指令之间补一行 //，它是格式要求，不算注释块。
                if ($lines[$i] -match '^\s*//\s*$' -and ($i + 1) -lt $lines.Count -and $lines[$i + 1] -match '^//go:') { continue }
                if ($lines[$i] -match '^\s*//') {
                    $run++
                    if ($run -eq 3) { $bad += ('{0}:{1}' -f $file, ($i + 1)) }
                } else { $run = 0 }
            }
        }
    foreach ($b in $bad) { Write-Host "      连续 3 行以上注释：$b" -ForegroundColor Yellow }
    $global:LASTEXITCODE = [int][bool]$bad
}

# 前端只在要编译 main 包时才建：dist 是 main.go 的 go:embed 硬依赖，其余包不依赖它。
function Build-Frontend {
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

# -Pkg：只回答「这个包坏没坏」。./backend/pi 与 backend/pi 两种写法都收，不碰前端。
if ($Pkg) {
    if ($Pkg -notmatch '^(\.|WorkBaby)') { $Pkg = "./$Pkg" }
    if ($Pkg -match '^(\./)?(WorkBaby)?$') { Build-Frontend }
    Invoke-Step "build $Pkg" { go build $Pkg }
    Invoke-Step "test $Pkg" { go test -short -count=1 $Pkg }
    Write-Host '全部通过。' -ForegroundColor Green
    exit 0
}

if ($Full) {
    Build-Frontend
    Invoke-Step 'build' { go build ./... }
    Invoke-Step 'vet' { go vet ./... }
    Invoke-Step 'boundaries' { go run ./tools/check-boundaries }
} elseif (-not (Test-Path 'frontend/dist/index.html')) {
    Build-Frontend
    Invoke-Step 'build' { go build ./... }
} else {
    Invoke-Step 'build' { go build ./... }
}

# -Run 只编译含测试的包：'/' 分段的空段匹配任意入口，于是「冲突」不必知道它挂在哪个 Chain 下。
$testArgs = @('test', '-count=1')
if (-not $Full) { $testArgs += '-short' }
if ($Run) {
    $testArgs += @('-v', '-run', "/$Run")
    $testArgs += @(Test-PkgList)
} else {
    $testArgs += './...'
}

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
