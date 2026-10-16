QuotaDock 0.7.77 — 계정 연결 안내와 Antigravity 사용량 표시를 개선했습니다.

**QuotaDock** is a Windows desktop widget for Claude, OpenAI Codex,
Google Antigravity (Gemini), and Grok usage limits and reset timers.
This release accumulates the work since 0.7.59.

## Added

- Claude 1번 계정에서 `Auth`를 선택하면 전용 브라우저 프로필을 사용합니다. 다른 Claude
  계정에 로그인해도 1번 계정의 세션을 덮지 않으며, 적용 가능한 기존 프로필은 이관합니다.
- Codex 계정마다 공유 CLI 로그인 또는 계정별 격리 브라우저 로그인을 선택할 수 있습니다.
  공유 CLI 로그인은 한 계정에서만 사용합니다.
- 첫 실행과 연결 설정에 설치·로그인·재연결 안내, 명령 복사, 다시 확인 동작을 추가했습니다.
- 10분 간격으로 CPU 사용 시간, 메모리, goroutine 수를 진단 로그에 기록합니다.

## Fixed

- 변경된 Antigravity IDE 언어 서버 설치 경로를 인식해 로컬 사용량 조회에 연결합니다.
- Antigravity CLI 캐시를 표시할 때 연결 상세에 실제 사용량 조회 시각을 보여줍니다.
- 모니터를 다시 연결하거나 디스플레이가 바뀐 뒤 창이 비어 보일 때 다시 그리도록 예약합니다.
- Windows 빌드에서 앱 버전 불일치와 설치기 누락을 실패로 처리합니다.

## Changed

- 연결 설정 화면의 폭과 배치를 다듬고 컨트롤과 안내 문구를 더 촘촘하게 배치했습니다.

## Files

| File | Purpose |
|---|---|
| `QuotaDock-0.7.77-win-x64-portable.exe` | Portable executable |
| `QuotaDock-0.7.77-win-x64-Setup.exe` | Installer |
| `SHA256SUMS.txt` | Checksums |

The Windows Package Manager catalog receives new versions after review:
`winget install jungdosa.QuotaDock`.

Windows 10 22H2+ / 11, x64. Binaries are unsigned; verify downloads against
`SHA256SUMS.txt`.
