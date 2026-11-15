# 开发态起停脚本：拉起后端 + Vite，并把端口与令牌交给前端。
#
# 用途：想在纯浏览器里调前端（不开 Wails 窗口）时用。
# 前端通过 localStorage 里的 wb.dev.port / wb.dev.token 找到后端，
# 所以这两个值必须与本脚本起的后端一致。
#
# 用法：
#   pwsh scripts/dev.ps1              # 起后端 + Vite，Ctrl+C 收尾
#   pwsh scripts/dev.ps1 -BackendOnly # 只起后端
param(
    [string]$Exe = (Join-Path $PSScriptRoot '..\bin\WorkBaby.exe'),
    [string]$Token = 'devtoken123',
    [int]$VitePort = 9245,
    [switch]$BackendOnly
)

$ErrorActionPreference = 'Continue'
$repoRoot = Split-Path $PSScriptRoot -Parent
Set-Location $repoRoot

# Stop-ProcessTree 连根带子一起收：npm 会把 node 与 esbuild 挂在 cmd 下面，
# 只杀最外层会留下占端口、锁文件的孤儿进程。
function Stop-ProcessTree([int]$RootId) {
    $children = @(Get-CimInstance Win32_Process -Filter "ParentProcessId=$RootId" -ErrorAction SilentlyContinue)
    foreach ($c in $children) { Stop-ProcessTree ([int]$c.ProcessId) }
    Stop-Process -Id $RootId -Force -ErrorAction SilentlyContinue
}

if (-not (Test-Path $Exe)) {
    Write-Host "找不到可执行文件：$Exe" -ForegroundColor Red
    Write-Host "先跑 task build。"
    exit 1
}

Stop-Process -Name WorkBaby -Force -ErrorAction SilentlyContinue
Start-Sleep -Milliseconds 400

$env:WB_DEV_TOKEN = $Token
$logDir = Join-Path $env:LOCALAPPDATA 'WorkBaby-dev\logs'
$logPath = Join-Path $logDir ('wb-' + (Get-Date).ToString('yyyy-MM-dd') + '.log')
$baseline = 0
if (Test-Path $logPath) { $baseline = (Get-Content $logPath).Count }

$backend = Start-Process -FilePath $Exe -ArgumentList '--hidden' -PassThru -WindowStyle Hidden

$port = 0
for ($i = 0; $i -lt 40; $i++) {
    Start-Sleep -Seconds 1
    if (-not (Test-Path $logPath)) { continue }
    $lines = @(Get-Content $logPath)
    if ($lines.Count -le $baseline) { continue }
    $chunk = $lines[$baseline..($lines.Count - 1)] -join "`n"
    if ($chunk -match 'HTTP 服务已监听 127\.0\.0\.1:(\d{4,5})') { $port = [int]$Matches[1]; break }
}

if (-not $port) {
    Write-Host "后端没起来。日志尾部：" -ForegroundColor Red
    if (Test-Path $logPath) { Get-Content $logPath -Tail 30 }
    Stop-Process -Id $backend.Id -Force -ErrorAction SilentlyContinue
    exit 1
}

Write-Host "后端端口：$port" -ForegroundColor Green
Write-Host "令牌：$Token"
Write-Host ""
Write-Host "在浏览器控制台写入这两行，然后用 Vite 页面调前端："
Write-Host "  localStorage.setItem('wb.dev.port', '$port')"
Write-Host "  localStorage.setItem('wb.dev.token', '$Token')"

$frontend = $null
if (-not $BackendOnly) {
    $frontend = Start-Process -FilePath 'cmd' `
        -ArgumentList '/c', "npm run dev -- --port $VitePort --strictPort" `
        -WorkingDirectory (Join-Path $repoRoot 'frontend') -PassThru -WindowStyle Hidden
    for ($i = 0; $i -lt 45; $i++) {
        Start-Sleep -Seconds 1
        try {
            Invoke-WebRequest -Uri "http://127.0.0.1:$VitePort/" -TimeoutSec 5 -UseBasicParsing | Out-Null
            Write-Host "前端已就绪：http://127.0.0.1:$VitePort/" -ForegroundColor Green
            break
        } catch {}
    }
}

Write-Host ""
Write-Host "Ctrl+C 收尾。"
try {
    while ($true) { Start-Sleep -Seconds 5 }
} finally {
    # 前端是通过 cmd 包起来的 npm→node→esbuild 三层：只杀 cmd 会把 node 留在后台，
    # 端口占着不释放、日志文件被锁住删不掉。整棵树一起收。
    if ($frontend) { Stop-ProcessTree $frontend.Id }
    Stop-ProcessTree $backend.Id
}
