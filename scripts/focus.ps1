# 强制把目标窗口拉到真正的前台，再发一次滚轮/点击。
# 后台 PowerShell 调 SetForegroundWindow 会被 Windows 的前台锁拦掉，
# 必须先 AttachThreadInput 挂到目标窗口所属线程，再用 ALT 键技巧解锁。
param(
  [string]$ProcName = "WorkBaby",
  [int]$Wheel = 0,
  [double]$ClickX = -1,
  [double]$ClickY = -1
)
$ErrorActionPreference = 'Stop'

Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Fg {
  [DllImport("user32.dll")] public static extern bool SetThreadDpiAwarenessContext(IntPtr c);
  [DllImport("user32.dll")] public static extern bool AttachThreadInput(uint a, uint b, bool f);
  [DllImport("user32.dll")] public static extern IntPtr SetForegroundWindow(IntPtr h);
  [DllImport("user32.dll")] public static extern bool BringWindowToTop(IntPtr h);
  [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr h, int n);
  [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, IntPtr p);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int x, int y);
  [DllImport("user32.dll")] public static extern void mouse_event(uint f, uint dx, uint dy, int d, IntPtr e);
  [DllImport("user32.dll")] public static extern void keybd_event(byte vk, byte scan, uint f, IntPtr e);
  [DllImport("user32.dll")] public static extern IntPtr GetDesktopWindow();
  [StructLayout(LayoutKind.Sequential)] public struct RECT { public int L, T, R, B; }
}
"@

[void][Fg]::SetThreadDpiAwarenessContext([IntPtr](-4))

$p = Get-Process -Name $ProcName -ErrorAction SilentlyContinue | Select-Object -First 1
if (-not $p) { throw "process not found: $ProcName" }
$h = $p.MainWindowHandle
[void][Fg]::ShowWindow($h, 9)

$me = [Fg]::GetWindowThreadProcessId([Fg]::GetDesktopWindow(), [IntPtr]::Zero)
$you = [Fg]::GetWindowThreadProcessId($h, [IntPtr]::Zero)
[void][Fg]::AttachThreadInput($me, $you, $true)

# ALT 按一下再放，前台锁会认为这是用户主动操作
[Fg]::keybd_event(0x12, 0, 0, [IntPtr]::Zero)
[Fg]::keybd_event(0x12, 0, 2, [IntPtr]::Zero)
[void][Fg]::BringWindowToTop($h)
[void][Fg]::SetForegroundWindow($h)
Start-Sleep -Milliseconds 500
[void][Fg]::AttachThreadInput($me, $you, $false)

$fg = [Fg]::GetForegroundWindow()
Write-Host "foreground==target : $($fg -eq $h)  (target=$h fg=$fg)"

$r = New-Object Fg+RECT
[void][Fg]::GetWindowRect($h, [ref]$r)
$w = $r.R - $r.L; $ht = $r.B - $r.T
Write-Host "rect L=$($r.L) T=$($r.T) W=$w H=$ht"

function Click([double]$cx, [double]$cy) {
  [void][Fg]::SetCursorPos($r.L + [int]($w * $cx), $r.T + [int]($ht * $cy))
  Start-Sleep -Milliseconds 400
  [Fg]::mouse_event(0x0002, 0, 0, 0, [IntPtr]::Zero)
  Start-Sleep -Milliseconds 80
  [Fg]::mouse_event(0x0004, 0, 0, 0, [IntPtr]::Zero)
}

if ($ClickX -ge 0) { Click $ClickX $ClickY; Start-Sleep -Milliseconds 1200 }

if ($Wheel -ne 0) {
  $notch = 120 * [Math]::Sign($Wheel)
  for ($i = 0; $i -lt [Math]::Abs($Wheel); $i++) {
    [Fg]::mouse_event(0x0800, 0, 0, $notch, [IntPtr]::Zero)
    Start-Sleep -Milliseconds 70
  }
  Start-Sleep -Milliseconds 800
}
Write-Host "done"
