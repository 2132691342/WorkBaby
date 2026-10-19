<#
失败摘要：把上一条命令的输出里「哪条链坏了、错在哪一行」摘进 job summary。
不进 artifact 的是测试结果——下载一次才知道原因，summary 当场就能看到。

用法（在 CI 里紧跟失败的那一步之后）：
  pwsh scripts/report-failures.ps1            # 读 .check/test-output.log
  pwsh scripts/report-failures.ps1 某.log
#>
[CmdletBinding()]
param([string]$Path = '.check/test-output.log')

Set-Location (Split-Path -Parent $PSScriptRoot)

if (-not (Test-Path $Path)) {
    Write-Host "没有 $Path，跳过失败摘要"
    exit 0
}

$log = Get-Content $Path -ErrorAction SilentlyContinue

# 优先抓 go test 的失败标记；抓不到就退回末尾 40 行，至少能看到最后一步的状态。
# 子测试名一并抓上（--- FAIL: TestXChain/某条链），它是定位的第一个把手。
$marks = $log | Select-String -Pattern '^--- FAIL|^    --- FAIL|^\s+--- FAIL|^FAIL\s|^panic:|\.go:\d+:' -Context 0, 4
$body = if ($marks) { $marks | Out-String } else { ($log | Select-Object -Last 40) -join "`n" }

if (-not $body.Trim()) {
    Write-Host "没有可摘的内容（$Path 是空的）"
    exit 0
}

if ($env:GITHUB_STEP_SUMMARY) {
    # 围栏用拼接而不是直接写在字符串里：反引号在 PowerShell 里是转义符，
    # 连续的奇数个会在收尾处把引号吃掉，字符串就再也闭合不了。
    $fence = ([string][char]96) * 3
    $md = "## 验证失败摘要`n`n${fence}text`n$body`n$fence"
    Add-Content -Path $env:GITHUB_STEP_SUMMARY -Value $md -Encoding utf8
    Write-Host '失败摘要已写入 job summary'
} else {
    Write-Host $body
}