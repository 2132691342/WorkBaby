# check-contract.ps1 — WorkBaby 契约门禁（CLAUDE.md §4 补充）
#
# 检查项：
#   route-cancel-resume-split   POST /chat/sessions/:id/cancel 与 POST /chat/runs/:id/resume 必须分别走不同路径
#                               （同形异义陷阱治理）
#   http-prefix-sync            前端 api/client.ts 的 KNOWN_PREFIXES 必须与后端 routes_*.go 注册路径前缀一致
#
# 设计原则：宁可误报也不要漏报——前端调了未注册路径就是 404，契约漂移必须立刻发现。
#
# 用法： powershell -ExecutionPolicy Bypass -File scripts/check-contract.ps1

$ErrorActionPreference = 'Stop'
try { [Console]::OutputEncoding = [System.Text.Encoding]::UTF8 } catch {}
# 脚本位于 <root>/scripts/，根目录是其祖父级
# Split-Path -Parent 在某些 PS 版本会做 cwd-relative join；用 Convert-Path 转成绝对路径
$scriptPath = (Resolve-Path -LiteralPath $PSCommandPath).Path
$root = Split-Path -Parent (Split-Path -Parent $scriptPath)
Set-Location $root

$failures = New-Object System.Collections.Generic.List[string]

function Fail([string]$msg) {
    Write-Host "FAIL  $msg" -ForegroundColor Red
    $failures.Add($msg)
}

function Pass([string]$msg) {
    Write-Host "PASS  $msg" -ForegroundColor Green
}

# ============== 1. cancel/resume 路径拆分检查 ==============
$cancelPattern = 'v1\.POST\("/chat/sessions/:id/cancel"'
$resumePattern = 'v1\.POST\("/chat/runs/:id/resume"'
$legacyCancel  = 'v1\.POST\("/chat/stream/:id/cancel"'
$legacyResume  = 'v1\.POST\("/chat/stream/:id/resume"'

$routeFiles = @(Get-ChildItem -Path (Join-Path $root 'internal\server') -Recurse -Filter 'routes_*.go' -ErrorAction SilentlyContinue | ForEach-Object { $_.FullName })
if ($routeFiles.Count -eq 0) {
    Write-Host "DEBUG  cwd=$((Get-Location).Path)  root=$root" -ForegroundColor Yellow
    Write-Host "DEBUG  joined=$(Join-Path $root 'internal\server')" -ForegroundColor Yellow
    Fail "无法定位 routes_*.go（cwd=$((Get-Location).Path)）"
}

function HasMatch($files, $substr) {
    foreach ($f in $files) {
        if (Get-Content -Raw -LiteralPath $f | Select-String -SimpleMatch $substr -Quiet) { return $true }
    }
    return $false
}

$hasNewCancel = HasMatch $routeFiles '/chat/sessions/:id/cancel'
$hasNewResume = HasMatch $routeFiles '/chat/runs/:id/resume'
$hasOldCancel = HasMatch $routeFiles '/chat/stream/:id/cancel'
$hasOldResume = HasMatch $routeFiles '/chat/stream/:id/resume'

if (-not $hasNewCancel) { Fail "缺少 POST /chat/sessions/:id/cancel 路由（按 CLAUDE.md §2.12 拆分后应为 sessionID 路由）" }
else { Pass "POST /chat/sessions/:id/cancel 已注册" }
if (-not $hasNewResume) { Fail "缺少 POST /chat/runs/:id/resume 路由（按 CLAUDE.md §2.12 拆分后应为 runID 路由）" }
else { Pass "POST /chat/runs/:id/resume 已注册" }
if ($hasOldCancel) { Fail "仍存在 POST /chat/stream/:id/cancel 旧路由——会与拆分后的 sessionID 路由冲突" }
if ($hasOldResume) { Fail "仍存在 POST /chat/stream/:id/resume 旧路由——会与拆分后的 runID 路由冲突" }

# ============== 2. 前端 KNOWN_PREFIXES 与后端路由前缀一致性 ==============
#
# 原则：前端 KNOWN_PREFIXES 是「段级白名单」——数组元素要么 '/api/v1/X'（段根），要么 '/api/v1/X/'（段根带 / 后缀）。
# 后端 routes_*.go 注册的路径去掉 :xxx 占位符后，凡归属到某段的，均应被前端对应白名单覆盖。
#
# 双向匹配语义：前缀 p 满足「p 等于 fp 或以 fp 开头」即视为匹配。
$backendSegments = @{}
foreach ($f in $routeFiles) {
    $content = Get-Content -Raw -LiteralPath $f
    # 注意：不得命名 $matches —— 与 PowerShell 自动变量冲突，-match 会覆盖它
    # routes_*.go 注册的是相对 gin Group 的路径（如 "/chat/sessions"），完整路径 = "/api/v1" + 相对路径
    # 接收者变量名不一（v1 / g / r 等），用 [A-Za-z]\w* 匹配
    $routeMatches = [regex]::Matches($content, '[A-Za-z]\w*\.(?:GET|POST|PUT|DELETE|Any|Handle)\("(/[^"]+)"')
    foreach ($rm in $routeMatches) {
        $path = '/api/v1' + $rm.Groups[1].Value
        # 去掉 :id / :name / :itemID / :mode / :cid / :sid / :run_id 等占位符
        $clean = [regex]::Replace($path, ':[A-Za-z_][A-Za-z0-9_]*', '')
        $clean = $clean.TrimEnd('/')
        # '/api/v1/chat/sessions' split 后 4 元素，取前 4 个 join = '/api/v1/chat'
        if ($clean -match '^/api/v1/[^/]+(/.*)?$') {
            $seg = ($clean -split '/')[0..3] -join '/'
            $backendSegments[$seg] = $true
        }
    }
}
$backendSegmentList = ($backendSegments.Keys | Sort-Object)

$clientFile = 'frontend\src\src\api\client.ts'
$frontendSegments = @{}
$lines = Get-Content -LiteralPath (Join-Path $root $clientFile)
for ($i = 0; $i -lt $lines.Count; $i++) {
    $line = $lines[$i]
    $m = [regex]::Match($line, "^\s*'(/api/v1/[^']+)'\s*,?\s*$")
    if (-not $m.Success) { continue }
    $p = $m.Groups[1].Value.TrimEnd('/')
    if ($p -eq '/api/v1') { continue }
    if ($p -match '^/api/v1/[^/]+(/.*)?$') {
        $seg = ($p -split '/')[0..3] -join '/'
        $frontendSegments[$seg] = $true
    }
}
if ($frontendSegments.Count -eq 0) {
    Fail "无法从 $clientFile 解析出任何 KNOWN_PREFIXES（文件不存在或为空）"
}
$frontendSegmentList = ($frontendSegments.Keys | Sort-Object)
if ($env:WB_CONTRACT_DEBUG) {
    Write-Host "DEBUG frontendSegments:" -ForegroundColor Yellow
    $frontendSegmentList | ForEach-Object { Write-Host "  [$_]" -ForegroundColor Yellow }
    Write-Host "DEBUG backendSegments:" -ForegroundColor Yellow
    $backendSegmentList | ForEach-Object { Write-Host "  [$_]" -ForegroundColor Yellow }
}

# A. 后端有段但前端无 → 死段（前端永远无法触达，需评估是否下线）
$backendOnly = @()
foreach ($seg in $backendSegmentList) {
    $matched = $false
    foreach ($fp in $frontendSegmentList) {
        if ($seg -eq $fp -or $seg.StartsWith($fp + '/')) { $matched = $true; break }
    }
    if (-not $matched) { $backendOnly += $seg }
}

# B. 前端有段但后端无 → 必 404（最严重）
$frontendOnly = @()
foreach ($seg in $frontendSegmentList) {
    $matched = $false
    foreach ($bp in $backendSegmentList) {
        if ($seg -eq $bp -or $seg.StartsWith($bp + '/')) { $matched = $true; break }
    }
    if (-not $matched) { $frontendOnly += $seg }
}

if ($frontendOnly.Count -gt 0) {
    $frontendOnly | ForEach-Object { Fail "前端调用 KNOWN_PREFIXES '$_' 在后端无任何匹配路由（必 404）" }
} else {
    Pass "前端 KNOWN_PREFIXES 全部匹配后端路由"
}

if ($backendOnly.Count -gt 0) {
    Write-Host ""
    Write-Host "WARN  以下后端前缀前端未覆盖（可能是死接口，审查是否下线）：" -ForegroundColor Yellow
    $backendOnly | ForEach-Object { Write-Host "      $_" -ForegroundColor Yellow }
}

# ============== 收尾 ==============
Write-Host ""
if ($failures.Count -gt 0) {
    Write-Host "======== 失败 $($failures.Count) 项 ========" -ForegroundColor Red
    exit 1
}
Write-Host "======== 全部通过 ========" -ForegroundColor Green
