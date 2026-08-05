QuotaDock 0.7.32 — 설정 항목 배치 정리.

**QuotaDock** is a tiny always-on-top Windows desktop widget that monitors your
**Claude, OpenAI Codex, and Google Antigravity (Gemini)** usage limits — session/weekly
quotas and reset timers — in one glance. Built with Go + Fyne. No telemetry, no
credential handling: it reuses your already-signed-in official CLIs/IDE locally.

## Changed

**설정 › 일반 동작에서 "최소화 상태로 시작"을 "Windows 시작 시" 바로 아래로 옮겼습니다.**
이 항목은 시작 시 실행이 켜져 있을 때만 의미가 있는데, 사이에 무관한 항목이 끼어 있어
둘의 관계가 보이지 않았습니다. "항상 위"가 그 아래로 내려갑니다.

Settings › General now places **Start minimized** directly under **Start with Windows**,
since it only takes effect when that is on. **Always on top** moves down a row.

## Files

| File | Purpose |
|---|---|
| `QuotaDock-0.7.32-win-x64-Setup.exe` | Installer (Start Menu, optional launch at startup) |
| `QuotaDock-0.7.32-win-x64-portable.exe` | Portable single executable |
| `SHA256SUMS.txt` | Checksums |

Binaries are unsigned — verify with `SHA256SUMS.txt`. Windows 10 22H2+ / 11, x64.
