# QuotaDock Windows 릴리스 빌드 스크립트
# 계획서 §13 산출물: Setup.exe, portable.exe, SHA256SUMS.txt, BUILD-REPORT.md
# 사용법: build-release.ps1 -Version 0.5.0

param(
    [Parameter(Mandatory=$true)][string]$Version,
    # Resolved from whichever go is on PATH. The repo carries no machine
    # specific toolchain path: a hardcoded one silently builds with the wrong
    # Go the moment the toolchain moves, and it moved once already.
    [string]$GoRoot = $(& go env GOROOT 2>$null),
    [string]$MinGW  = "$env:LOCALAPPDATA\Microsoft\WinGet\Packages\BrechtSanders.WinLibs.POSIX.UCRT_Microsoft.Winget.Source_8wekyb3d8bbwe\mingw64\bin",
    [string]$ISCC   = "$env:LOCALAPPDATA\Programs\Inno Setup 6\ISCC.exe"
)

$ErrorActionPreference = "Stop"
if (-not $GoRoot -or -not (Test-Path (Join-Path $GoRoot "bin\go.exe"))) {
    throw "Go 툴체인을 찾지 못했습니다. go 를 PATH 에 두거나 -GoRoot 로 지정하십시오."
}
$repo = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
$dist = Join-Path $repo "dist"
$env:GOROOT = $GoRoot
$env:CGO_ENABLED = "1"
$env:Path = "$GoRoot\bin;$MinGW;$env:Path"

Write-Host "=== QuotaDock $Version 릴리스 빌드 ===" -ForegroundColor Cyan
Set-Location $repo

# 0. 사전 검증
Write-Host "[0/5] 테스트 실행..."
$test = go test ./... 2>&1
if ($LASTEXITCODE -ne 0) { $test | Select-Object -Last 20; throw "테스트 실패 — 릴리스 중단" }
Write-Host "  테스트 통과" -ForegroundColor Green

# 1. dist 준비
if (Test-Path $dist) { Remove-Item $dist -Recurse -Force }
New-Item -ItemType Directory -Path $dist | Out-Null

# 2. 실행 파일 빌드 (GUI 서브시스템, 버전 주입)
Write-Host "[1/5] 실행 파일 빌드..."
# exe 아이콘 리소스(.syso) 생성 — 저장소에 커밋하지 않고 빌드 시 생성 (§23.3)
$rsrc = Join-Path (go env GOPATH) "bin\rsrc.exe"
if (-not (Test-Path $rsrc)) {
    Write-Host "  rsrc 설치..." -ForegroundColor DarkGray
    go install github.com/akavel/rsrc@latest 2>&1 | Out-Null
}
$syso = Join-Path $repo "cmd\quotadock\rsrc_windows_amd64.syso"
& $rsrc -ico (Join-Path $repo "assets\icon.ico") -arch amd64 -o $syso
if ($LASTEXITCODE -ne 0) { throw "아이콘 리소스 생성 실패" }

$ldflags = "-H windowsgui -X main.version=$Version -s -w"
go build -trimpath -ldflags $ldflags -o "$dist\quotadock.exe" ./cmd/quotadock
$buildExit = $LASTEXITCODE
Remove-Item $syso -ErrorAction SilentlyContinue  # 저장소 오염 방지
if ($buildExit -ne 0) { throw "빌드 실패" }
$exeSize = [math]::Round((Get-Item "$dist\quotadock.exe").Length/1MB, 1)
Write-Host "  quotadock.exe ($exeSize MB)" -ForegroundColor Green

# 3. Portable 산출물 (실행 파일 복사 + 마커)
Write-Host "[2/5] Portable 생성..."
$portable = "$dist\QuotaDock-$Version-win-x64-portable.exe"
Copy-Item "$dist\quotadock.exe" $portable
# Portable 마커: 자동 실행 제한 신호 (§11)
"portable" | Out-File "$dist\.portable" -Encoding ascii
Write-Host "  $(Split-Path $portable -Leaf)" -ForegroundColor Green

# 4. Setup.exe (Inno Setup)
Write-Host "[3/5] Setup.exe 생성..."
if (Test-Path $ISCC) {
    & $ISCC "/DAppVersion=$Version" "$PSScriptRoot\installer.iss" 2>&1 | Select-Object -Last 5
    if ($LASTEXITCODE -ne 0) { throw "Inno Setup 컴파일 실패" }
    Write-Host "  QuotaDock-$Version-win-x64-Setup.exe" -ForegroundColor Green
} else {
    Write-Host "  ISCC 미발견 ($ISCC) — Setup 건너뜀" -ForegroundColor Yellow
}

# 5. SHA-256 + 빌드 보고서
Write-Host "[4/5] SHA-256 계산..."
$artifacts = Get-ChildItem $dist -Filter "QuotaDock-*.exe"
$shaLines = foreach ($a in $artifacts) {
    $h = (Get-FileHash $a.FullName -Algorithm SHA256).Hash.ToLower()
    "$h  $($a.Name)"
}
$shaLines | Out-File "$dist\SHA256SUMS.txt" -Encoding ascii
Write-Host "  SHA256SUMS.txt" -ForegroundColor Green

Write-Host "[5/5] 빌드 보고서..."
$goVer = (go version) -replace "go version ", ""
$report = @"
# QuotaDock $Version 빌드 보고서

- 빌드 시각: (스크립트 실행 시점)
- Go: $goVer
- CGO: 활성 (MinGW-w64)
- 서명: NotSigned (인증서 없음 — §13에 따라 실패로 위장하지 않음)

## 산출물

$($artifacts | ForEach-Object { "- $($_.Name) ($([math]::Round($_.Length/1MB,1)) MB)" } | Out-String)

## SHA-256

``````
$($shaLines -join "`n")
``````

## 검증 필요 (수동)

- 같은 버전 재설치 / 이전 버전 위 업그레이드
- 설치 위치 선택, 시작 메뉴, 선택적 바탕화면 바로가기, 제거 프로그램
- 제거 시 %APPDATA%\QuotaDock 설정 유지
- 설치형 자동 실행 경로 갱신, Portable 이동 후 자동 실행 제한
- 실행 스모크: 세 화면, 종료 후 자식 프로세스/트레이 잔류 없음
"@
$report | Out-File "$dist\BUILD-REPORT.md" -Encoding utf8
Remove-Item "$dist\.portable" -ErrorAction SilentlyContinue

Write-Host "`n=== 완료: $dist ===" -ForegroundColor Cyan
Get-ChildItem $dist | Select-Object Name, @{n='MB';e={[math]::Round($_.Length/1MB,2)}} | Format-Table -AutoSize
