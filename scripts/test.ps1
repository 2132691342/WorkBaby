# 测试入口。日常改动用 -Fast：跳过真实解压并复用构建缓存，秒级返回；
# 提交前与 CI 跑默认全量（-count=1 强制真跑，不吃缓存），保证结果可信。
param(
    [switch]$Fast,   # -short + 允许缓存：跳过 runtime 的真实解压
    [string]$Pkg,    # 只跑一个包，如 backend/service
    [switch]$Race    # 竞态检测，慢，改并发相关代码时用
)

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
    $goArgs = @('test')
    $goArgs += if ($Pkg) { "./$Pkg" } else { './...' }
    $goArgs += if ($Fast) { '-short' } else { '-count=1' }
    if ($Race) { $goArgs += '-race' }

    Write-Host "go $($goArgs -join ' ')" -ForegroundColor DarkGray
    & go @goArgs
    if ($LASTEXITCODE -ne 0) { throw 'tests failed' }
} finally {
    Pop-Location
}
