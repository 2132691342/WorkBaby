$ErrorActionPreference = 'Stop'

# 语义键 → Lucide 图标名
$map = [ordered]@{
  'chat'='message-square'; 'plus'='plus'; 'search'='search'; 'settings'='settings'
  'folder'='folder'; 'folder-open'='folder-open'; 'file'='file'; 'file-text'='file-text'
  'memory'='database'; 'book'='book-open'; 'zap'='zap'; 'server'='server'; 'bot'='bot'
  'terminal'='terminal'; 'webhook'='webhook'; 'dashboard'='layout-dashboard'
  'play'='circle-play'; 'paw'='paw-print'; 'scroll'='scroll-text'; 'info'='info'
  'wrench'='wrench'; 'wiki'='book-marked'; 'chart'='chart-line'; 'inbox'='inbox'
  'send'='send'; 'square'='square'; 'paperclip'='paperclip'; 'at'='at-sign'
  'shield'='shield-check'; 'cpu'='cpu'; 'sparkle'='sparkles'; 'brain'='brain'
  'copy'='copy'; 'trash'='trash'; 'edit'='pencil'; 'refresh'='refresh-cw'
  'branch'='git-branch'; 'check'='check'; 'x'='x'
  'arrow-right'='arrow-right'; 'arrow-left'='arrow-left'
  'chev-down'='chevron-down'; 'chev-right'='chevron-right'; 'chev-up'='chevron-up'
  'more'='ellipsis'; 'pin'='pin'; 'archive'='archive'; 'eye'='eye'
  'upload'='upload'; 'download'='download'; 'link'='link'; 'clock'='clock'
  'warn'='triangle-alert'; 'lock'='lock'; 'globe'='globe'; 'image'='image'
  'code'='code'; 'layers'='layers'; 'target'='target'; 'list'='list'
  'crosshair'='crosshair'; 'moon'='moon'; 'sun'='sun'; 'globe2'='languages'
  'key'='key-round'; 'activity'='activity'; 'filter'='funnel'; 'grab'='grip-vertical'
  'minus'='minus'; 'external'='external-link'; 'ruler'='ruler'; 'quote'='quote'
  'slash'='slash'; 'loader'='loader-circle'; 'star'='star'; 'thumbs-up'='thumbs-up'
}

$names = ($map.Values | Select-Object -Unique) -join ','
$url = "https://api.iconify.design/lucide.json?icons=$names"
Write-Host "fetching $($map.Count) icons ..."
$resp = Invoke-RestMethod -Uri $url -TimeoutSec 60

$sb = New-Object System.Text.StringBuilder
[void]$sb.AppendLine("/* ============================================================")
[void]$sb.AppendLine("   图标：Lucide（ISC 许可）内联快照，24x24 网格 / 统一笔画")
[void]$sb.AppendLine("   用法：ic('send', 16) → 返回 svg 字符串")
[void]$sb.AppendLine("   生成：scripts 一次性拉取后内联，离线可用（勿手改路径）")
[void]$sb.AppendLine("   ============================================================ */")
[void]$sb.AppendLine("(function () {")
[void]$sb.AppendLine("  'use strict';")
[void]$sb.AppendLine("  var P = {")

$missing = @()
foreach ($k in $map.Keys) {
  $lucideName = $map[$k]
  $icon = $resp.icons.$lucideName
  if (-not $icon) { $missing += "$k($lucideName)"; continue }
  $body = [string]$icon.body
  $body = $body -replace "`r?`n", ''
  # 去掉图标体自带的填充/描边属性，让外层 svg 统一控制笔画粗细
  $body = $body -replace '\s*(fill|stroke|stroke-width|stroke-linecap|stroke-linejoin|stroke-miterlimit|stroke-dasharray)="[^"]*"', ''
  $body = $body.Trim()
  $body = $body -replace "'", "\'"
  [void]$sb.AppendLine("    '$k': '$body',")
}

[void]$sb.AppendLine("  };")
[void]$sb.AppendLine("")
[void]$sb.AppendLine("  function ic(name, size) {")
[void]$sb.AppendLine("    var d = P[name];")
[void]$sb.AppendLine("    if (!d) return '';")
[void]$sb.AppendLine("    var s = size || 16;")
[void]$sb.AppendLine("    return (")
[void]$sb.AppendLine("      '<svg class=`"ic`" width=`"' + s + '`" height=`"' + s + '`" viewBox=`"0 0 24 24`" fill=`"none`" ' +")
[void]$sb.AppendLine("      'stroke=`"currentColor`" stroke-width=`"1.7`" stroke-linecap=`"round`" stroke-linejoin=`"round`" aria-hidden=`"true`">' +")
[void]$sb.AppendLine("      d + '</svg>'")
[void]$sb.AppendLine("    );")
[void]$sb.AppendLine("  }")
[void]$sb.AppendLine("")
[void]$sb.AppendLine("  window.ic = ic;")
[void]$sb.AppendLine("})();")

$out = 'd:\GoFiles\WorkBaby\doc\prd\prototype\js\icons.js'
[System.IO.File]::WriteAllText($out, $sb.ToString(), (New-Object System.Text.UTF8Encoding($false)))

Write-Host "written: $out"
Write-Host "icons: $($resp.icons.PSObject.Properties.Count)"
if ($missing.Count -gt 0) { Write-Host "MISSING: $($missing -join ', ')" }
