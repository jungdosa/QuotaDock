# QuotaDock GitHub 공개 등록 스크립트
# 사전 조건: gh CLI 설치 + `gh auth login` 1회 (브라우저 인증)
# 사용법:   .\github-publish.ps1 [-Version 0.7.8]
param([string]$Version = "0.7.8")
$ErrorActionPreference = "Stop"
$repo = "jungdosa/QuotaDock"
$repoRoot = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
$dist = Join-Path $repoRoot "dist"
$notes = Join-Path $PSScriptRoot "release-notes-$Version.md"

# gh 탐색: PATH → winget 설치 경로
$gh = (Get-Command gh -ErrorAction SilentlyContinue).Source
if (-not $gh) { $gh = "$env:ProgramFiles\GitHub CLI\gh.exe" }
if (-not (Test-Path $gh)) { throw "gh CLI를 찾을 수 없습니다. winget install GitHub.cli 후 gh auth login 하세요." }

# $ErrorActionPreference="Stop"은 네이티브 exe의 실패를 잡지 못한다.
# gh 호출마다 종료 코드를 직접 확인하지 않으면 전부 실패해도 "완료"가 찍힌다.
function Invoke-Gh {
    param([string]$Step, [string[]]$GhArgs)
    & $gh @GhArgs
    if ($LASTEXITCODE -ne 0) { throw "$Step 실패 (gh 종료 코드 $LASTEXITCODE)" }
}

# 0. 사전 검증 — 인증 · 릴리스 노트 · 산출물
& $gh auth status 2>&1 | Out-Null
if ($LASTEXITCODE -ne 0) { throw "gh 인증이 없습니다. 먼저 gh auth login 을 실행하세요." }

if (-not (Test-Path $notes)) { throw "릴리스 노트를 찾을 수 없습니다: $notes" }

$assets = @(
    (Join-Path $dist "QuotaDock-$Version-win-x64-Setup.exe"),
    (Join-Path $dist "QuotaDock-$Version-win-x64-portable.exe"),
    (Join-Path $dist "SHA256SUMS.txt")
)
$missing = $assets | Where-Object { -not (Test-Path $_) }
if ($missing) { throw "산출물 누락 — build-release.ps1 -Version $Version 을 먼저 실행하세요:`n  $($missing -join "`n  ")" }

# 산출물이 SHA256SUMS.txt 기재값과 일치하는지 확인 (재빌드 후 갱신 누락 방지)
$sums = Get-Content (Join-Path $dist "SHA256SUMS.txt")
foreach ($line in $sums) {
    if ($line -notmatch '^\s*([0-9a-fA-F]{64})\s+(\S+)\s*$') { continue }
    $expected, $name = $Matches[1].ToLower(), $Matches[2]
    $target = Join-Path $dist $name
    if (-not (Test-Path $target)) { throw "SHA256SUMS.txt가 없는 파일을 가리킵니다: $name" }
    $actual = (Get-FileHash $target -Algorithm SHA256).Hash.ToLower()
    if ($actual -ne $expected) { throw "해시 불일치 — $name`n  기대 $expected`n  실제 $actual" }
}
Write-Host "사전 검증 통과 (인증 · 노트 · 산출물 $($assets.Count)개 · 해시)" -ForegroundColor Green

# 1. 저장소 설명 (영문 위주 — GitHub 검색 노출)
Invoke-Gh "저장소 설명 등록" @("repo", "edit", $repo, "--description",
  "Windows desktop widget that monitors Claude, OpenAI Codex, and Google Antigravity (Gemini) usage limits, quotas, and reset timers. Go + Fyne, no telemetry.")

# 2. 토픽(검색 키워드) 등록
$topics = @("claude", "claude-code", "openai", "codex", "gemini", "antigravity",
            "usage-monitor", "rate-limit", "quota", "ai-tools", "desktop-widget",
            "system-tray", "windows", "golang", "fyne")
Invoke-Gh "토픽 등록" (@("repo", "edit", $repo) + ($topics | ForEach-Object { "--add-topic", $_ }))

# 3. 릴리스 생성 + 산출물 업로드
Invoke-Gh "릴리스 생성" (@("release", "create", "v$Version") + $assets +
  @("--repo", $repo, "--title", "QuotaDock $Version", "--notes-file", $notes, "--latest"))

Write-Host "`n완료. 공개 전환은 아래 한 줄 (또는 GitHub Settings > Danger Zone):"
Write-Host "  gh repo edit $repo --visibility public --accept-visibility-change-consequences"
