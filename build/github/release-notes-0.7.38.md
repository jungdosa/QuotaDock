QuotaDock 0.7.38 — 하루 반쯤 지나면 조용히 사라지던 문제 수정.

**QuotaDock** is a tiny always-on-top Windows desktop widget that monitors your
**Claude, OpenAI Codex, Google Antigravity (Gemini), and Grok** usage limits — session/weekly
quotas and reset timers — in one glance. Built with Go + Fyne. No telemetry, no
browser-cookie extraction: it reuses your already-signed-in official CLIs, or lets you
sign in through the provider's own page inside the app.

## Fixed

**오래 켜 두면 아무 흔적 없이 종료되던 문제를 고쳤습니다.** 화면 배치를 확인하는 코드가
1분마다 시스템 콜백을 새로 등록했는데, 이 등록은 해제되지 않고 프로세스당 개수 상한이
있습니다. 그래서 **깨어 있는 시간으로 약 33시간이 쌓이면** 상한에 걸려 프로세스가 즉시
사라졌습니다. 절전으로 시계가 멈추는 만큼 수명이 늘어나 "절전에서 깨어난 직후 죽는" 것처럼
보였지만, 실제로는 사용 시간이 누적된 결과였습니다. 이제 콜백을 한 번만 등록합니다.

The work-area probe registered a new system callback on every call, and those
registrations are permanent and capped per process. With the 60-second refresh calling it
once a minute, the cap was reached after roughly 33 hours of awake time and the process
died instantly with no crash dialog — sleep pauses the clock, which made it look like a
wake-up crash. The callback is now registered once for the life of the process. Diagnosed
from the crash stack that 0.7.37's fatal-log capture recorded.

## Files

| File | Purpose |
|---|---|
| `QuotaDock-0.7.38-win-x64-Setup.exe` | Installer (Start Menu, optional launch at startup) |
| `QuotaDock-0.7.38-win-x64-portable.exe` | Portable single executable |
| `SHA256SUMS.txt` | Checksums |

Binaries are unsigned — verify with `SHA256SUMS.txt`. Windows 10 22H2+ / 11, x64.
The in-app sign-in needs the Edge WebView2 runtime (present on Windows 11 by default);
without it the Auth method stays unavailable and the CLI paths are unaffected.
