# 全仓唯一验证入口：后端测试 + 依赖方向门禁。
#   scripts/test.ps1                              全量（含运行时真解压与门禁，约 1 分钟）
#   scripts/test.ps1 -Fast                        日常：跳过解压，秒级返回
#   scripts/test.ps1 -Fast -Pkg backend/service   定点：只跑一个包
#   scripts/test.ps1 -Fast -Run TestLoopProtocol  定点：只跑一条链路
#   scripts/test.ps1 -Race                        改并发相关代码时加（需要 CGO 与 gcc，慢）
param(
    [string]$Pkg,
    [string]$Run,
    [switch]$Fast,
    [switch]$Race
)

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
$watch = [System.Diagnostics.Stopwatch]::StartNew()
try {
    $goArgs = @('test')
    $goArgs += if ($Pkg) { "./$Pkg" } else { './...' }
    $goArgs += if ($Fast) { '-short' } else { '-count=1' }
    if ($Run) { $goArgs += @('-run', $Run) }
    if ($Race) { $goArgs += '-race' }

    Write-Host "go $($goArgs -join ' ')" -ForegroundColor DarkGray
    & go @goArgs
    if ($LASTEXITCODE -ne 0) { throw '测试未通过' }

    # 门禁只在整仓验证时跑：定点调试一个包 / 一条链路时它没有新信息。
    if (-not ($Pkg -or $Run)) {
        Write-Host 'go run ./tools/check-boundaries' -ForegroundColor DarkGray
        & go run ./tools/check-boundaries
        if ($LASTEXITCODE -ne 0) { throw '依赖方向门禁未通过' }
    }

    Write-Host ("elapsed {0:N1}s" -f $watch.Elapsed.TotalSeconds) -ForegroundColor DarkGray
} finally {
    Pop-Location
}
