# 测试入口。三个开关决定「跑什么」：范围（-Pkg / -Run）、模式（-Fast / -Race）。
# 日常改动用 -Fast -Run <名字>：只碰一个 Test，构建命中缓存，秒级返回。
param(
    [string]$Pkg,     # 只跑一个包，如 backend/service
    [string]$Run,     # 只跑匹配的 Test，支持正则，如 TestChatRunChain
    [switch]$Fast,    # -short + 允许缓存：跳过 runtime 的真实解压（全仓唯一的慢点）
    [switch]$Race     # 竞态检测，慢，改并发相关代码时用；需要 CGO 与 gcc
)

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
    $goArgs = @('test')
    $goArgs += if ($Pkg) { "./$Pkg" } else { './...' }
    $goArgs += if ($Fast) { '-short' } else { '-count=1' }
    if ($Run) { $goArgs += @('-run', $Run) }
    if ($Race) { $goArgs += '-race' }

    Write-Host "go $($goArgs -join ' ')" -ForegroundColor DarkGray
    & go @goArgs
    if ($LASTEXITCODE -ne 0) { throw '测试未通过' }
} finally {
    Pop-Location
}
