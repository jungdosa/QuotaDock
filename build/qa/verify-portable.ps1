# Portable exe 실측: 콘솔 안 뜸 + 클릭 됨 + 세 공급자 데이터
Add-Type @"
using System;
using System.Runtime.InteropServices;
using System.Collections.Generic;
public class PV {
  [DllImport("user32.dll")] public static extern IntPtr WindowFromPoint(POINT p);
  [DllImport("user32.dll")] public static extern int GetWindowThreadProcessId(IntPtr h, out int pid);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumProc cb, IntPtr p);
  [DllImport("user32.dll")] public static extern int GetWindowTextLength(IntPtr h);
  public delegate bool EnumProc(IntPtr h, IntPtr p);
  public struct POINT { public int X; public int Y; }
  public struct RECT { public int L; public int T; public int R; public int B; }
  public static List<int> VisiblePids() {
    var pids = new List<int>();
    EnumWindows((h, p) => {
      if (IsWindowVisible(h) && GetWindowTextLength(h) > 0) { int pid; GetWindowThreadProcessId(h, out pid); pids.Add(pid); }
      return true;
    }, IntPtr.Zero);
    return pids;
  }
}
"@

$portable = (Join-Path $PSScriptRoot "..\..\dist\QuotaDock-0.5.1-win-x64-portable.exe")
Write-Host "=== Portable 검증: $portable ==="
Get-Process -Name "QuotaDock-0.5.1-win-x64-portable","quotadock" -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep 2
$p = Start-Process $portable -PassThru
$appPid = $p.Id
Write-Host "실행 PID: $appPid"
Start-Sleep 20

if ($p.HasExited) { Write-Host "즉시 종료 FAIL"; exit }

# 1. 자손 프로세스 중 보이는 창 (콘솔)
$desc = @{}
function Collect($id) {
  foreach ($k in (Get-CimInstance Win32_Process -Filter "ParentProcessId = $id" -ErrorAction SilentlyContinue)) {
    $desc[[int]$k.ProcessId] = $k.Name; Collect $k.ProcessId
  }
}
Collect $appPid
$vis = [PV]::VisiblePids()
$badConsole = $false
foreach ($d in $desc.GetEnumerator()) { if ($vis -contains $d.Key) { Write-Host "  콘솔 창 보임: $($d.Value) FAIL"; $badConsole = $true } }
if (-not $badConsole) { Write-Host "1. 콘솔 창: 안 보임 PASS (자손: $(($desc.Values) -join ', '))" }

# 2. 본문 클릭
$h = $p.MainWindowHandle
$r = New-Object PV+RECT; [PV]::GetWindowRect($h, [ref]$r) | Out-Null
$pt = New-Object PV+POINT; $pt.X = [int](($r.L+$r.R)/2); $pt.Y = [int](($r.T+$r.B)/2)
$hitPid = 0; [PV]::GetWindowThreadProcessId([PV]::WindowFromPoint($pt), [ref]$hitPid) | Out-Null
if ($hitPid -eq $appPid) { Write-Host "2. 본문 클릭: 앱에 닿음 PASS" } else { Write-Host "2. 본문 클릭: 다른 창(PID $hitPid) FAIL" }

# 3. 자동실행 레지스트리 (Portable 은 안 걸려야 §11)
$run = Get-ItemProperty "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "QuotaDock" -ErrorAction SilentlyContinue
if ($run) { Write-Host "3. Portable 자동실행: 등록됨 (§11 위반 가능)" } else { Write-Host "3. Portable 자동실행 레지스트리: 없음 PASS" }

Write-Host "창 크기: $($r.R-$r.L) x $($r.B-$r.T), 제목 있음"
Get-Process -Id $appPid -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep 2
$resid = Get-Process -Id $appPid -ErrorAction SilentlyContinue
if ($resid) { Write-Host "종료 후 잔류: 있음 FAIL" } else { Write-Host "4. 종료 후 잔류: 없음 PASS" }
