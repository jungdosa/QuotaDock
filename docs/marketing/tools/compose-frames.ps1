$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing
$sp = Join-Path $env:TEMP "quotadock-frames"
New-Item -ItemType Directory -Force $sp | Out-Null

$W = 720; $H = 640
$bg      = [System.Drawing.Color]::FromArgb(9, 14, 24)
$white   = [System.Drawing.Color]::FromArgb(238, 243, 250)
$gray    = [System.Drawing.Color]::FromArgb(148, 163, 184)
$accent  = [System.Drawing.Color]::FromArgb(255, 171, 76)
$border  = [System.Drawing.Color]::FromArgb(46, 60, 84)

function New-Frame([string]$outPath, [scriptblock]$draw) {
    $bmp = New-Object System.Drawing.Bitmap($W, $H)
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.SmoothingMode = 'AntiAlias'
    $g.TextRenderingHint = 'ClearTypeGridFit'
    $g.Clear($bg)
    & $draw $g
    $g.Dispose()
    $bmp.Save($outPath, [System.Drawing.Imaging.ImageFormat]::Png)
    $bmp.Dispose()
}

function Draw-CenteredText($g, [string]$text, [System.Drawing.Font]$font, [System.Drawing.Color]$color, [float]$y) {
    $brush = New-Object System.Drawing.SolidBrush($color)
    $size = $g.MeasureString($text, $font)
    $g.DrawString($text, $font, $brush, [float](($W - $size.Width) / 2), $y)
    $brush.Dispose()
}

function Draw-Shot($g, [string]$imgPath, [float]$top) {
    $img = [System.Drawing.Image]::FromFile($imgPath)
    $x = [float](($W - $img.Width) / 2)
    # subtle border to lift the widget off the background
    $pen = New-Object System.Drawing.Pen($border, 1)
    $g.DrawRectangle($pen, $x - 1, $top - 1, $img.Width + 1, $img.Height + 1)
    $pen.Dispose()
    $g.DrawImage($img, $x, $top, $img.Width, $img.Height)
    $img.Dispose()
}

$fTitle    = New-Object System.Drawing.Font("Segoe UI", 44, [System.Drawing.FontStyle]::Bold)
$fSub      = New-Object System.Drawing.Font("Segoe UI", 17, [System.Drawing.FontStyle]::Regular)
$fSmall    = New-Object System.Drawing.Font("Segoe UI", 12.5, [System.Drawing.FontStyle]::Regular)
$fMode     = New-Object System.Drawing.Font("Segoe UI", 24, [System.Drawing.FontStyle]::Bold)
$fModeSub  = New-Object System.Drawing.Font("Segoe UI", 14, [System.Drawing.FontStyle]::Regular)

# Frame 1: title card
New-Frame "$sp\frame-1.png" {
    param($g)
    Draw-CenteredText $g "QuotaDock" $fTitle $white 200
    Draw-CenteredText $g "Claude · Codex · Antigravity limits in one widget" $fSub $gray 300
    Draw-CenteredText $g "Free · Open source · No telemetry" $fSmall $accent 350
}

# Frame 2: normal (539x546)
New-Frame "$sp\frame-2.png" {
    param($g)
    Draw-CenteredText $g "Normal" $fMode $white 14
    Draw-CenteredText $g "Every window, reset time, and countdown" $fModeSub $gray 58
    Draw-Shot $g "$sp\shot-normal.png" 90
}

# Frame 3: compact (312x272)
New-Frame "$sp\frame-3.png" {
    param($g)
    Draw-CenteredText $g "Compact" $fMode $white 14
    Draw-CenteredText $g "All providers at a glance" $fModeSub $gray 58
    Draw-Shot $g "$sp\shot-compact.png" 180
}

# Frame 4: nano (360x78)
New-Frame "$sp\frame-4.png" {
    param($g)
    Draw-CenteredText $g "Nano" $fMode $white 14
    Draw-CenteredText $g "One slim strip on your desktop" $fModeSub $gray 58
    Draw-Shot $g "$sp\shot-nano.png" 270
}

"frames done"
