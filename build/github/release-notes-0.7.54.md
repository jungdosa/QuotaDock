QuotaDock 0.7.54 — 여러 모니터 사이를 옮길 때 창이 틀어지거나 뒤로 숨던 문제 수정.

**QuotaDock** is a tiny always-on-top Windows desktop widget that monitors your
**Claude, OpenAI Codex, Google Antigravity (Gemini), and Grok** usage limits — session and
weekly quotas with their reset timers — in one glance. Built with Go + Fyne. No telemetry:
it reads the credentials your official CLIs and IDE already hold, and only ever calls their
usage endpoints.

## Fixed

**배율이 다른 모니터로 옮겨도 창 모양이 틀어지지 않습니다.** 예를 들어 125%로 쓰는 4K
모니터와 100%인 1080p 모니터 사이를 오가면, 위쪽에 빈 띠가 생기고 아래가 잘리며 오른쪽으로
뒤의 창이 비쳐 보였습니다. 이제 옮긴 뒤 1초 안팎에 크기와 둥근 테두리가 제자리를 찾습니다.
Moving the widget between monitors with different display scaling no longer leaves it
misshapen — a blank band on top, the bottom cut off, windows behind showing through on the
right. It settles to the right size and outline about a second after it lands.

**옮겨 간 모니터에 이미 떠 있던 창 뒤로 숨지 않습니다.** 모니터를 가득 채운 창이 있는 곳에
위젯을 놓으면 2초쯤 뒤 그 창 아래로 내려가 버렸습니다. 제목 표시줄 없이 최대화된 창(가상 머신
화면 등)도 더는 전체화면으로 취급하지 않습니다. 위젯이 있는 모니터에서 영상이나 게임이 새로
전체화면이 되면 예전처럼 그 아래로 비켜 줍니다.
The widget no longer drops behind a window that already filled the monitor it was moved
to, and a maximized window without a title bar, such as a virtual machine console, no longer
counts as fullscreen. Video or a game that goes fullscreen over the widget still pushes it
aside, as before.

## Files

| File | Purpose |
|---|---|
| `QuotaDock-0.7.54-win-x64-portable.exe` | Portable single executable — no installation |
| `QuotaDock-0.7.54-win-x64-Setup.exe` | Installer (Start Menu, optional launch at startup) |
| `SHA256SUMS.txt` | Checksums |

Also available through the Windows Package Manager: `winget install jungdosa.QuotaDock`
(the catalog picks up new versions after review).

Binaries are unsigned, so SmartScreen may warn. Verify with `SHA256SUMS.txt`, or build from
source. Windows 10 22H2+ / 11, x64.
