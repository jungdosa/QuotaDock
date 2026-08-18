QuotaDock 0.7.35 — Grok 레인과 앱 내 로그인.

**QuotaDock** is a tiny always-on-top Windows desktop widget that monitors your
**Claude, OpenAI Codex, Google Antigravity (Gemini), and Grok** usage limits — session/weekly
quotas and reset timers — in one glance. Built with Go + Fyne. No telemetry, no
browser-cookie extraction: it reuses your already-signed-in official CLIs, or lets you
sign in through the provider's own page inside the app.

## Added

**Grok을 네 번째 공급자로 지원합니다.** Grok CLI에 로그인되어 있으면 주간 사용률과 리셋
시각이 다른 공급자와 같은 문법으로 표시됩니다. 기본은 꺼져 있으니 설정 › 공급자 표시에서
**Grok 표시**를 켜세요. 노멀·컴팩트·나노 모든 모드와 연결 탭·트레이 툴팁에 나타납니다.

Grok joins as the fourth provider, with the same meter-and-reset grammar as the others.
The weekly percentage comes from the account's own billing endpoint, using the credential
the Grok CLI already stored — QuotaDock never rotates it. Off by default: turn on
**Show Grok** in Settings › Provider display.

**앱 안에서 Claude에 로그인할 수 있습니다.** Claude Code CLI가 없어도 연결 탭의 **Auth**를
눌러 로그인하면 사용량이 표시됩니다. 창에는 Anthropic 공식 로그인 페이지가 그대로 열리고,
비밀번호는 Anthropic으로 직접 전송됩니다 — QuotaDock은 읽을 수 없습니다. 세션은 이 앱
전용 폴더에만 저장되며, 브라우저의 쿠키는 건드리지 않습니다.

Claude can now be connected without the CLI. The **Auth** method opens Anthropic's own
sign-in page in a window inside the app; the password goes straight to Anthropic and the
app has no hook into the form. The session is stored only in QuotaDock's own browser
profile — your browser's cookies are never read. An installed CLI always keeps priority;
the in-app session serves the lane only when the CLI cannot.

**공급자별로 연결 방식을 고를 수 있습니다.** 연결 탭에서 사용 가능한 방식을 누르면 그
방식이 활성으로 표시됩니다.

Each provider now remembers which sign-in route you picked. Existing settings are
unaffected: without a stored choice, everything behaves exactly as before.

## Changed

**크레딧을 살 수 있는 계정에만 크레딧 줄이 표시됩니다.** 구매한 적이 없는 계정은 잔액 0이
와서 `크레딧 0`이라는 빈 정보가 떴는데, 이제 표시하지 않습니다.

The credits line now appears only when it carries information. An account that never
bought credits reports a zero balance, and the old surface still wrote "Credits 0".

## Files

| File | Purpose |
|---|---|
| `QuotaDock-0.7.35-win-x64-Setup.exe` | Installer (Start Menu, optional launch at startup) |
| `QuotaDock-0.7.35-win-x64-portable.exe` | Portable single executable |
| `SHA256SUMS.txt` | Checksums |

Binaries are unsigned — verify with `SHA256SUMS.txt`. Windows 10 22H2+ / 11, x64.
The in-app sign-in needs the Edge WebView2 runtime (present on Windows 11 by default);
without it the Auth method stays unavailable and the CLI paths are unaffected.
