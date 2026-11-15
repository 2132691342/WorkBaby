# 后端进程助手：起一个纯后端实例（--hidden，不建窗口）并等到它报出监听端口。
#
# 被 dev.ps1 与 smoke.ps1 点源引用，两个脚本都只需要「端口 + 进程」这一件事。
# 端口是随机的，日志是唯一可靠的出口，因此端口只从「本次启动之后新增的日志行」里取。
#
# 用法：. "$PSScriptRoot\backend.ps1"; $backend = Start-WorkBabyBackend -Exe $Exe -Token $Token

# Stop-ProcessTree 连根带子一起收：npm 会把 node 与 esbuild 挂在 cmd 下面，
# 只杀最外层会留下占端口、锁文件的孤儿进程。
function Stop-ProcessTree([int]$RootId) {
    $children = @(Get-CimInstance Win32_Process -Filter "ParentProcessId=$RootId" -ErrorAction SilentlyContinue)
    foreach ($c in $children) { Stop-ProcessTree ([int]$c.ProcessId) }
    Stop-Process -Id $RootId -Force -ErrorAction SilentlyContinue
}

# Start-WorkBabyBackend 起后端并回 @{ Process; Port }。等不到端口就直接失败退出。
function Start-WorkBabyBackend {
    param(
        [Parameter(Mandatory)][string]$Exe,
        [Parameter(Mandatory)][string]$Token,
        [int]$TimeoutSec = 40
    )

    if (-not (Test-Path $Exe)) {
        Write-Host "找不到可执行文件：$Exe" -ForegroundColor Red
        Write-Host "先跑 task build。"
        exit 1
    }

    # 令牌用环境变量传给后端：后端启动时校验它，界面侧每次请求都带上。
    $env:WB_DEV_TOKEN = $Token
    $logPath = Join-Path $env:LOCALAPPDATA ('WorkBaby-dev\logs\wb-' + (Get-Date).ToString('yyyy-MM-dd') + '.log')

    # 基线取「启动前已有多少行」：之前跑过的实例也写在这同一个按天日志里，老的端口行不算数。
    $baseline = 0
    if (Test-Path $logPath) { $baseline = @(Get-Content $logPath).Count }

    $proc = Start-Process -FilePath $Exe -ArgumentList '--hidden' -PassThru -WindowStyle Hidden
    for ($i = 0; $i -lt $TimeoutSec; $i++) {
        Start-Sleep -Seconds 1
        if (-not (Test-Path $logPath)) { continue }
        $lines = @(Get-Content $logPath)
        if ($lines.Count -le $baseline) { continue }
        $chunk = $lines[$baseline..($lines.Count - 1)] -join "`n"
        if ($chunk -match 'HTTP 服务已监听 127\.0\.0\.1:(\d{4,5})') {
            return @{ Process = $proc; Port = [int]$Matches[1] }
        }
    }

    Write-Host "没等到端口（$TimeoutSec 秒）。日志尾部：" -ForegroundColor Red
    if (Test-Path $logPath) { Get-Content $logPath -Tail 30 }
    Stop-ProcessTree $proc.Id
    exit 1
}

# Stop-WorkBabyBackend 收掉 Start-WorkBabyBackend 起的实例。
function Stop-WorkBabyBackend($Backend) {
    if ($Backend) { Stop-ProcessTree $Backend.Process.Id }
}
