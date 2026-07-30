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

# 1. 저장소 설명 (영문 위주 — GitHub 검색 노출)
& $gh repo edit $repo --description "Windows desktop widget that monitors Claude, OpenAI Codex, and Google Antigravity (Gemini) usage limits, quotas, and reset timers. Go + Fyne, no telemetry."

# 2. 토픽(검색 키워드) 등록
& $gh repo edit $repo --add-topic claude --add-topic claude-code --add-topic openai --add-topic codex `
  --add-topic gemini --add-topic antigravity --add-topic usage-monitor --add-topic rate-limit `
  --add-topic quota --add-topic ai-tools --add-topic desktop-widget --add-topic system-tray `
  --add-topic windows --add-topic golang --add-topic fyne

# 3. 릴리스 생성 + 산출물 업로드 (산출물이 없으면 먼저 build-release.ps1 실행)
& $gh release create "v$Version" `
  (Join-Path $dist "QuotaDock-$Version-win-x64-Setup.exe") `
  (Join-Path $dist "QuotaDock-$Version-win-x64-portable.exe") `
  (Join-Path $dist "SHA256SUMS.txt") `
  --repo $repo --title "QuotaDock $Version" --notes-file $notes --latest

Write-Host "`n완료. 공개 전환은 아래 한 줄 (또는 GitHub Settings > Danger Zone):"
Write-Host "  gh repo edit $repo --visibility public --accept-visibility-change-consequences"
