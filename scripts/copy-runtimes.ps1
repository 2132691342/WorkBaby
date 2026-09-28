# 构建后把内置 Python 运行时压缩包拷到产物目录旁边，保证绿色包开箱即用。
# 代码侧 internal/runtime.ArchivePath 找的是 exe 同级 runtimes/python-<版本>-win-x64.tar.gz。
param(
    [string]$Bin
)

$ErrorActionPreference = 'Stop'

if ([string]::IsNullOrWhiteSpace($Bin)) {
    Write-Host "copy-runtimes: 未提供产物路径，跳过"
    exit 0
}

$root = Split-Path -Parent $PSScriptRoot
$src = Join-Path $root 'runtimes'
$archives = Get-ChildItem -Path $src -Filter 'python-*.tar.gz' -File -ErrorAction SilentlyContinue
if (-not $archives) {
    Write-Host "copy-runtimes: $src 下没有 python-*.tar.gz，跳过（运行时未下载）"
    exit 0
}

$destRoot = Split-Path -Parent $Bin
$dest = Join-Path $destRoot 'runtimes'
New-Item -ItemType Directory -Force -Path $dest | Out-Null
Copy-Item -Path (Join-Path $src 'python-*.tar.gz') -Destination $dest -Force
if (Test-Path (Join-Path $src 'manifest.json')) {
    Copy-Item -Path (Join-Path $src 'manifest.json') -Destination $dest -Force
}
Write-Host "copy-runtimes: 已拷贝 Python 运行时到 $dest"
