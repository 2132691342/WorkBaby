# 构建后把内置运行时压缩包拷到产物目录旁边，保证绿色包开箱即用。
# 代码侧 internal/runtime.ArchivePath 找的是 exe 同级 runtimes/ 下的归档：
# python-<版本>-win-x64.tar.gz 与 PowerShell-<版本>-win-x64.zip。
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
$archives = @()
$archives += Get-ChildItem -Path $src -Filter 'python-*.tar.gz' -File -ErrorAction SilentlyContinue
$archives += Get-ChildItem -Path $src -Filter 'PowerShell-*.zip' -File -ErrorAction SilentlyContinue
if (-not $archives) {
    Write-Host "copy-runtimes: $src 下没有运行时归档，跳过（运行时未下载）"
    exit 0
}

$destRoot = Split-Path -Parent $Bin
$dest = Join-Path $destRoot 'runtimes'
New-Item -ItemType Directory -Force -Path $dest | Out-Null
foreach ($a in $archives) {
    Copy-Item -Path $a.FullName -Destination $dest -Force
}
if (Test-Path (Join-Path $src 'manifest.json')) {
    Copy-Item -Path (Join-Path $src 'manifest.json') -Destination $dest -Force
}

# NSIS 安装器源目录（project.nsi 引用 build/windows/runtimes）
$nsisDest = Join-Path (Split-Path -Parent $destRoot) 'windows\runtimes'
New-Item -ItemType Directory -Force -Path $nsisDest | Out-Null
foreach ($a in $archives) {
    Copy-Item -Path $a.FullName -Destination $nsisDest -Force
}
if (Test-Path (Join-Path $src 'manifest.json')) {
    Copy-Item -Path (Join-Path $src 'manifest.json') -Destination $nsisDest -Force
}
Write-Host "copy-runtimes: 已拷贝 $($archives.Count) 个运行时归档到 $dest 与 $nsisDest"
