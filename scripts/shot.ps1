# 截运行中的 WorkBaby 窗口（真机视觉验收）：可先置顶、调整尺寸、点一下、滚一段、
# 拖一下、敲一句。全部走 Win32 事件——窗口由另一个进程的消息循环驱动，
# SendKeys / Set-Clipboard 之类的高层 cmdlet 送不进 WebView2。
param(
  [string]$Out = "D:\GoFiles\WorkBaby\shot.png",
  [string]$ProcName = "WorkBaby",
  [int]$W = 0,
  [int]$H = 0,
  [int]$Wheel = 0,
  [double]$ClickX = -1,
  [double]$ClickY = -1,
  [string]$Type = "",
  [string]$Drag = ""
)
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing

Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Cap {
  [DllImport("user32.dll")] public static extern IntPtr SetThreadDpiAwarenessContext(IntPtr ctx);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint flags);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr h, IntPtr after, int x, int y, int cx, int cy, uint flags);
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
  [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr h, int n);
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int x, int y);
  [DllImport("user32.dll")] public static extern void mouse_event(uint f, uint dx, uint dy, int d, IntPtr e);
  [DllImport("user32.dll")] public static extern void keybd_event(byte vk, byte scan, uint f, IntPtr e);
  [StructLayout(LayoutKind.Sequential)] public struct RECT { public int L, T, R, B; }
}
"@ -ReferencedAssemblies System.Drawing

# 必须先切到 per-monitor aware：不切的话 GetWindowRect 返回的是虚拟化坐标，
# 高 DPI 屏上窗口真实物理尺寸是它的 1.75 倍，截出来只剩左上角一块。
[void][Cap]::SetThreadDpiAwarenessContext([IntPtr](-4))

$p = Get-Process -Name $ProcName -ErrorAction SilentlyContinue | Select-Object -First 1
if (-not $p) { throw "process not found: $ProcName" }
$h = $p.MainWindowHandle
if ($h -eq [IntPtr]::Zero) { throw "no main window handle" }

[void][Cap]::ShowWindow($h, 9)
if ($W -gt 0 -and $H -gt 0) {
  [void][Cap]::SetWindowPos($h, [IntPtr]::Zero, 0, 0, $W, $H, 0x0040)
  Start-Sleep -Milliseconds 900
}
[void][Cap]::SetForegroundWindow($h)
Start-Sleep -Milliseconds 1400

$r = New-Object Cap+RECT
[void][Cap]::GetWindowRect($h, [ref]$r)
$w = $r.R - $r.L
$ht = $r.B - $r.T

if ($ClickX -ge 0) {
  # 坐标用窗口尺寸的比例给，换分辨率不用重算
  $px = $r.L + [int]($w * $ClickX)
  $py = $r.T + [int]($ht * $ClickY)
  [void][Cap]::SetCursorPos($px, $py)
  Start-Sleep -Milliseconds 300
  [Cap]::mouse_event(0x0002, 0, 0, 0, [IntPtr]::Zero)
  [Cap]::mouse_event(0x0004, 0, 0, 0, [IntPtr]::Zero)
  Start-Sleep -Milliseconds 1500
}

if ($Wheel -ne 0) {
  # 先在消息区空白处点一下把焦点交给 WebView，否则滚轮/按键被置顶的动态壁纸层吃掉
  $cx = $r.L + [int]($w * 0.62)
  $cy = $r.T + [int]($ht * 0.5)
  [void][Cap]::SetCursorPos($cx, $cy)
  Start-Sleep -Milliseconds 250
  [Cap]::mouse_event(0x0002, 0, 0, 0, [IntPtr]::Zero)
  [Cap]::mouse_event(0x0004, 0, 0, 0, [IntPtr]::Zero)
  Start-Sleep -Milliseconds 300
  $notch = 120 * [Math]::Sign($Wheel)
  for ($i = 0; $i -lt [Math]::Abs($Wheel); $i++) {
    [Cap]::mouse_event(0x0800, 0, 0, $notch, [IntPtr]::Zero)
    Start-Sleep -Milliseconds 80
  }
  Start-Sleep -Milliseconds 800
}

# 拖拽：x1,y1,x2,y2 全部是窗口尺寸的比例，中间分步移动让 pointermove 真的触发
if ($Drag -ne "") {
  $p = $Drag.Split(',') | ForEach-Object { [double]$_ }
  $x1 = $r.L + [int]($w * $p[0]); $y1 = $r.T + [int]($ht * $p[1])
  $x2 = $r.L + [int]($w * $p[2]); $y2 = $r.T + [int]($ht * $p[3])
  [void][Cap]::SetCursorPos($x1, $y1)
  Start-Sleep -Milliseconds 300
  [Cap]::mouse_event(0x0002, 0, 0, 0, [IntPtr]::Zero)
  Start-Sleep -Milliseconds 150
  $steps = 24
  for ($i = 1; $i -le $steps; $i++) {
    $sx = [int]($x1 + ($x2 - $x1) * $i / $steps)
    $sy = [int]($y1 + ($y2 - $y1) * $i / $steps)
    [void][Cap]::SetCursorPos($sx, $sy)
    Start-Sleep -Milliseconds 25
  }
  Start-Sleep -Milliseconds 250
  [Cap]::mouse_event(0x0004, 0, 0, 0, [IntPtr]::Zero)
  Start-Sleep -Milliseconds 600
}

# 敲字：先点一下输入框拿到焦点，再逐字符发按键事件
if ($Type -ne "") {
  foreach ($ch in $Type.ToCharArray()) {
    $vk = [int][char]$ch
    if ($vk -ge 65 -and $vk -le 90) { $vk = $vk + 32 }   # 统一小写，省掉 shift
    if ($vk -lt 32 -or $vk -gt 126) { continue }         # 非 ASCII 跳过
    [Cap]::keybd_event([byte]$vk, 0, 0, [IntPtr]::Zero)
    [Cap]::keybd_event([byte]$vk, 0, 2, [IntPtr]::Zero)
    Start-Sleep -Milliseconds 8
  }
  Start-Sleep -Milliseconds 500
}

$bmp = New-Object System.Drawing.Bitmap $w, $ht
$g = [System.Drawing.Graphics]::FromImage($bmp)
$hdc = $g.GetHdc()
# 2 = PW_RENDERFULLCONTENT，绕过被遮挡 / DWM 合成拿不到内容的问题
$ok = [Cap]::PrintWindow($h, $hdc, 2)
$g.ReleaseHdc($hdc)
$g.Dispose()
$bmp.Save($Out, [System.Drawing.Imaging.ImageFormat]::Png)
$bmp.Dispose()
"saved $Out ($w x $ht) printwindow=$ok"
