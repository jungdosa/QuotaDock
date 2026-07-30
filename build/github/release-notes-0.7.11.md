QuotaDock 0.7.11 — 업데이트 다운로드 수정.

**QuotaDock** is a tiny always-on-top Windows desktop widget that monitors your
**Claude, OpenAI Codex, and Google Antigravity (Gemini)** usage limits — session/weekly
quotas and reset timers — in one glance. Built with Go + Fyne. No telemetry, no
credential handling: it reuses your already-signed-in official CLIs/IDE locally.

## Fixed

- **0.7.10의 업데이트 다운로드가 동작하지 않던 문제를 고쳤습니다.** 릴리스 자산의
  다운로드 주소는 `github.com`이 직접 서빙하는데 허용 호스트 목록에서 빠져 있어,
  새 버전을 찾아도 내려받기 단계에서 거부됐습니다.
  0.7.10을 쓰고 계시면 이 릴리스를 직접 내려받아 설치해 주세요.

허용 목록은 여전히 정확한 호스트 일치입니다 — `github.com`, `api.github.com`,
그리고 리다이렉트 목적지인 두 `githubusercontent.com` 호스트만 허용되며
하위 도메인은 거부됩니다. 업데이트 요청에는 인증 정보가 실리지 않습니다.

실제 GitHub 응답을 고정한 회귀 테스트를 추가해 같은 부류의 문제가 다시 생기지
않도록 했습니다.

## 0.7.8 이후 변경 요약

- 연결 방식 선택 버튼 — Claude `CLI`·`Auth`·`기타`, Codex `CLI`, Antigravity `IDE`.
  상태를 색으로 구분하고, CLI 미설치 시 카드 안에서 설치 안내가 펼쳐집니다
- 업데이트 확인·다운로드·설치 — 시작 시 1회와 버튼으로 확인하고,
  SHA-256을 다운로드 직후와 설치 직전 두 번 검증합니다. 주기적 폴링은 없습니다

## Files

| File | Purpose |
|---|---|
| `QuotaDock-0.7.11-win-x64-Setup.exe` | Installer (Start Menu, optional launch at startup) |
| `QuotaDock-0.7.11-win-x64-portable.exe` | Portable single executable |
| `SHA256SUMS.txt` | Checksums |

Binaries are unsigned — verify with `SHA256SUMS.txt`. Windows 10 22H2+ / 11, x64.
