QuotaDock 0.7.10 — 연결 방식 선택 버튼과 자동 업데이트.

**QuotaDock** is a tiny always-on-top Windows desktop widget that monitors your
**Claude, OpenAI Codex, and Google Antigravity (Gemini)** usage limits — session/weekly
quotas and reset timers — in one glance. Built with Go + Fyne. No telemetry, no
credential handling: it reuses your already-signed-in official CLIs/IDE locally.

Claude Code · Codex CLI · Antigravity IDE의 세션/주간 사용량과 리셋 타이머를 한 화면에서
보여주는 상시 표시 위젯입니다.

## What's new since 0.7.8

### 연결 방식 선택 버튼 (0.7.9)

- 설정 연결 카드의 각 공급자 행에 연결 방식 버튼이 생겼습니다
  (Claude: `CLI`·`Auth`·`기타` / Codex: `CLI` / Antigravity: `IDE`)
- 상태를 색으로 구분합니다 — 활성(초록), 사용 가능(파랑), 미설치(회색 점선), 준비 중(흐림)
- CLI가 없을 때 버튼을 누르면 카드 안에서 설치 안내가 펼쳐집니다
  (설치 명령, 로그인 방법, 탐색 경로, `다시 탐색`·`설치 문서 열기`)
- Claude `기타`는 `CLAUDE_CODE_OAUTH_TOKEN` 환경변수의 **존재 여부만** 확인하며
  토큰 값을 읽거나 표시하지 않습니다

### 업데이트 확인과 설치 (0.7.10)

- 앱 시작 시 1회와 설정의 `업데이트` 버튼으로 새 버전을 확인합니다
- 새 버전이 있으면 안내 후 다운로드 → **SHA-256 검증** → 설치까지 진행합니다
- 해시는 GitHub 릴리스 자산의 `digest` 값과 대조하며, 다운로드 직후와
  설치 실행 직전 **두 번** 검증합니다
- 주기적 폴링은 하지 않습니다. 시작 시 1회와 사용자 클릭 외에는 외부 통신이 없습니다
- 업데이트 요청에는 인증 헤더가 실리지 않으며, 전용 호스트 허용 목록을 씁니다
- 포터블 빌드는 자동 설치 대신 릴리스 페이지를 엽니다

## Files

| File | Purpose |
|---|---|
| `QuotaDock-0.7.10-win-x64-Setup.exe` | Installer (Start Menu, optional launch at startup) |
| `QuotaDock-0.7.10-win-x64-portable.exe` | Portable single executable |
| `SHA256SUMS.txt` | Checksums |

Binaries are unsigned — verify with `SHA256SUMS.txt`. Windows 10 22H2+ / 11, x64.
