QuotaDock 0.7.59 — 요금제 등급이 실제 구독과 맞고, 모니터를 옮길 때 창이 일그러져 보이지 않습니다.

**QuotaDock** is a Windows desktop widget for Claude, OpenAI Codex,
Google Antigravity (Gemini), and Grok usage limits and reset timers.

## Fixed

**Claude 요금제 등급.** 로그인한 뒤 요금제를 올린 계정이 예전 등급(예: MAX 20X 구독인데 MAX 5X)으로
보이던 문제를 고쳤습니다. 이제 Anthropic 프로필에서 6시간마다 현재 등급을 읽습니다.

**Claude plan tier.** An account upgraded after signing in no longer shows its old plan (for
example MAX 5X on a MAX 20X subscription). The tier now comes from Anthropic's profile endpoint
every six hours, falling back to the value saved at sign-in.

- Crossing onto a monitor with different display scaling hides the widget for the instant it
  resizes, instead of showing it grow, shrink and draw several outlines. It always reappears
  within 0.6 seconds.
- The Antigravity CLI report runs in the background, so a slow run no longer stalls the
  refresh; the language server fills in until the report arrives.
- Reset times in the normal window line up at the same distance from every meter.
- Credit balances show at most two decimal places, in a slightly larger font.

## Files

| File | Purpose |
|---|---|
| `QuotaDock-0.7.59-win-x64-portable.exe` | Portable executable |
| `QuotaDock-0.7.59-win-x64-Setup.exe` | Installer |
| `SHA256SUMS.txt` | Checksums |

The Windows Package Manager catalog receives new versions after review:
`winget install jungdosa.QuotaDock`.

Windows 10 22H2+ / 11, x64. Binaries are unsigned; verify downloads against
`SHA256SUMS.txt`.
