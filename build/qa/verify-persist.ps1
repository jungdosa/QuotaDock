# 설정 저장/복원 실측: 값 변경 → 저장 확인 → 재시작 → 복원 확인
$cfg = "$env:APPDATA\QuotaDock\settings.json"
$exe = "C:\dev\qd-351.exe"

Write-Host "=== 현재 settings.json ==="
if (Test-Path $cfg) {
  $j = Get-Content $cfg -Raw | ConvertFrom-Json
  Write-Host "  language=$($j.language) theme=$($j.theme) alwaysOnTop=$($j.alwaysOnTop) refreshSeconds=$($j.refreshSeconds) compact=$($j.compact) showInTaskbar=$($j.showInTaskbar)"
  Write-Host "  warningPercent=$($j.warningPercent) dangerPercent=$($j.dangerPercent) usageMode=$($j.usageMode)"
  Write-Host "  windowX=$($j.windowX) windowY=$($j.windowY) windowPositioned=$($j.windowPositioned)"
} else { Write-Host "  파일 없음" }

Write-Host "`n=== 저장되는 필드 목록 (스키마 전체) ==="
if (Test-Path $cfg) {
  $j = Get-Content $cfg -Raw | ConvertFrom-Json
  $j.PSObject.Properties.Name | ForEach-Object { Write-Host "  - $_" }
}

Write-Host "`n=== 저장 동작 검증: 프로그램적으로 값 바꾸고 앱이 유지하는지 ==="
# 앱이 실행 중 설정을 저장하는 경로 = applyConfig → saveSettings
# 여기서는 파일 자체의 원자적 저장/복원을 검증
$backup = "$cfg.verifybak"
if (Test-Path $cfg) { Copy-Item $cfg $backup -Force }

# 테스트: 값 하나 바꿔 저장 후 앱 재시작해도 유지되는지
if (Test-Path $cfg) {
  $j = Get-Content $cfg -Raw | ConvertFrom-Json
  $origTheme = $j.theme
  $newTheme = if ($origTheme -eq "dark") { "light" } else { "dark" }
  $j.theme = $newTheme
  $j.refreshSeconds = 900
  $j | ConvertTo-Json | Out-File $cfg -Encoding utf8
  Write-Host "  변경: theme $origTheme→$newTheme, refreshSeconds→900"

  # 앱 실행 → 종료 (앱이 로드→저장 시 값 보존하는지)
  Get-Process -Name qd-351,quotadock -ErrorAction SilentlyContinue | Stop-Process -Force
  Start-Sleep 2
  $p = Start-Process $exe -PassThru
  Start-Sleep 12
  Get-Process -Id $p.Id -ErrorAction SilentlyContinue | Stop-Process -Force
  Start-Sleep 2

  $j2 = Get-Content $cfg -Raw | ConvertFrom-Json
  Write-Host "  재시작 후: theme=$($j2.theme) refreshSeconds=$($j2.refreshSeconds)"
  if ($j2.theme -eq $newTheme -and $j2.refreshSeconds -eq 900) {
    Write-Host "  설정 유지 PASS (앱이 값을 덮어쓰지 않음)"
  } else {
    Write-Host "  설정 유실 FAIL"
  }
  # 원복
  if (Test-Path $backup) { Copy-Item $backup $cfg -Force; Remove-Item $backup -Force }
}
