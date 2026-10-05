QuotaDock 0.7.56 — Antigravity 사용량을 IDE 없이도 표시하고, 배율이 다른 모니터로 드래그할 때 창이 깨지지 않습니다.

**QuotaDock** is a Windows desktop widget for Claude, OpenAI Codex,
Google Antigravity (Gemini), and Grok usage limits and reset timers.

## Added

**Antigravity CLI 우선 조회.** 로그인된 Antigravity CLI(`agy` 1.1.11 이상)의 읽기 전용
사용량 보고를 먼저 사용하므로 IDE를 켜지 않아도 사용량이 보입니다. CLI가 없거나 로그인돼
있지 않으면 이전처럼 IDE 언어 서버에서 읽고, 5분 뒤 CLI를 다시 시도합니다. 이 보고는
에이전트 턴을 시작하지 않고 쿼터를 쓰지 않으며, 읽기 전용임을 확인할 수 없는 보고가 오면
그 세션 동안 CLI 경로를 다시 쓰지 않습니다.

**Antigravity usage from the CLI first.** QuotaDock now reads the signed-in Antigravity
CLI's read-only usage report (`agy` 1.1.11 or later), so usage shows without the IDE open.
When the CLI is missing or signed out, it reads the IDE's language server as before and
tries the CLI again five minutes later. The report starts no agent turn and spends no quota;
a report that cannot be verified as read-only stops the CLI path for the rest of the session.

## Fixed

- Dragging onto a monitor with different display scaling resizes the widget the moment it
  crosses, keeping the spot you grabbed under the cursor, instead of showing a stretched,
  cut-off frame until release. On release it settles at once.
- An Antigravity limit that does not currently apply (the Claude/GPT five-hour limit while
  the weekly one is used up) no longer shows as a usage reading.
- Claude waits at least five minutes after a rate limit, doubling up to an hour. A
  token-service outage no longer reads as "sign in again".
- Additional Claude accounts signed in through the built-in browser no longer show an empty
  connected lane on rate limits or server errors, or "sign in again" on a Cloudflare check.
- A Grok week with no usage yet shows 0% instead of "–".

## Known issues

- While Antigravity usage comes from the CLI, the plan badge is not shown.

## Files

| File | Purpose |
|---|---|
| `QuotaDock-0.7.56-win-x64-portable.exe` | Portable executable |
| `QuotaDock-0.7.56-win-x64-Setup.exe` | Installer |
| `SHA256SUMS.txt` | Checksums |

The Windows Package Manager catalog receives new versions after review:
`winget install jungdosa.QuotaDock`.

Windows 10 22H2+ / 11, x64. Binaries are unsigned; verify downloads against
`SHA256SUMS.txt`.
