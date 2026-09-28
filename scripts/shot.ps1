param([int]$X = 110, [int]$Y = 251)
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing

$code = @"
using System;
using System.Runtime.InteropServices;
public class WBC {
  [StructLayout(LayoutKind.Sequential)] public struct RECT { public int L,T,R,B; }
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
  [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr h, int c);
  [DllImport("user32.dll")] public static extern bool MoveWindow(IntPtr h,int x,int y,int w,int ht,bool rp);
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int x, int y);
  [DllImport("user32.dll")] public static extern void mouse_event(uint f, uint dx, uint dy, uint d, IntPtr e);
  [DllImport("user32.dll")] public static extern IntPtr GetDC(IntPtr h);
  [DllImport("user32.dll")] public static extern int ReleaseDC(IntPtr h, IntPtr dc);
  [DllImport("gdi32.dll")] public static extern bool BitBlt(IntPtr d,int x,int y,int w,int ht,IntPtr s,int sx,int sy,int rop);
}
"@
if (-not ('WBC' -as [type])) { Add-Type -TypeDefinition $code }

$p = Get-Process WorkBaby -ErrorAction SilentlyContinue | Select-Object -First 1
if (-not $p) { Write-Output 'NOT RUNNING'; exit 1 }
$h = $p.MainWindowHandle
# 3 = SW_MAXIMIZE；最大化后窗口铺满工作区，量出来的才是真实布局宽度
[WBC]::ShowWindow($h, 3) | Out-Null
[WBC]::SetForegroundWindow($h) | Out-Null
Start-Sleep -Milliseconds 1200

if ($X -gt 0) {
  [WBC]::SetCursorPos($X, $Y) | Out-Null
  Start-Sleep -Milliseconds 250
  [WBC]::mouse_event(0x0002, 0, 0, 0, [IntPtr]::Zero)
  [WBC]::mouse_event(0x0004, 0, 0, 0, [IntPtr]::Zero)
  Start-Sleep -Milliseconds 900
}

$r = New-Object WBC+RECT
[WBC]::GetWindowRect($h, [ref]$r) | Out-Null
$w = $r.R - $r.L
$ht = $r.B - $r.T
$bmp = New-Object System.Drawing.Bitmap $w, $ht
$g = [System.Drawing.Graphics]::FromImage($bmp)
$dc = $g.GetHdc()
$src = [WBC]::GetDC([IntPtr]::Zero)
[WBC]::BitBlt($dc, 0, 0, $w, $ht, $src, $r.L, $r.T, 0x00CC0020) | Out-Null
$g.ReleaseHdc($dc)
[WBC]::ReleaseDC([IntPtr]::Zero, $src) | Out-Null
$g.Dispose()
$out = 'D:\GoFiles\WorkBaby\build\bin\_shot.png'
$bmp.Save($out, [System.Drawing.Imaging.ImageFormat]::Png)
$bmp.Dispose()
Write-Output "saved ${w}x${ht} -> $out"
