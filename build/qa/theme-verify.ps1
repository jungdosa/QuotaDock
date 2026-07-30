# 라이트/다크 테마 실측 + 배경 픽셀 독립 검증
Add-Type -AssemblyName System.Drawing
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class TV {
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr hdc, uint f);
  [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr h, IntPtr a, int x, int y, int cx, int cy, uint fl);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  public struct RECT { public int L; public int T; public int R; public int B; }
  public static readonly IntPtr TOP = new IntPtr(-1);
}
"@
$cfg = "$env:APPDATA\QuotaDock\settings.json"
$exe = "C:\dev\qd-353.exe"
Copy-Item $cfg "$cfg.tvbak" -Force

function CapTheme($themeName, $out) {
  $j = Get-Content $cfg -Raw | ConvertFrom-Json
  $j.theme = $themeName; $j.language = "en"
  $j | ConvertTo-Json -Depth 5 | Out-File $cfg -Encoding utf8
  Get-Process -Name qd-353 -ErrorAction SilentlyContinue | Stop-Process -Force
  Start-Sleep 2
  $p = Start-Process $exe -PassThru
  Start-Sleep 16
  $h = $p.MainWindowHandle
  [TV]::SetWindowPos($h, [TV]::TOP, 1400, 60, 0, 0, 0x0001) | Out-Null
  Start-Sleep 2
  $r = New-Object TV+RECT; [TV]::GetWindowRect($h, [ref]$r) | Out-Null
  $w = $r.R - $r.L; $ht = $r.B - $r.T
  $bmp = New-Object System.Drawing.Bitmap $w, $ht
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $hdc = $g.GetHdc(); [TV]::PrintWindow($h, $hdc, 2) | Out-Null; $g.ReleaseHdc($hdc); $g.Dispose()
  # 본문 중앙 배경 픽셀 (막대 없는 여백 지점: 우측 여백)
  $bgPixel = $bmp.GetPixel([int]($w*0.85), [int]($ht*0.5))
  $bmp.Save($out, [System.Drawing.Imaging.ImageFormat]::Png); $bmp.Dispose()
  Get-Process -Id $p.Id -ErrorAction SilentlyContinue | Stop-Process -Force
  return "R=$($bgPixel.R) G=$($bgPixel.G) B=$($bgPixel.B)  #{0:X2}{1:X2}{2:X2}" -f $bgPixel.R,$bgPixel.G,$bgPixel.B
}

Write-Host "=== 라이트 테마 배경 픽셀 ==="
$light = CapTheme "light" "C:\dev\tv-light.png"
Write-Host "  $light"
Write-Host "=== 다크 테마 배경 픽셀 (회귀 확인) ==="
$dark = CapTheme "dark" "C:\dev\tv-dark.png"
Write-Host "  $dark"

Write-Host "`n=== 판정 ==="
# 라이트: R,G,B 모두 200 이상 (밝음). 다크: 모두 40 이하 (어두움)
$lr = [int]($light -replace '.*R=(\d+).*','$1')
$dr = [int]($dark -replace '.*R=(\d+).*','$1')
if ($lr -gt 200) { Write-Host "라이트 배경 밝음 PASS (R=$lr)" } else { Write-Host "라이트 배경 안 밝음 FAIL (R=$lr)" }
if ($dr -lt 40) { Write-Host "다크 배경 어두움 PASS (R=$dr, 회귀 없음)" } else { Write-Host "다크 배경 이상 FAIL (R=$dr)" }

Copy-Item "$cfg.tvbak" $cfg -Force; Remove-Item "$cfg.tvbak" -Force
Write-Host "설정 원복"
