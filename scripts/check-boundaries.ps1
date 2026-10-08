# 依赖方向门禁入口。检查逻辑在 tools/check-boundaries（Go 程序）：
# 用 go list -json 读编译器视角的真实 import 关系，绕开 PowerShell 5.1
# 的编码与多记录 JSON 切分问题。CI 与 release 都调本脚本。
param()

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
    go run ./tools/check-boundaries
    if ($LASTEXITCODE -ne 0) { throw "check-boundaries failed" }
} finally {
    Pop-Location
}
