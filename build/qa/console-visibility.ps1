# conhost 가 "보이는 창" 을 가지는지 확인 (안 보이면 문제 아님)
Add-Type @"
using System;
using System.Runtime.InteropServices;
using System.Collections.Generic;
public class Vis {
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumProc cb, IntPtr p);
  [DllImport("user32.dll")] public static extern int GetWindowThreadProcessId(IntPtr h, out int pid);
  [DllImport("user32.dll")] public static extern int GetWindowTextLength(IntPtr h);
  public delegate bool EnumProc(IntPtr h, IntPtr p);
  public static List<int> VisiblePids() {
    var pids = new List<int>();
    EnumWindows((h, p) => {
      if (IsWindowVisible(h) && GetWindowTextLength(h) > 0) {
        int pid; GetWindowThreadProcessId(h, out pid); pids.Add(pid);
      }
      return true;
    }, IntPtr.Zero);
    return pids;
  }
}
"@

Get-Process -Name qd-351,quotadock -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep 2
$p = Start-Process "C:\dev\qd-351.exe" -PassThru
$appPid = $p.Id
Start-Sleep 20

# 앱의 자식 conhost / codex PID 수집
$descendants = @{}
function Collect($parentId) {
  $kids = Get-CimInstance Win32_Process -Filter "ParentProcessId = $parentId" -ErrorAction SilentlyContinue
  foreach ($k in $kids) {
    $descendants[[int]$k.ProcessId] = $k.Name
    Collect $k.ProcessId
  }
}
Collect $appPid

Write-Host "앱 자손 프로세스:"
$descendants.GetEnumerator() | ForEach-Object { Write-Host "  $($_.Value) (PID $($_.Key))" }

# 보이는 창을 가진 PID 목록
$visiblePids = [Vis]::VisiblePids()
Write-Host "`n앱 자손 중 '보이는 창'을 가진 것:"
$foundVisible = $false
foreach ($d in $descendants.GetEnumerator()) {
  if ($visiblePids -contains $d.Key) {
    Write-Host "  ★ $($d.Value) (PID $($d.Key)) 가 보이는 창을 가짐 → 사용자에게 빈 창으로 보임"
    $foundVisible = $true
  }
}
if (-not $foundVisible) { Write-Host "  없음 → 자식 프로세스 콘솔 창이 화면에 안 보임 PASS" }

Get-Process -Name qd-351 -ErrorAction SilentlyContinue | Stop-Process -Force
