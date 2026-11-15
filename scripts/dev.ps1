# 开发态起停：拉起后端 + Vite，把端口与令牌交给浏览器里的前端。
#
# 用途：想在纯浏览器里调前端（不开 Wails 窗口）时用。
# 前端从 localStorage 读 wb.dev.port / wb.dev.token 找后端，所以这两个值必须与本脚本一致。
#
# 用法：
#   pwsh scripts/dev.ps1               # 起后端 + Vite，Ctrl+C 收尾
#   pwsh scripts/dev.ps1 -BackendOnly  # 只起后端
param(
    [string]$Exe = (Join-Path $PSScriptRoot '..\bin\WorkBaby.exe'),
    [string]$Token = 'devtoken123',
    [int]$VitePort = 9245,
    [switch]$BackendOnly
)

. (Join-Path $PSScriptRoot 'backend.ps1')

$ErrorActionPreference = 'Continue'
$repoRoot = Split-Path $PSScriptRoot -Parent
Set-Location $repoRoot

# 先把上一轮的实例收掉：老进程占着端口与文件锁，新实例起来了也没法验。
Stop-Process -Name WorkBaby -Force -ErrorAction SilentlyContinue
Start-Sleep -Milliseconds 400

$backend = Start-WorkBabyBackend -Exe $Exe -Token $Token
Write-Host "后端端口：$($backend.Port)" -ForegroundColor Green
Write-Host "令牌：$Token"
Write-Host ""
Write-Host "在浏览器控制台写入这两行，然后用 Vite 页面调前端："
Write-Host "  localStorage.setItem('wb.dev.port', '$($backend.Port)')"
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
    # 前端是 cmd 包起来的 npm→node→esbuild 三层：只杀 cmd 会把 node 留在后台，
    # 端口占着不释放、日志文件被锁住删不掉。整棵树一起收。
    if ($frontend) { Stop-ProcessTree $frontend.Id }
    Stop-WorkBabyBackend $backend
}
