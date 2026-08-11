QuotaDock 0.7.34 — 다중 모니터 크래시 수정과 전체화면 양보.

**QuotaDock** is a tiny always-on-top Windows desktop widget that monitors your
**Claude, OpenAI Codex, and Google Antigravity (Gemini)** usage limits — session/weekly
quotas and reset timers — in one glance. Built with Go + Fyne. No telemetry, no
credential handling: it reuses your already-signed-in official CLIs/IDE locally.

## Fixed

**모니터를 끄면 앱이 조용히 종료되던 문제를 고쳤습니다.** 원인은 GUI 라이브러리(Fyne
v2.8.0) 내부에서 사라진 모니터를 검사 없이 참조하던 버그였고, nil 검사를 넣은 패치로
빌드했습니다. 수정은 업스트림에도 제출했습니다
([fyne-io/fyne#6467](https://github.com/fyne-io/fyne/issues/6467)).

Turning a monitor off could silently kill the app: a nil dereference inside Fyne
v2.8.0's window-move callback when the window's monitor disappears. This build ships
with a patched Fyne; the fix has been submitted upstream
([fyne-io/fyne#6467](https://github.com/fyne-io/fyne/issues/6467)). Verified against
real monitor on/off cycles — the previous build crashed, this one survives.

**진단 로그가 Go 모듈 경로를 이메일 주소로 오인해 가리던 문제도 고쳤습니다.**
Diagnostics no longer redact Go module paths as if they were email addresses,
so crash stacks stay readable.

## Added

**전체화면 양보.** "항상 위"가 켜져 있어도, 전체화면 영상이나 게임이 위젯이 있는
모니터를 완전히 덮으면 위젯이 자동으로 그 뒤로 물러납니다. 전체화면이 끝나면 2초 안에
제자리로 돌아옵니다.

The widget now yields to fullscreen content. When a fullscreen video or game covers
the widget's monitor — foreground or not — the widget slips directly beneath it and
rejoins the topmost layer within two seconds of the fullscreen surface going away.
No setting needed; it is how "Always on top" now behaves.

**Codex 리셋 크레딧 개수 표시.** 계정에 rate-limit 리셋 크레딧이 있으면 크레딧 표시
옆에 개수가 나타납니다. 0개면 아무것도 표시하지 않습니다.

The Codex lane now shows how many rate-limit reset credits the account holds, next to
the existing credits text. Nothing changes when the count is zero, and the display
follows the existing Codex credits toggle.

## Files

| File | Purpose |
|---|---|
| `QuotaDock-0.7.34-win-x64-Setup.exe` | Installer (Start Menu, optional launch at startup) |
| `QuotaDock-0.7.34-win-x64-portable.exe` | Portable single executable |
| `SHA256SUMS.txt` | Checksums |

Binaries are unsigned — verify with `SHA256SUMS.txt`. Windows 10 22H2+ / 11, x64.
