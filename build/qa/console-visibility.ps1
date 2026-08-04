param(
    # 검증 대상 실행 파일. 기본값은 dist 의 최신 portable 빌드다.
    [string]$Exe = (Get-ChildItem (Join-Path $PSScriptRoot "..\..\dist\*-portable.exe") -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending | Select-Object -First 1).FullName,
    # 캡처 산출물 위치. 저장소 밖 임시 폴더에 남긴다.
    [string]$OutDir = (Join-Path $env:TEMP "quotadock-qa")
)
if (-not $Exe) { throw "검증 대상 exe 를 찾지 못했습니다. dist 를 빌드하거나 -Exe 로 지정하십시오." }
if (-not (Test-Path $OutDir)) { New-Item -ItemType Directory -Path $OutDir | Out-Null }
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

Get-Process -Name ([IO.Path]::GetFileNameWithoutExtension($Exe)),quotadock -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep 2
$p = Start-Process $Exe -PassThru
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

Get-Process -Name ([IO.Path]::GetFileNameWithoutExtension($Exe)) -ErrorAction SilentlyContinue | Stop-Process -Force
