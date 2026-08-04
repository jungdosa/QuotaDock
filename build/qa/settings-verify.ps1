param(
    # 검증 대상 실행 파일. 기본값은 dist 의 최신 portable 빌드다.
    [string]$Exe = (Get-ChildItem (Join-Path $PSScriptRoot "..\..\dist\*-portable.exe") -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending | Select-Object -First 1).FullName,
    # 캡처 산출물 위치. 저장소 밖 임시 폴더에 남긴다.
    [string]$OutDir = (Join-Path $env:TEMP "quotadock-qa")
)
if (-not $Exe) { throw "검증 대상 exe 를 찾지 못했습니다. dist 를 빌드하거나 -Exe 로 지정하십시오." }
if (-not (Test-Path $OutDir)) { New-Item -ItemType Directory -Path $OutDir | Out-Null }
# 각 설정 변경 → 앱 실행 → 화면 캡처로 반영 확인 (일반 위젯)
Add-Type -AssemblyName System.Drawing
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class SV {
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr hdc, uint f);
  [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr h, IntPtr a, int x, int y, int cx, int cy, uint fl);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  public struct RECT { public int L; public int T; public int R; public int B; }
  public static readonly IntPtr TOP = new IntPtr(-1);
}
"@
$cfg = "$env:APPDATA\QuotaDock\settings.json"
$exe = $Exe

function CaptureWith($changes, $outFile) {
  $j = Get-Content $cfg -Raw | ConvertFrom-Json
  foreach ($k in $changes.Keys) { $j.$k = $changes[$k] }
  $j | ConvertTo-Json -Depth 5 | Out-File $cfg -Encoding utf8
  Get-Process -Name ([IO.Path]::GetFileNameWithoutExtension($Exe)) -ErrorAction SilentlyContinue | Stop-Process -Force
  Start-Sleep 2
  $p = Start-Process $exe -PassThru
  Start-Sleep 16
  $h = $p.MainWindowHandle
  [SV]::SetWindowPos($h, [SV]::TOP, 1400, 60, 0, 0, 0x0001) | Out-Null
  Start-Sleep 2
  $r = New-Object SV+RECT; [SV]::GetWindowRect($h, [ref]$r) | Out-Null
  $w = $r.R - $r.L; $ht = $r.B - $r.T
  $bmp = New-Object System.Drawing.Bitmap $w, $ht
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $hdc = $g.GetHdc(); [SV]::PrintWindow($h, $hdc, 2) | Out-Null; $g.ReleaseHdc($hdc); $g.Dispose()
  $bmp.Save($outFile, [System.Drawing.Imaging.ImageFormat]::Png); $bmp.Dispose()
  Get-Process -Id $p.Id -ErrorAction SilentlyContinue | Stop-Process -Force
  Start-Sleep 1
  # 저장 확인
  $saved = Get-Content $cfg -Raw | ConvertFrom-Json
  $ok = $true
  foreach ($k in $changes.Keys) { if ("$($saved.$k)" -ne "$($changes[$k])") { $ok = $false } }
  return $ok
}

# 원본 백업
Copy-Item $cfg "$cfg.svbak" -Force

Write-Host "=== A. 표시 방법: 사용량 → 남은량 ==="
$okA = CaptureWith @{ usageMode = "remaining"; language = "en" } (Join-Path $OutDir 'sv-remaining.png')
Write-Host "  저장 유지: $okA"

Write-Host "=== B. 표시 방법: 남은량 → 사용량 ==="
$okB = CaptureWith @{ usageMode = "used"; language = "en" } (Join-Path $OutDir 'sv-used.png')
Write-Host "  저장 유지: $okB"

Write-Host "=== C. 테마: Light ==="
$okC = CaptureWith @{ theme = "light"; language = "en" } (Join-Path $OutDir 'sv-light.png')
Write-Host "  저장 유지: $okC"

Write-Host "=== D. Claude 색상: blue → red ==="
$j = Get-Content $cfg -Raw | ConvertFrom-Json
$j.providerColors.claude = "red"; $j.theme = "dark"; $j.usageMode = "used"
$j | ConvertTo-Json -Depth 5 | Out-File $cfg -Encoding utf8
$okD = CaptureWith @{ language = "en" } (Join-Path $OutDir 'sv-claude-red.png')
$saved = Get-Content $cfg -Raw | ConvertFrom-Json
Write-Host "  Claude 색상 저장값: $($saved.providerColors.claude)"

# 원복
Copy-Item "$cfg.svbak" $cfg -Force; Remove-Item "$cfg.svbak" -Force
Write-Host "`n설정 원복 완료"
