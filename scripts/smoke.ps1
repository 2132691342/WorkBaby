# 冒烟脚本：起一个纯后端实例（不建窗），把全部只读接口打一遍，报告非 0 业务码。
#
# 用途：改完路由或 service 后，确认「接口还在、还回 200、还带 data」。
# 它不测业务正确性——那由 tests/ 负责；它只测接线。
#
# 用法：
#   task build                      # 先产出 bin\WorkBaby.exe
#   pwsh scripts/smoke.ps1          # 默认用 bin\WorkBaby.exe
#   pwsh scripts/smoke.ps1 -Exe D:\path\to\WorkBaby.exe -Token abc123
param(
    [string]$Exe = (Join-Path $PSScriptRoot '..\bin\WorkBaby.exe'),
    [string]$Token = 'smoke-token',
    [int]$TimeoutSec = 40
)

$ErrorActionPreference = 'Continue'

if (-not (Test-Path $Exe)) {
    Write-Host "找不到可执行文件：$Exe" -ForegroundColor Red
    Write-Host "先跑 task build。"
    exit 1
}

# 只读接口清单：这些接口不该改磁盘状态，全扫一遍最省事。
$ReadOnly = @(
    '/bootstrap', '/settings', '/models', '/providers', '/skills', '/mcp', '/sessions',
    '/engines', '/prefs', '/workspaces', '/web/status', '/index/status', '/templates',
    '/apps', '/health/engines', '/models/defaults', '/chat/state', '/logs'
)

# 令牌用环境变量传给后端；端口从日志里读出来（随机端口，没有别的办法）。
$env:WB_DEV_TOKEN = $Token
$logDir = Join-Path $env:LOCALAPPDATA 'WorkBaby-dev\logs'
$logPath = Join-Path $logDir ('wb-' + (Get-Date).ToString('yyyy-MM-dd') + '.log')

# 基线取「启动前已有多少行」，端口要从基线之后的新行里找：
# 之前跑过的实例也在同一个按天日志里，老的端口行不算数。
$baseline = 0
if (Test-Path $logPath) { $baseline = (Get-Content $logPath).Count }

$proc = Start-Process -FilePath $Exe -ArgumentList '--hidden' -PassThru -WindowStyle Hidden

$port = 0
for ($i = 0; $i -lt $TimeoutSec; $i++) {
    Start-Sleep -Seconds 1
    if (-not (Test-Path $logPath)) { continue }
    $lines = @(Get-Content $logPath)
    if ($lines.Count -le $baseline) { continue }
    $chunk = $lines[$baseline..($lines.Count - 1)] -join "`n"
    if ($chunk -match 'HTTP 服务已监听 127\.0\.0\.1:(\d{4,5})') { $port = [int]$Matches[1]; break }
}

if (-not $port) {
    Write-Host "没等到端口（$TimeoutSec 秒）。" -ForegroundColor Red
    if (Test-Path $logPath) { Get-Content $logPath -Tail 30 }
    Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
    exit 1
}
Write-Host "后端已就绪：127.0.0.1:$port"

$headers = @{ 'X-Desktop-Token' = $Token }
$failed = 0

foreach ($path in $ReadOnly) {
    try {
        $r = Invoke-WebRequest -Uri ("http://127.0.0.1:$port/api/v1" + $path) -Headers $headers -TimeoutSec 30 -UseBasicParsing
        $body = $r.Content | ConvertFrom-Json
        if ($r.StatusCode -ne 200) {
            Write-Host ("{0,-22} HTTP {1}" -f $path, $r.StatusCode) -ForegroundColor Red
            $failed++
            continue
        }
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

# 无令牌必须回 401：那是唯一的门禁。
try {
    Invoke-WebRequest -Uri "http://127.0.0.1:$port/api/v1/settings" -TimeoutSec 10 -UseBasicParsing | Out-Null
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

Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue

if ($failed -gt 0) {
    Write-Host "`n$failed 项不通。" -ForegroundColor Red
    exit 1
}
Write-Host "`n全部接口通。" -ForegroundColor Green
