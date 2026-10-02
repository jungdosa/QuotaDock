QuotaDock 0.7.55 — Codex 계정을 최대 5개까지 함께 확인할 수 있습니다.

**QuotaDock** is a Windows desktop widget for Claude, OpenAI Codex,
Google Antigravity (Gemini), and Grok usage limits and reset timers.

## Added

**Codex 다중 계정.** 설정 → 연결에서 마지막 Codex 카드의 `+`를 누르고
브라우저에서 로그인하세요. 첫 계정은 기존 Codex CLI 로그인을 사용하고,
추가 계정은 로그인 정보를 각각 별도로 보관합니다. 최대 5개 계정을 표시하며,
계정마다 표시명과 색상을 지정할 수 있습니다.

**Up to five Codex accounts.** In Settings → Connections, use `+` on the last
Codex card and sign in through the browser. The first account uses the existing
Codex CLI login; added accounts keep their sign-in data separately. Each account
has its own display name and colour.

- Normal, compact, nano and tray views show each account's usage independently.
  General and Spark limits stay separate within every account.
- Connection settings scroll when additional Codex accounts are shown.
- A pending sign-in can be cancelled. Conflicting actions are disabled during
  sign-in, and delayed refresh results cannot restore usage from a previous login.
- Existing settings migrate automatically. Removing an account card retains its
  saved sign-in data so the card can be added back later.

## Files

| File | Purpose |
|---|---|
| `QuotaDock-0.7.55-win-x64-portable.exe` | Portable executable |
| `QuotaDock-0.7.55-win-x64-Setup.exe` | Installer |
| `SHA256SUMS.txt` | Checksums |

The Windows Package Manager catalog receives new versions after review:
`winget install jungdosa.QuotaDock`.

Windows 10 22H2+ / 11, x64. Binaries are unsigned; verify downloads against
`SHA256SUMS.txt`.
