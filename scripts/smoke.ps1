# 冒烟脚本：起一个纯后端实例（不建窗），把免参数的只读接口全打一遍，报告非 0 业务码。
#
# 用途：改完路由或 service 后，确认「装出来的 exe 还起得来、接口还在、还回 200 还带 data」。
# 它不测业务正确性——那由 tests/ 负责；它只测接线，且测的是发布产物这一侧的接线。
#
# 用法：
#   task build                                  # 先产出 bin\WorkBaby.exe
#   pwsh scripts/smoke.ps1                      # 默认用 bin\WorkBaby.exe
#   pwsh scripts/smoke.ps1 -Exe D:\path\to\WorkBaby.exe -Token abc123
param(
    [string]$Exe = (Join-Path $PSScriptRoot '..\bin\WorkBaby.exe'),
    [string]$Token = 'smoke-token',
    [int]$TimeoutSec = 40
)

. (Join-Path $PSScriptRoot 'backend.ps1')

$ErrorActionPreference = 'Continue'

# 只读接口清单：只收「不带参数就能回 200」的那批，带路径或 ID 的留给 tests/ 覆盖。
# 这些接口不该改磁盘状态，全扫一遍最省事——路由名拼错、service 没接上，这里就露出来。
$ReadOnly = @(
    '/bootstrap', '/settings', '/models', '/providers', '/skills', '/mcp', '/sessions',
    '/engines', '/prefs', '/workspaces', '/web/status', '/index/status', '/templates',
    '/apps', '/health/engines', '/models/defaults', '/chat/state', '/logs'
)

$backend = Start-WorkBabyBackend -Exe $Exe -Token $Token -TimeoutSec $TimeoutSec
Write-Host "后端已就绪：127.0.0.1:$($backend.Port)"
$base = "http://127.0.0.1:$($backend.Port)/api/v1"
$headers = @{ 'X-Desktop-Token' = $Token }
$failed = 0

foreach ($path in $ReadOnly) {
    try {
        $response = Invoke-WebRequest -Uri ($base + $path) -Headers $headers -TimeoutSec 30 -UseBasicParsing
        $body = $response.Content | ConvertFrom-Json
        if ($response.StatusCode -ne 200) {
            Write-Host ("{0,-22} HTTP {1}" -f $path, $response.StatusCode) -ForegroundColor Red
            $failed++
            continue
        }
        # 恒 200 是出口契约：业务失败写在 code 里，所以两个都得看。
        if ($body.code -ne '0') {
            Write-Host ("{0,-22} code={1} {2}" -f $path, $body.code, $body.message) -ForegroundColor Red
            $failed++
            continue
        }
        Write-Host ("{0,-22} OK" -f $path) -ForegroundColor DarkGray
    } catch {
        Write-Host ("{0,-22} ERR {1}" -f $path, $_.Exception.Message) -ForegroundColor Red
        $failed++
    }
}

# 无令牌必须回 401：那是唯一的门禁，回 200 就是门禁没装上。
try {
    Invoke-WebRequest -Uri ($base + '/settings') -TimeoutSec 10 -UseBasicParsing | Out-Null
    Write-Host "无令牌竟通过了门禁" -ForegroundColor Red
    $failed++
} catch {
    if ($_.Exception.Response.StatusCode.value__ -eq 401) {
        Write-Host "鉴权门禁            OK（401）" -ForegroundColor DarkGray
    } else {
        Write-Host "无令牌回了 $($_.Exception.Response.StatusCode.value__)，该是 401" -ForegroundColor Red
        $failed++
    }
}

Stop-WorkBabyBackend $backend

if ($failed -gt 0) {
    Write-Host "`n$failed 项不通。" -ForegroundColor Red
    exit 1
}
Write-Host "`n全部接口通。" -ForegroundColor Green
