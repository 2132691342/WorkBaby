# 全仓唯一验证入口：后端测试 + 依赖方向门禁。
# 日常：-Fast（跳过运行时归档解压，命中缓存秒级返回）
# 定点：-Fast -Pkg backend/service 或 -Fast -Run TestServiceRunChain
# 提交前：不给开关，跑全量（含真实解压与门禁）
param(
    [string]$Pkg,     # 只跑一个包，如 backend/service
    [string]$Run,     # 只跑匹配的 Test，支持正则，如 TestServiceRunChain
    [switch]$Fast,    # -short + 允许缓存：跳过 runtime 的真实解压（全仓唯一的慢点）
    [switch]$Race     # 竞态检测，慢，改并发相关代码时用；需要 CGO 与 gcc
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

    # 门禁只在整仓验证时跑：定点调试一个包 / 一个 Test 时它没有新信息。
    if (-not ($Pkg -or $Run)) {
        Write-Host 'go run ./tools/check-boundaries' -ForegroundColor DarkGray
        & go run ./tools/check-boundaries
        if ($LASTEXITCODE -ne 0) { throw '依赖方向门禁未通过' }
    }

    Write-Host ("elapsed {0:N1}s" -f $watch.Elapsed.TotalSeconds) -ForegroundColor DarkGray
} finally {
    Pop-Location
}
