# check-contract.ps1 - WorkBaby contract gate (see AGENTS.md section 4)
#
# Checks:
#   route-cancel-resume-split   POST /chat/sessions/:id/cancel and POST /chat/runs/:id/resume
#                               must live on different paths (same-shape-different-meaning trap)
#   http-prefix-sync            frontend api/client.ts KNOWN_PREFIXES must match backend
#                               routes_*.go registered path prefixes
#
# Principle: prefer false positives over misses - an unregistered frontend path is a hard 404,
# and contract drift must surface immediately.
#
# Usage: powershell -ExecutionPolicy Bypass -File scripts/check-contract.ps1

$ErrorActionPreference = 'Stop'
try { [Console]::OutputEncoding = [System.Text.Encoding]::UTF8 } catch {}
# Script lives in <root>/scripts/, so the repo root is its grandparent.
# Split-Path -Parent may do cwd-relative joins on some PS versions; resolve to absolute first.
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

# ============== 1. cancel/resume path split ==============
$routeFiles = @(Get-ChildItem -Path (Join-Path $root 'internal\server') -Recurse -Filter 'routes*.go' -ErrorAction SilentlyContinue | ForEach-Object { $_.FullName })
if ($routeFiles.Count -eq 0) {
    Write-Host "DEBUG  cwd=$((Get-Location).Path)  root=$root" -ForegroundColor Yellow
    Write-Host "DEBUG  joined=$(Join-Path $root 'internal\server')" -ForegroundColor Yellow
    Fail "cannot locate routes*.go (cwd=$((Get-Location).Path))"
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

if (-not $hasNewCancel) { Fail "missing route POST /chat/sessions/:id/cancel (sessionID route per AGENTS.md)" }
else { Pass "POST /chat/sessions/:id/cancel registered" }
if (-not $hasNewResume) { Fail "missing route POST /chat/runs/:id/resume (runID route per AGENTS.md)" }
else { Pass "POST /chat/runs/:id/resume registered" }
if ($hasOldCancel) { Fail "legacy route POST /chat/stream/:id/cancel still present - conflicts with the sessionID route" }
if ($hasOldResume) { Fail "legacy route POST /chat/stream/:id/resume still present - conflicts with the runID route" }

# ============== 2. frontend KNOWN_PREFIXES vs backend route prefixes ==============
#
# KNOWN_PREFIXES is a segment-level allowlist: each entry is either '/api/v1/X' (segment root)
# or '/api/v1/X/' (segment root with trailing slash).
# Backend routes registered in routes_*.go, after stripping :xxx placeholders, should all be
# covered by the matching frontend entry.
#
# Bidirectional match: prefix p matches frontend prefix fp when p equals fp or starts with fp.
$backendSegments = @{}
foreach ($f in $routeFiles) {
    $content = Get-Content -Raw -LiteralPath $f
    # Do not name a variable $matches - it collides with the PowerShell automatic variable
    # and -match overwrites it.
    # routes_*.go registers paths relative to a gin group (e.g. "/chat/sessions");
    # the full path is "/api/v1" + relative path. Receiver names vary (v1 / g / r),
    # hence the [A-Za-z]\w* pattern.
    $routeMatches = [regex]::Matches($content, '[A-Za-z]\w*\.(?:GET|POST|PUT|DELETE|Any|Handle)\("(/[^"]+)"')
    foreach ($rm in $routeMatches) {
        $path = '/api/v1' + $rm.Groups[1].Value
        # strip :id / :name / :itemID / :mode / :cid / :sid / :run_id placeholders
        $clean = [regex]::Replace($path, ':[A-Za-z_][A-Za-z0-9_]*', '')
        $clean = $clean.TrimEnd('/')
        # '/api/v1/chat/sessions' splits into 4 elements; the first 4 joined give '/api/v1/chat'
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
    Fail "cannot parse any KNOWN_PREFIXES from $clientFile (missing or empty)"
}
$frontendSegmentList = ($frontendSegments.Keys | Sort-Object)
if ($env:WB_CONTRACT_DEBUG) {
    Write-Host "DEBUG frontendSegments:" -ForegroundColor Yellow
    $frontendSegmentList | ForEach-Object { Write-Host "  [$_]" -ForegroundColor Yellow }
    Write-Host "DEBUG backendSegments:" -ForegroundColor Yellow
    $backendSegmentList | ForEach-Object { Write-Host "  [$_]" -ForegroundColor Yellow }
}

# A. backend segment not covered by frontend - dead segment (unreachable, review for removal)
$backendOnly = @()
foreach ($seg in $backendSegmentList) {
    $matched = $false
    foreach ($fp in $frontendSegmentList) {
        if ($seg -eq $fp -or $seg.StartsWith($fp + '/')) { $matched = $true; break }
    }
    if (-not $matched) { $backendOnly += $seg }
}

# B. frontend segment with no backend route - guaranteed 404 (most severe)
$frontendOnly = @()
foreach ($seg in $frontendSegmentList) {
    $matched = $false
    foreach ($bp in $backendSegmentList) {
        if ($seg -eq $bp -or $seg.StartsWith($bp + '/')) { $matched = $true; break }
    }
    if (-not $matched) { $frontendOnly += $seg }
}

if ($frontendOnly.Count -gt 0) {
    $frontendOnly | ForEach-Object { Fail "frontend KNOWN_PREFIXES '$_' has no matching backend route (guaranteed 404)" }
} else {
    Pass "all frontend KNOWN_PREFIXES match backend routes"
}

if ($backendOnly.Count -gt 0) {
    Write-Host ""
    Write-Host "WARN  backend prefixes not covered by frontend (possibly dead endpoints):" -ForegroundColor Yellow
    $backendOnly | ForEach-Object { Write-Host "      $_" -ForegroundColor Yellow }
}

# ============== summary ==============
Write-Host ""
if ($failures.Count -gt 0) {
    Write-Host "======== $($failures.Count) failure(s) ========" -ForegroundColor Red
    exit 1
}
Write-Host "======== all checks passed ========" -ForegroundColor Green
