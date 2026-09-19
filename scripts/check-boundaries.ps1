# check-boundaries.ps1 — WorkBaby 架构门禁（CLAUDE.md §4）
#
# 检查项：
#   pkg-no-internal          叶子工具包 internal/pkg 不得依赖任何 internal 业务包
#   domain-no-upward         internal/domain 不得 import internal 上层包
#   repo-no-upward           internal/repo 不得 import service/api/server/能力域
#   service-no-http          internal/service 不得 import gin / wails
#   server-no-downward       internal/server 不得 import service/repo/能力域
#   core-no-upward           internal/core 只依赖 llm/tool/pkg/domain，不碰编排与传输层
#   core-no-http             internal/core 不得 import gin / wails
#   contract-version-sync    前端/后端契约版本常量必须一致
#
# 事件载荷 snake_case 契约由 Go 侧守卫：internal/service/integration_test.go
# （文本正则无法区分 map key 与结构体字段名，放测试更可读、更准）。
#
# 用法： powershell -ExecutionPolicy Bypass -File scripts/check-boundaries.ps1

$ErrorActionPreference = 'Stop'
# go list -json 输出为 UTF-8；中文 Windows 控制台默认按 GBK 解码子进程 stdout，
# 会把包注释里的多字节字符解码坏，导致 ConvertFrom-Json 解析失败。强制以 UTF-8 解码。
try { [Console]::OutputEncoding = [System.Text.Encoding]::UTF8 } catch {}
$scriptPath = (Resolve-Path -LiteralPath $PSCommandPath).Path
$root = Split-Path -Parent (Split-Path -Parent $scriptPath)
Set-Location $root

$failures = New-Object System.Collections.Generic.List[string]

function Assert-NoImports {
    param(
        [string]$Label,
        [string[]]$PkgPatterns,   # go list 包路径通配（如 'WorkBaby/internal/domain*'）
        [string[]]$Forbidden,     # 禁止 import 前缀（通配自动加 *）
        [string[]]$Allow = @()    # 即便命中 Forbidden 也豁免的确切路径（叶子工具包 internal/pkg）
    )
    $found = @()
    foreach ($p in $PkgPatterns) {
        $imports = $dep[$p]
        if ($null -eq $imports) { continue }   # 目录可能无 Go 文件
        foreach ($im in $imports) {
            if ($Allow -contains $im) { continue }
            foreach ($b in $Forbidden) {
                if ($im -eq $b -or $im -like "$b/*") {
                    $found += "$p -> $im"
                }
            }
        }
    }
    if ($found.Count -gt 0) {
        $failures.Add("FAIL $Label`n    " + ($found -join "`n    "))
    } else {
        Write-Host "PASS $Label"
    }
}

# ---- 1. 采集每个包的直接依赖（go list -json 权威，逐包避免模板引号被 shell 转义污染） ----
$pkgs = & go list ./internal/... 2>$null
if ($LASTEXITCODE -ne 0 -or $null -eq $pkgs) {
    Write-Host 'FATAL go list failed - 请确认在 WorkBaby 模块根目录执行'
    exit 1
}
$dep = @{}
foreach ($pkg in $pkgs) {
    $js = (& go list -json $pkg 2>$null) -join "`n"
    if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($js)) { continue }
    $o = $js | ConvertFrom-Json
    $dep[$o.ImportPath] = @($o.Imports)
}

# ---- 2. 边界断言 ----
$pkgAll = @($dep.Keys | Where-Object { $_ -like 'WorkBaby/internal/pkg*' })
$domainAll = @($dep.Keys | Where-Object { $_ -like 'WorkBaby/internal/domain*' })
$repoAll = @($dep.Keys | Where-Object { $_ -like 'WorkBaby/internal/repo*' })
$apiAll = @($dep.Keys | Where-Object { $_ -like 'WorkBaby/internal/api*' })
$serviceAll = @($dep.Keys | Where-Object { $_ -like 'WorkBaby/internal/service*' })
$serverAll = @($dep.Keys | Where-Object { $_ -like 'WorkBaby/internal/server*' })
$coreAll = @($dep.Keys | Where-Object { $_ -like 'WorkBaby/internal/core*' })

# repo / domain 禁止依赖的 internal 上层包（repo 仅允许 domain/db/repo/pkg）。
# api-no-repo：组合根已下沉 internal/bootstrap（App 持有 repo），api 层经 App 取用——
# 业务数据访问必须经 service，api 不得直接 import repo。
$capDomain = @('WorkBaby/internal/api','WorkBaby/internal/service','WorkBaby/internal/server',
    'WorkBaby/internal/harness','WorkBaby/internal/core','WorkBaby/internal/llm','WorkBaby/internal/tool',
    'WorkBaby/internal/skill','WorkBaby/internal/mcp','WorkBaby/internal/memory','WorkBaby/internal/rag',
    'WorkBaby/internal/pet','WorkBaby/internal/runtime','WorkBaby/internal/config','WorkBaby/internal/event')

# server 允许依赖 api/domain/event/pkg/gin，禁止 service/repo/能力域
$forbidServer = @('WorkBaby/internal/service','WorkBaby/internal/repo','WorkBaby/internal/db',
    'WorkBaby/internal/harness','WorkBaby/internal/core','WorkBaby/internal/llm','WorkBaby/internal/tool',
    'WorkBaby/internal/skill','WorkBaby/internal/mcp','WorkBaby/internal/memory','WorkBaby/internal/rag',
    'WorkBaby/internal/pet','WorkBaby/internal/runtime','WorkBaby/internal/config')

Assert-NoImports 'pkg-no-internal' $pkgAll @('WorkBaby/internal')
# domain 允许依赖叶子工具包 internal/pkg，禁止依赖其他任何 internal 业务包。
Assert-NoImports 'domain-no-upward' $domainAll @('WorkBaby/internal') @('WorkBaby/internal/pkg')
Assert-NoImports 'repo-no-upward' $repoAll $capDomain
Assert-NoImports 'api-no-repo' $apiAll @('WorkBaby/internal/repo')
Assert-NoImports 'service-no-http' $serviceAll @('github.com/gin-gonic','github.com/wailsapp')
Assert-NoImports 'server-no-downward' $serverAll $forbidServer
# core 是 Agent 内核：持久化与审批经接口注入，因此不得反向依赖编排层与传输层。
# 允许 llm / tool / pkg / domain 四个叶子方向的能力包，其余 internal 业务包一律禁止。
Assert-NoImports 'core-no-upward' $coreAll @('WorkBaby/internal') @(
    'WorkBaby/internal/llm','WorkBaby/internal/tool','WorkBaby/internal/pkg','WorkBaby/internal/domain',
    'WorkBaby/internal/core')
Assert-NoImports 'core-no-http' $coreAll @('github.com/gin-gonic','github.com/wailsapp')

# ---- 3. 契约版本双写校验（前后端常量一致性） ----
$goVer = Select-String -Path (Join-Path $root 'internal/domain/contract.go') -Pattern 'ContractVersion\s*=\s*(\d+)' |
    ForEach-Object { $_.Matches[0].Groups[1].Value } | Select-Object -First 1
$tsFile = Join-Path $root 'frontend/src/src/api/contract.ts'
$tsVer = Select-String -Path $tsFile -Pattern 'UI_API_CONTRACT_VERSION\s*=\s*(\d+)' |
    ForEach-Object { $_.Matches[0].Groups[1].Value } | Select-Object -First 1

if ($goVer -and $tsVer -and $goVer -eq $tsVer) {
    Write-Host "PASS contract-version-sync (v$goVer)"
} else {
    $failures.Add("FAIL contract-version-sync 后端=$goVer 前端=$tsVer")
}

# ---- 4. 汇总 ----
if ($failures.Count -gt 0) {
    Write-Host ''
    Write-Host '======== 门禁失败 ========' -ForegroundColor Red
    foreach ($f in $failures) { Write-Host $f -ForegroundColor Red }
    exit 1
}
Write-Host ''
Write-Host '======== 全部通过 ========' -ForegroundColor Green
exit 0
