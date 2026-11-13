# make-icons.ps1 — 生成 WorkBaby 全套图标（可复现，改完跑一遍即可）。
# 产物：build\appicon.png（1024 主稿）、build\windows\icon.ico（多尺寸）、
#       backend\shell\res\icon.ico（托盘内嵌）、frontend\public\favicon.ico。
# 设计：靛蓝渐变圆角砖 + 白色圆头 W 字标 + 右上 AI 火花；16px 下 W 仍可辨。
# 必须用 Windows PowerShell 5.1 跑（System.Drawing）；文件带 BOM，避免 GBK 误读。
Add-Type -AssemblyName System.Drawing

# 仓库根：tools 的上两级（build\tools → 仓库根）。
$root = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent

function New-IconBitmap([int]$size) {
    $bmp = [System.Drawing.Bitmap]::new($size, $size, [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.SmoothingMode = 'AntiAlias'
    $g.CompositingQuality = 'HighQuality'
    $g.PixelOffsetMode = 'HighQuality'
    $s = [float]$size / 1024.0

    # 圆角砖：22% 圆角半径，对角渐变 #6366f1 → #818cf8
    $r = 225 * $s
    $tile = [System.Drawing.Drawing2D.GraphicsPath]::new()
    $tile.AddArc(0, 0, 2 * $r, 2 * $r, 180, 90)
    $tile.AddArc($size - 2 * $r, 0, 2 * $r, 2 * $r, 270, 90)
    $tile.AddArc($size - 2 * $r, $size - 2 * $r, 2 * $r, 2 * $r, 0, 90)
    $tile.AddArc(0, $size - 2 * $r, 2 * $r, 2 * $r, 90, 90)
    $tile.CloseFigure()

    $rect = [System.Drawing.RectangleF]::new(0, 0, $size, $size)
    $grad = [System.Drawing.Drawing2D.LinearGradientBrush]::new(
        $rect,
        [System.Drawing.Color]::FromArgb(255, 99, 102, 241),
        [System.Drawing.Color]::FromArgb(255, 129, 140, 248),
        [float]45)
    $g.FillPath($grad, $tile)

    # 左上柔光：白 46/255 → 0，给砖面一点体积感
    $halo = [System.Drawing.RectangleF]::new(60 * $s, 40 * $s, 720 * $s, 560 * $s)
    $haloPath = [System.Drawing.Drawing2D.GraphicsPath]::new()
    $haloPath.AddEllipse($halo)
    $haloBrush = [System.Drawing.Drawing2D.PathGradientBrush]::new($haloPath)
    $haloBrush.CenterColor = [System.Drawing.Color]::FromArgb(46, 255, 255, 255)
    $haloBrush.SurroundColors = @([System.Drawing.Color]::FromArgb(0, 255, 255, 255))
    $g.FillPath($haloBrush, $tile)

    # W 字标：粗圆头折线；先画一层偏移影，再画本体。小尺寸笔画加粗 1.15 倍保辨识。
    $penW = if ($size -le 32) { 92 * 1.15 * $s } else { 92 * $s }
    $pts = [System.Collections.Generic.List[System.Drawing.PointF]]::new()
    $pts.Add([System.Drawing.PointF]::new(288 * $s, 336 * $s))
    $pts.Add([System.Drawing.PointF]::new(400 * $s, 696 * $s))
    $pts.Add([System.Drawing.PointF]::new(512 * $s, 512 * $s))
    $pts.Add([System.Drawing.PointF]::new(624 * $s, 696 * $s))
    $pts.Add([System.Drawing.PointF]::new(736 * $s, 336 * $s))

    $shadow = [System.Drawing.Pen]::new([System.Drawing.Color]::FromArgb(42, 30, 27, 90), [float]$penW)
    $shadow.StartCap = 'Round'; $shadow.EndCap = 'Round'; $shadow.LineJoin = 'Round'
    $shadowPts = [System.Collections.Generic.List[System.Drawing.PointF]]::new()
    foreach ($p in $pts) { $shadowPts.Add([System.Drawing.PointF]::new($p.X, $p.Y + 16 * $s)) }
    $g.DrawLines($shadow, $shadowPts.ToArray())

    $ink = [System.Drawing.Pen]::new([System.Drawing.Color]::White, [float]$penW)
    $ink.StartCap = 'Round'; $ink.EndCap = 'Round'; $ink.LineJoin = 'Round'
    $g.DrawLines($ink, $pts.ToArray())

    # AI 火花：右上四角星，白 235/255
    $cx = 824 * $s; $cy = 268 * $s; $ro = 62 * $s; $ri = 15 * $s
    $star = [System.Collections.Generic.List[System.Drawing.PointF]]::new()
    for ($i = 0; $i -lt 8; $i++) {
        $ang = [Math]::PI / 2 - $i * [Math]::PI / 4
        $rad = if ($i % 2 -eq 0) { $ro } else { $ri }
        $star.Add([System.Drawing.PointF]::new($cx + $rad * [Math]::Cos($ang), $cy - $rad * [Math]::Sin($ang)))
    }
    $g.FillPolygon([System.Drawing.SolidBrush]::new([System.Drawing.Color]::FromArgb(235, 255, 255, 255)), $star.ToArray())

    # 砖面描边：白 30/255 内描边，收住边缘
    $edge = [System.Drawing.Pen]::new([System.Drawing.Color]::FromArgb(30, 255, 255, 255), [float][Math]::Max(1.0, 3 * $s))
    $g.DrawPath($edge, $tile)

    $g.Dispose()
    return $bmp
}

function Save-Ico([int[]]$sizes, [string]$path) {
    $blobs = @()
    foreach ($sz in $sizes) {
        $b = New-IconBitmap $sz
        $ms = [System.IO.MemoryStream]::new()
        $b.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
        $blobs += , @($sz, $ms.ToArray())
        $b.Dispose()
    }
    $fs = [System.IO.File]::Create($path)
    $bw = [System.IO.BinaryWriter]::new($fs)
    $bw.Write([uint16]0); $bw.Write([uint16]1); $bw.Write([uint16]$blobs.Count)
    $offset = 6 + 16 * $blobs.Count
    foreach ($e in $blobs) {
        $sz = [int]$e[0]; $data = [byte[]]$e[1]
        $bw.Write([byte]$(if ($sz -ge 256) { 0 } else { $sz }))
        $bw.Write([byte]$(if ($sz -ge 256) { 0 } else { $sz }))
        $bw.Write([byte]0); $bw.Write([byte]0)
        $bw.Write([uint16]1); $bw.Write([uint16]32)
        $bw.Write([uint32]$data.Length); $bw.Write([uint32]$offset)
        $offset += $data.Length
    }
    foreach ($e in $blobs) { $bw.Write([byte[]]$e[1]) }
    $bw.Close(); $fs.Close()
}

# 主稿 1024
$master = New-IconBitmap 1024
$master.Save((Join-Path $root 'build\appicon.png'), [System.Drawing.Imaging.ImageFormat]::Png)
$master.Dispose()

$all = @(16, 24, 32, 48, 64, 128, 256)
Save-Ico $all (Join-Path $root 'build\windows\icon.ico')

$shellRes = Join-Path $root 'backend\shell\res'
New-Item -ItemType Directory -Force -Path $shellRes | Out-Null
Save-Ico $all (Join-Path $shellRes 'icon.ico')

$public = Join-Path $root 'frontend\public'
New-Item -ItemType Directory -Force -Path $public | Out-Null
Save-Ico @(16, 24, 32, 48) (Join-Path $public 'favicon.ico')

Write-Host 'icons done.'
