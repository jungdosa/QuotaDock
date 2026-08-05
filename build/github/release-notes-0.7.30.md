QuotaDock 0.7.30 — 트레이에서 창을 열면 비어 보이던 문제 수정.

**QuotaDock** is a tiny always-on-top Windows desktop widget that monitors your
**Claude, OpenAI Codex, and Google Antigravity (Gemini)** usage limits — session/weekly
quotas and reset timers — in one glance. Built with Go + Fyne. No telemetry, no
credential handling: it reuses your already-signed-in official CLIs/IDE locally.

## Fixed

**부팅 후 처음 창을 열면 내용이 하나도 안 그려지던 문제를 고쳤습니다.** 시작 프로그램으로
등록해 두면 QuotaDock은 트레이에 숨은 채로 시작합니다. 그 상태에서 바탕화면이나 작업
표시줄 아이콘을 눌러 창을 열면, 테두리만 있고 안이 텅 빈 창이 뜨는 경우가 있었습니다.
앱을 껐다 켜야만 정상으로 돌아왔습니다.

원인은 창을 살리는 경로가 그리기 엔진을 거치지 않는 데 있었습니다. 이제 창이 화면에
나타나는 순간을 직접 감지해 상태를 맞추고, 그래도 비어 있으면 스스로 다시 그립니다.

Opening the window from the tray after a hidden start could show an empty frame that only
a restart would fix. The activation path bypassed the drawing engine; the app now detects
the moment the window appears, syncs, and repaints itself if anything is still blank.

## Files

| File | Purpose |
|---|---|
| `QuotaDock-0.7.30-win-x64-Setup.exe` | Installer (Start Menu, optional launch at startup) |
| `QuotaDock-0.7.30-win-x64-portable.exe` | Portable single executable |
| `SHA256SUMS.txt` | Checksums |

Binaries are unsigned — verify with `SHA256SUMS.txt`. Windows 10 22H2+ / 11, x64.
