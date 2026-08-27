QuotaDock 0.7.37 — 조용한 비정상 종료에 증거를 남기는 계측.

**QuotaDock** is a tiny always-on-top Windows desktop widget that monitors your
**Claude, OpenAI Codex, Google Antigravity (Gemini), and Grok** usage limits — session/weekly
quotas and reset timers — in one glance. Built with Go + Fyne. No telemetry, no
browser-cookie extraction: it reuses your already-signed-in official CLIs, or lets you
sign in through the provider's own page inside the app.

## Added

**소리 없이 사라지는 종료에도 이제 증거가 남습니다.** GUI 앱은 표준 오류가 버려지기 때문에,
내부 스레드에서 터지는 일부 크래시는 지금까지 아무 기록 없이 프로세스만 사라졌습니다
(절전 해제 직후 모니터 재구성 국면에서 실제로 관측된 사례가 있습니다). 이제 그런 크래시의
전체 스택이 앱 데이터 폴더의 `fatal.log`에 남고, 다음 실행 때 `crash.log`로 자동 이관됩니다.
수집·전송은 없습니다 — 모든 기록은 이 PC의 로컬 파일에만 남습니다.

Some crashes happen on threads outside the app's own panic recovery — the Go runtime
prints those only to stderr, which a GUI-subsystem process discards, so the process could
vanish without a trace (observed once right after a sleep-wake display reshuffle). The
runtime's crash output is now routed to `fatal.log` in the local app-data folder; on the
next launch a non-empty dump is recorded into `crash.log` and the file is re-armed.
Everything stays on your machine — nothing is uploaded.

## Files

| File | Purpose |
|---|---|
| `QuotaDock-0.7.37-win-x64-Setup.exe` | Installer (Start Menu, optional launch at startup) |
| `QuotaDock-0.7.37-win-x64-portable.exe` | Portable single executable |
| `SHA256SUMS.txt` | Checksums |

Binaries are unsigned — verify with `SHA256SUMS.txt`. Windows 10 22H2+ / 11, x64.
The in-app sign-in needs the Edge WebView2 runtime (present on Windows 11 by default);
without it the Auth method stays unavailable and the CLI paths are unaffected.
