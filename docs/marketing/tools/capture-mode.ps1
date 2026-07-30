param(
    [Parameter(Mandatory)][string]$Mode,
    [Parameter(Mandatory)][string]$OutPng,
    [int]$WaitSeconds = 30
)

$ErrorActionPreference = 'Stop'
$settingsPath = "$env:APPDATA\QuotaDock\settings.json"
$exe = "C:\Users\jungd\AppData\Local\Programs\QuotaDock\quotadock.exe"

# Ensure app is not running
Get-Process quotadock -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Milliseconds 800

# Configure settings for capture
$s = Get-Content $settingsPath -Raw | ConvertFrom-Json
$s.language = 'en'
$s.theme = 'dark'
$s.displayMode = $Mode
$s.windowX = 400
$s.windowY = 250
$s.windowPositioned = $true
$s | ConvertTo-Json -Depth 10 | Set-Content $settingsPath -Encoding UTF8

# DPI awareness + window rect P/Invoke
Add-Type -AssemblyName System.Drawing
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Win32Cap {
    [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
    [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr hWnd, out RECT rect);
    [StructLayout(LayoutKind.Sequential)]
    public struct RECT { public int Left, Top, Right, Bottom; }
}
"@
[Win32Cap]::SetProcessDPIAware() | Out-Null

# Launch and wait for window
$proc = Start-Process $exe -PassThru
$deadline = (Get-Date).AddSeconds(20)
while ((Get-Date) -lt $deadline) {
    $proc.Refresh()
    if ($proc.MainWindowHandle -ne 0) { break }
    Start-Sleep -Milliseconds 500
}
if ($proc.MainWindowHandle -eq 0) { throw "No window handle after 20s" }

# Let provider data load
Start-Sleep -Seconds $WaitSeconds

$proc.Refresh()
$rect = New-Object Win32Cap+RECT
[Win32Cap]::GetWindowRect($proc.MainWindowHandle, [ref]$rect) | Out-Null
$w = $rect.Right - $rect.Left
$h = $rect.Bottom - $rect.Top
if ($w -le 0 -or $h -le 0) { throw "Bad window rect: $($rect.Left),$($rect.Top),$w,$h" }

$bmp = New-Object System.Drawing.Bitmap($w, $h)
$gfx = [System.Drawing.Graphics]::FromImage($bmp)
$gfx.CopyFromScreen($rect.Left, $rect.Top, 0, 0, (New-Object System.Drawing.Size($w, $h)))
$bmp.Save($OutPng, [System.Drawing.Imaging.ImageFormat]::Png)
$gfx.Dispose(); $bmp.Dispose()

Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
"CAPTURED $Mode -> $OutPng (${w}x${h} at $($rect.Left),$($rect.Top))"
