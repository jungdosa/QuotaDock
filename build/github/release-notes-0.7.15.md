QuotaDock 0.7.15 — 설치 중 언어 선택 제거.

**QuotaDock** is a tiny always-on-top Windows desktop widget that monitors your
**Claude, OpenAI Codex, and Google Antigravity (Gemini)** usage limits — session/weekly
quotas and reset timers — in one glance. Built with Go + Fyne. No telemetry, no
credential handling: it reuses your already-signed-in official CLIs/IDE locally.

## Changed

**설치 프로그램이 더 이상 언어를 묻지 않습니다.** 이전에는 설치를 시작하면 언어 선택
대화상자가 먼저 떴는데, 앱 본체는 Windows 표시 언어를 자동으로 따르면서 설치기만 묻는 것이
일관되지 않았습니다. 이제 설치기도 OS 언어를 그대로 따라갑니다.

The installer no longer asks which language to use. It now follows your Windows display
language automatically, matching how the app itself behaves.

## Files

| File | Purpose |
|---|---|
| `QuotaDock-0.7.15-win-x64-Setup.exe` | Installer (Start Menu, optional launch at startup) |
| `QuotaDock-0.7.15-win-x64-portable.exe` | Portable single executable |
| `SHA256SUMS.txt` | Checksums |

Binaries are unsigned — verify with `SHA256SUMS.txt`. Windows 10 22H2+ / 11, x64.
