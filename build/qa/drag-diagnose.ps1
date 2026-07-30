# 드래그 진단: 더 느리게, 버튼 다운 후 충분히 대기, 여러 그랩 위치
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class DD {
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int x, int y);
  [DllImport("user32.dll")] public static extern void mouse_event(uint f, int x, int y, uint d, IntPtr e);
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
  [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
  public struct RECT { public int L; public int T; public int R; public int B; }
  public const uint LDOWN=0x0002, LUP=0x0004, MOVE=0x0001, ABS=0x8000;
}
"@
Get-Process -Name qd-354 -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep 2
$p = Start-Process "C:\dev\qd-354.exe" -PassThru
Start-Sleep 16
$h = $p.MainWindowHandle
if ([DD]::GetForegroundWindow() -eq 0) { Write-Host "잠금"; Get-Process -Id $p.Id|Stop-Process -Force; exit }

$r = New-Object DD+RECT; [DD]::GetWindowRect($h, [ref]$r) | Out-Null
Write-Host "시작: ($($r.L),$($r.T))"

# 타이틀바 정중앙 빈 곳 (제목과 버튼 사이)
$gx = $r.L + 280; $gy = $r.T + 17
[DD]::SetForegroundWindow($h) | Out-Null; Start-Sleep -Milliseconds 500
[DD]::SetCursorPos($gx, $gy); Start-Sleep -Milliseconds 300
Write-Host "그랩 ($gx,$gy) 에서 LDOWN"
[DD]::mouse_event([DD]::LDOWN, 0, 0, 0, [IntPtr]::Zero)
Start-Sleep -Milliseconds 500   # 마우스다운 처리 대기 (grab offset 선취)

# 큰 폭으로 5번만 이동, 각 단계 충분히 대기
$positions = @()
for ($i = 1; $i -le 5; $i++) {
  [DD]::SetCursorPos(($gx + $i*30), ($gy + $i*20))
  Start-Sleep -Milliseconds 200   # Dragged 이벤트 처리 대기
  [DD]::GetWindowRect($h, [ref]$r) | Out-Null
  $positions += "($($r.L),$($r.T))"
}
[DD]::mouse_event([DD]::LUP, 0, 0, 0, [IntPtr]::Zero); Start-Sleep -Milliseconds 300
[DD]::GetWindowRect($h, [ref]$r) | Out-Null
Write-Host "단계별 창 위치: $($positions -join ' → ')"
Write-Host "최종: ($($r.L),$($r.T))  이동량: ($($r.L - ($gx-280)), ...)"
$moved = ($r.L -ne ($gx - 280))
Write-Host "이동 여부: $(if($moved){'움직임 ✓'}else{'안 움직임 ✗'})"
Get-Process -Id $p.Id -ErrorAction SilentlyContinue | Stop-Process -Force
