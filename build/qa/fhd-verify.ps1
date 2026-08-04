# QuotaDock 표준 검수 도구 — FHD 모니터에서 실측 (잠금 시에도 마우스 주입 가능)
# 사용: fhd-verify.ps1 <exe경로> [normal|settings|compact]
# 핵심 발견: 4K 주모니터는 잠금 시 마우스 주입 차단되나, FHD 보조모니터는 항상 가능.
param(
    # 검증 대상 실행 파일. 기본값은 dist 의 최신 portable 빌드다.
    [string]$Exe = (Get-ChildItem (Join-Path $PSScriptRoot "..\..\dist\*-portable.exe") -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending | Select-Object -First 1).FullName,
    # 캡처 산출물 위치. 저장소 밖 임시 폴더에 남긴다.
    [string]$OutDir = (Join-Path $env:TEMP "quotadock-qa"),
    [ValidateSet("normal", "settings", "compact")][string]$Screen = "normal"
)
if (-not $Exe) { throw "검증 대상 exe 를 찾지 못했습니다. dist 를 빌드하거나 -Exe 로 지정하십시오." }
if (-not (Test-Path $OutDir)) { New-Item -ItemType Directory -Path $OutDir | Out-Null }

Add-Type -AssemblyName System.Drawing
Add-Type @"
using System;using System.Runtime.InteropServices;
public class FHD {
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr hdc, uint f);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool ClientToScreen(IntPtr h, ref PT p);
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int x, int y);
  [DllImport("user32.dll")] public static extern bool GetCursorPos(out PT p);
  [DllImport("user32.dll")] public static extern void mouse_event(uint f, int x, int y, uint d, IntPtr e);
  [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr h, IntPtr a, int x, int y, int cx, int cy, uint fl);
  [DllImport("user32.dll")] public static extern IntPtr WindowFromPoint(PT p);
  [DllImport("user32.dll")] public static extern int GetWindowThreadProcessId(IntPtr h, out int pid);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, IntPtr p);
  [DllImport("user32.dll")] public static extern bool AttachThreadInput(uint a, uint b, bool f);
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
  [DllImport("kernel32.dll")] public static extern uint GetCurrentThreadId();
  public struct RECT { public int L; public int T; public int R; public int B; }
  public struct PT { public int X; public int Y; }
  public const uint LDOWN=0x0002, LUP=0x0004;
  public static void Focus(IntPtr h){ uint fg=GetWindowThreadProcessId(h,IntPtr.Zero);uint me=GetCurrentThreadId();AttachThreadInput(me,fg,true);SetForegroundWindow(h);AttachThreadInput(me,fg,false); }
}
"@

# FHD 보조 모니터 목표 위치 (사용자 환경: 모니터2 = 1920~3840, 2160~3240)
$FHD_X = 2300; $FHD_Y = 2350

# 마우스 주입 가능 확인
[FHD]::SetCursorPos($FHD_X, $FHD_Y)|Out-Null; Start-Sleep -Milliseconds 200
$c=New-Object FHD+PT;[FHD]::GetCursorPos([ref]$c)|Out-Null
if([Math]::Abs($c.X-$FHD_X) -gt 5){ Write-Host "FHD 마우스 주입 차단 — 좌표 확인 필요 (커서 $($c.X),$($c.Y))"; }
else { Write-Host "FHD 마우스 주입 OK" }

$name = [System.IO.Path]::GetFileNameWithoutExtension($Exe)
Get-Process -Name $name -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep 2
$p = Start-Process $Exe -PassThru
Start-Sleep 16
$h = $p.MainWindowHandle
[FHD]::SetWindowPos($h, [IntPtr]::Zero, $FHD_X, $FHD_Y, 0, 0, 0x0005)|Out-Null  # NOSIZE|NOZORDER
Start-Sleep 2
$r = New-Object FHD+RECT; [FHD]::GetWindowRect($h, [ref]$r)|Out-Null
$w=$r.R-$r.L; $ht=$r.B-$r.T
Write-Host "창: ($($r.L),$($r.T)) $w x $ht"

# 오프셋
$ctl=New-Object FHD+PT;$ctl.X=0;$ctl.Y=0;[FHD]::ClientToScreen($h,[ref]$ctl)|Out-Null
Write-Host "오프셋: $($ctl.Y - $r.T)px"

# 캡처
$bmp=New-Object System.Drawing.Bitmap $w,$ht
$g=[System.Drawing.Graphics]::FromImage($bmp)
$hdc=$g.GetHdc();[FHD]::PrintWindow($h,$hdc,2)|Out-Null;$g.ReleaseHdc($hdc);$g.Dispose()
$out = (Join-Path $OutDir "fhd-$Screen.png")
$bmp.Save($out,[System.Drawing.Imaging.ImageFormat]::Png);$bmp.Dispose()
Write-Host "캡처: $out"

# 버튼 클릭 (X)
[FHD]::Focus($h);Start-Sleep -Milliseconds 300
$xs=$r.R-22; $ty=$ctl.Y+19
[FHD]::SetCursorPos($xs,$ty)|Out-Null;Start-Sleep -Milliseconds 200
[FHD]::mouse_event([FHD]::LDOWN,0,0,0,[IntPtr]::Zero);Start-Sleep -Milliseconds 80
[FHD]::mouse_event([FHD]::LUP,0,0,0,[IntPtr]::Zero);Start-Sleep -Milliseconds 800
Write-Host "X버튼 클릭 → 닫힘: $(if($p.HasExited){'예 (버튼 OK)'}else{'아니오 (무반응)'})"
Get-Process -Id $p.Id -ErrorAction SilentlyContinue | Stop-Process -Force 2>$null
