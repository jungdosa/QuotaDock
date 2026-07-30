# 드래그 궤적 떨림 정량 측정
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class JT {
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int x, int y);
  [DllImport("user32.dll")] public static extern void mouse_event(uint f, int x, int y, uint d, IntPtr e);
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
  [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
  public struct RECT { public int L; public int T; public int R; public int B; }
  public const uint LDOWN=0x0002, LUP=0x0004;
}
"@

Get-Process -Name qd-352,qd-351,quotadock -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep 2
$p = Start-Process "C:\dev\qd-352.exe" -PassThru
Start-Sleep 18
$h = $p.MainWindowHandle

$fg = [JT]::GetForegroundWindow()
if ($fg -eq 0) { Write-Host "화면 잠김 (GetForegroundWindow=0) — 측정 불가"; Get-Process -Id $p.Id | Stop-Process -Force; exit }
Write-Host "화면 활성 (fg=$fg)"

$r = New-Object JT+RECT; [JT]::GetWindowRect($h, [ref]$r) | Out-Null
$startL = $r.L; $startT = $r.T
Write-Host "시작 위치: ($startL, $startT)"

$gx = $r.L + 250; $gy = $r.T + 16
[JT]::SetForegroundWindow($h) | Out-Null; Start-Sleep -Milliseconds 300
[JT]::SetCursorPos($gx, $gy); Start-Sleep -Milliseconds 200
[JT]::mouse_event([JT]::LDOWN, 0, 0, 0, [IntPtr]::Zero); Start-Sleep -Milliseconds 200

# 20단계 이동하며 매 단계 창 위치 기록
$trajX = @(); $trajY = @()
for ($i = 1; $i -le 20; $i++) {
  [JT]::SetCursorPos(($gx + $i*10), ($gy + $i*6))
  Start-Sleep -Milliseconds 50
  [JT]::GetWindowRect($h, [ref]$r) | Out-Null
  $trajX += $r.L; $trajY += $r.T
}
[JT]::mouse_event([JT]::LUP, 0, 0, 0, [IntPtr]::Zero); Start-Sleep -Milliseconds 400
[JT]::GetWindowRect($h, [ref]$r) | Out-Null

# 부호 뒤집힘 계산 (X 축): 창 위치 증가분의 부호가 바뀌는 횟수
function CountFlips($arr) {
  $flips = 0; $prevSign = 0
  for ($i = 1; $i -lt $arr.Count; $i++) {
    $d = $arr[$i] - $arr[$i-1]
    $sign = if ($d -gt 1) {1} elseif ($d -lt -1) {-1} else {0}
    if ($sign -ne 0 -and $prevSign -ne 0 -and $sign -ne $prevSign) { $flips++ }
    if ($sign -ne 0) { $prevSign = $sign }
  }
  return $flips
}

$flipsX = CountFlips $trajX
$flipsY = CountFlips $trajY
Write-Host "`nX 궤적: $($trajX -join ',')"
Write-Host "X 부호 뒤집힘: $flipsX 회"
Write-Host "Y 부호 뒤집힘: $flipsY 회"
Write-Host "최종 이동량: ($($r.L - $startL), $($r.T - $startT))  (커서는 +200,+120 이동)"
Write-Host ""
if ($flipsX -le 1 -and $flipsY -le 1) {
  Write-Host "판정: 부드러움 PASS (부호 뒤집힘 X$flipsX Y$flipsY)"
} else {
  Write-Host "판정: 떨림 감지 FAIL (부호 뒤집힘 X$flipsX Y$flipsY — 진동)"
}
Get-Process -Id $p.Id -ErrorAction SilentlyContinue | Stop-Process -Force
