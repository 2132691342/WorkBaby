# 依赖方向门禁入口：把 AGENTS.md §2.1 / §2.2 的约束表变成可执行检查。
#
# 实现在 tools/check-boundaries（Go 程序）：用 go list -json 读编译器视角的真实
# import 关系，而不是对源码做文本 grep——注释、字符串字面量、构建标签分支都会
# 让 grep 得出错误结论。检查逻辑放 Go 是为了绕开 PowerShell 5.1 的两个坑：
# 无 BOM 的 UTF-8 会被按 ANSI 解码，以及多条 JSON 记录首尾相接时无法可靠切分。
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
