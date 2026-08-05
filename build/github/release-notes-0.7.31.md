QuotaDock 0.7.31 — 최소화 상태로 시작 옵션 추가.

**QuotaDock** is a tiny always-on-top Windows desktop widget that monitors your
**Claude, OpenAI Codex, and Google Antigravity (Gemini)** usage limits — session/weekly
quotas and reset timers — in one glance. Built with Go + Fyne. No telemetry, no
credential handling: it reuses your already-signed-in official CLIs/IDE locally.

## Added

**설정 › 일반 동작에 "최소화 상태로 시작" 항목이 생겼습니다. 기본값은 꺼짐입니다.**

지금까지는 Windows 시작 시 실행을 켜두면 **항상 트레이로만** 떴습니다. 시작 프로그램으로
등록해 둔 위젯이 부팅 후 화면에 안 보이면 실행이 됐는지조차 알 수 없었습니다. 이제 기본은
창이 보이는 상태로 시작하고, 트레이로 시작하고 싶으면 이 항목을 켜면 됩니다.

기존에 자동 시작을 쓰고 계셨다면 **업데이트 후 첫 실행에서 자동으로 정리됩니다.** 따로
하실 일은 없습니다.

Settings › General now has a **Start minimized** toggle, off by default. Launching at
Windows startup used to always go straight to the tray, which left no sign the app had
started at all. Existing autostart entries are reconciled automatically on first run.

## Files

| File | Purpose |
|---|---|
| `QuotaDock-0.7.31-win-x64-Setup.exe` | Installer (Start Menu, optional launch at startup) |
| `QuotaDock-0.7.31-win-x64-portable.exe` | Portable single executable |
| `SHA256SUMS.txt` | Checksums |

Binaries are unsigned — verify with `SHA256SUMS.txt`. Windows 10 22H2+ / 11, x64.
