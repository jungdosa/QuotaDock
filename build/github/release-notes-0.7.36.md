QuotaDock 0.7.36 — 세션 리셋 직후의 낡은 Claude 사용률 수정.

**QuotaDock** is a tiny always-on-top Windows desktop widget that monitors your
**Claude, OpenAI Codex, Google Antigravity (Gemini), and Grok** usage limits — session/weekly
quotas and reset timers — in one glance. Built with Go + Fyne. No telemetry, no
browser-cookie extraction: it reuses your already-signed-in official CLIs, or lets you
sign in through the provider's own page inside the app.

## Fixed

**Claude 세션(5시간) 사용률이 리셋 직후 낡은 값을 보일 수 있던 문제를 고쳤습니다.** 5시간
창이 리셋된 직후 Anthropic 응답의 레거시 필드가 일시적으로 이전 창의 값을 유지하는 경우가
있어, claude.ai는 새 창의 낮은 사용률을 보여주는데 위젯은 이전 값을 그대로 표시할 수
있었습니다. 이제 주간 사용률과 같은 규칙으로 응답의 `limits[]` 세션 항목을 우선 사용합니다.

Right after a 5-hour window rollover, the legacy `five_hour` field in Anthropic's usage
response can briefly keep reporting the previous window's value, so the widget could show
a stale session bar while claude.ai already showed the fresh one. The session lane now
prefers the `limits[]` session entry over the legacy field — the same rule the weekly
lane has always used — and falls back to the legacy field when no such entry exists.

## Files

| File | Purpose |
|---|---|
| `QuotaDock-0.7.36-win-x64-Setup.exe` | Installer (Start Menu, optional launch at startup) |
| `QuotaDock-0.7.36-win-x64-portable.exe` | Portable single executable |
| `SHA256SUMS.txt` | Checksums |

Binaries are unsigned — verify with `SHA256SUMS.txt`. Windows 10 22H2+ / 11, x64.
The in-app sign-in needs the Edge WebView2 runtime (present on Windows 11 by default);
without it the Auth method stays unavailable and the CLI paths are unaffected.
