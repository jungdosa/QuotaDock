QuotaDock 0.7.12 — 트레이 아이콘을 작업 표시줄에 고정.

**QuotaDock** is a tiny always-on-top Windows desktop widget that monitors your
**Claude, OpenAI Codex, and Google Antigravity (Gemini)** usage limits — session/weekly
quotas and reset timers — in one glance. Built with Go + Fyne. No telemetry, no
credential handling: it reuses your already-signed-in official CLIs/IDE locally.

## What's new

### 작업 표시줄에 고정 (Windows 11)

Windows 11은 트레이 아이콘을 기본적으로 오버플로(∧) 안에 숨깁니다. 설정 →
**일반 동작**의 `작업 표시줄에 고정`을 켜면 QuotaDock 아이콘이 작업 표시줄에
항상 보입니다. 사용량을 확인하려고 오버플로를 여는 단계가 사라집니다.

- 권한 상승이 필요 없습니다 (사용자 레지스트리만 사용)
- Windows 10 이하에서는 토글이 표시되지 않습니다
- 이미 Windows 설정에서 손수 고정해 두셨다면, 이 설정을 꺼도 그 고정을
  되돌리지 않습니다

## Files

| File | Purpose |
|---|---|
| `QuotaDock-0.7.12-win-x64-Setup.exe` | Installer (Start Menu, optional launch at startup) |
| `QuotaDock-0.7.12-win-x64-portable.exe` | Portable single executable |
| `SHA256SUMS.txt` | Checksums |

Binaries are unsigned — verify with `SHA256SUMS.txt`. Windows 10 22H2+ / 11, x64.
