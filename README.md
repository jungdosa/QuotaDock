# QuotaDock

[English](./README.md) | [한국어](./README.ko-KR.md) | [简体中文](./README.zh-CN.md) | [繁體中文（臺灣）](./README.zh-TW.md) | [日本語](./README.ja-JP.md)

QuotaDock is a tiny always-on-top Windows desktop widget that watches your **Claude, OpenAI
Codex, and Google Antigravity** usage limits — session/weekly quotas and reset timers — in one
glance.

[![Release](https://img.shields.io/github/v/release/jungdosa/QuotaDock?include_prereleases&label=release)](https://github.com/jungdosa/QuotaDock/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Fyne](https://img.shields.io/badge/GUI-Fyne-orange)](https://fyne.io/)
[![Platform](https://img.shields.io/badge/platform-Windows%2010%2F11-0078D4?logo=windows&logoColor=white)](#supported-platforms)

<p align="center">
  <img src="docs/marketing/quotadock-normal-en.png" alt="QuotaDock normal view — Claude / Codex / Antigravity usage and reset timers" width="539">
</p>

When you work with Claude Code, Codex CLI, and Antigravity IDE, you keep asking the same
questions: *how much of the 5-hour session is left? when does the weekly limit reset?*
QuotaDock answers both without you having to check.

| Provider | What it shows | How it connects |
|---|---|---|
| **Claude** (Claude Code) | 5-hour session · 7-day weekly · Fable weekly | Reuses your official Claude CLI credentials |
| **OpenAI Codex** | Session · weekly limits | Official Codex CLI app-server (stdio JSONL) |
| **Google Antigravity** | Gemini and Claude/GPT group session · weekly | Local language server (loopback) |

If one provider fails, the others keep working. Every row draws a **segmented bar (usage)**
above a **thin continuous bar (time until reset)**, so you can see at a glance whether your
quota is burning faster than the clock. Bars and figures switch to warning colors once your
thresholds are crossed.

## Views

Three widget views plus a settings screen. The toolbar button cycles
`normal → compact → nano`, and a single click returns nano to compact.

### Normal — everything at once

Provider groups, plan badges, usage bars with reset bars, countdown and reset time.

<p align="center"><img src="docs/marketing/quotadock-normal-en.png" alt="Normal view (dark)" width="480"></p>

### Compact — one line per row

A narrow view with time-until-reset next to each percentage. Hover a time to get a tooltip
with the exact reset moment.

<p align="center"><img src="docs/marketing/quotadock-compact-en.png" alt="Compact view (dark)" width="312"></p>

### Nano — a tiny strip

One cell per provider, roughly 78px tall — made for parking in a corner of your monitor.
Hovering a row shows a three-line tooltip: `provider · window / remaining / reset time`.

<p align="center"><img src="docs/marketing/quotadock-nano-en.png" alt="Nano view (dark)" width="360"></p>

## Features

- **Light · dark · system themes**, 12/24-hour date formats, per-provider color palettes
  and configurable warning thresholds
- **Twelve languages** — English, Korean, German, French, Italian, Indonesian,
  Portuguese (Brazil), Spanish (Spain / Latin America), Japanese, Simplified Chinese,
  and Traditional Chinese (Taiwan). `System` follows your Windows
  display language
- **Connection method buttons** — each provider row shows how it connects (`CLI`, `Auth`,
  `IDE`) with its state. If a CLI is missing, clicking the button expands install guidance
  right inside the card
- **Built-in updates** — checks GitHub Releases once at startup and whenever you ask.
  Downloads are verified against the release asset's SHA-256 **twice**: after download and
  again immediately before the installer runs. No periodic polling, no credentials sent
- **Pin to taskbar (Windows 11)** — keeps the tray icon out of the overflow flyout

## Install

Grab it from [**Releases**](https://github.com/jungdosa/QuotaDock/releases).

| File | Purpose |
|---|---|
| `QuotaDock-<version>-win-x64-Setup.exe` | Installer — Start Menu entry, optional launch at startup |
| `QuotaDock-<version>-win-x64-portable.exe` | Single portable executable |

Binaries are unsigned, so SmartScreen may warn you. Verify integrity with `SHA256SUMS.txt`.
There is nothing to configure — QuotaDock discovers your already-signed-in official CLIs and
IDE on its own.

> Current version is `0.7.15`. It becomes `1.0.0` once Windows feature verification is done.

## First run

There is no setup wizard, no login screen, and nothing to paste.

1. Launch **QuotaDock**. It appears as a small always-on-top widget.
2. It looks for the official tools you already use and connects on its own —
   Claude Code CLI, Codex CLI, and the Antigravity IDE.
3. Providers you are signed in to start showing usage within a few seconds.

If a row reports a missing CLI, open **Settings → Connections** and click that provider's
`CLI` button. Install guidance expands inside the card — the command to run, how to sign in,
and where QuotaDock looks. Press `Rescan` when you are done; no restart needed.

Day to day:

- The toolbar button cycles `normal → compact → nano`; one click returns nano to compact.
- `✕` sends the widget to the tray instead of quitting. Quit from the tray menu.
- Drag the title bar to move it. The position is remembered across restarts.

## Privacy

QuotaDock is built so that it never sees your credentials in the first place.

- **It reads, it never asks.** Claude usage comes from the token Claude Code already stored on
  your machine. Codex usage comes from the official Codex CLI's app-server over stdio.
  Antigravity usage comes from its language server on `127.0.0.1`. There is no session-key
  box, no cookie extraction, and no browser automation.
- **Credentials never reach the surface.** Tokens, cookies, and raw credential files are not
  rendered in the UI. Only normalized usage percentages, allowlist-validated plan labels, and
  reset timestamps do.
- **No log file.** Nothing is written to disk as a log. Diagnostic output goes to standard
  error only, and secrets, bearer tokens, and email addresses are redacted first.
- **Outbound traffic is a short allowlist.** Provider requests may only reach
  `api.anthropic.com` and `platform.claude.com`. Update checks may only reach
  `api.github.com` and GitHub's release hosts, and they carry no credentials. Everything else
  is loopback. No telemetry, no analytics, no crash reporting.
- **Your settings stay yours.** Configuration lives in `%APPDATA%\QuotaDock\settings.json`
  and survives uninstall. Nothing is synced anywhere.
- **No billable requests.** Refreshing usage never spends your quota or credits.

## Design principles

- **It doesn't ask.** If the official tools are installed and signed in, it connects on its
  own. No first-run login wizard, no forced popups.
- **It doesn't handle secrets.** Tokens, cookies, and raw `auth.json` never reach the UI or
  the logs. Only normalized usage rates, allowlist-validated plan labels, and reset times do.
- **It stays local.** No telemetry, no external analytics. Loopback connections are allowed
  only to verified processes on fixed endpoints. The one outbound request is the update
  check, which carries no credentials and runs only at startup or when you click.
- **It doesn't spend credits.** Refreshing usage never sends a billable AI request.
- **It stays small.** Native Go + Fyne rendering, 0% idle CPU, actively managed memory.

## Supported platforms

| Priority | Platform | Status |
|---|---|---|
| 1st | Windows 10 22H2+ / 11, x64 | **Working** (0.7.x) |
| 2nd | macOS 14+, Apple Silicon | Planned |
| 3rd | Linux x64 → arm64 | Planned |

## Building from source

Fyne uses CGO, so Go alone is not enough.

- Go 1.26 or newer
- A C compiler (Windows: MinGW-w64 / macOS: Xcode Command Line Tools / Linux: gcc + X11 headers)

```sh
git clone https://github.com/jungdosa/QuotaDock.git
cd QuotaDock
go build ./cmd/quotadock
```

Release artifacts (Setup.exe, portable, SHA256SUMS) are produced by
`build/windows/build-release.ps1`.

## License

MIT — see [LICENSE](LICENSE).

QuotaDock's source, wording, screen design, and app icon are original work.
Remaining work is tracked in [docs/REMAINING-WORK.md](docs/REMAINING-WORK.md).

### Third-party assets

| Asset | Source | License |
|---|---|---|
| `assets/fonts/Pretendard-*.ttf` | [Pretendard](https://github.com/orioncactus/pretendard) | SIL OFL 1.1 — [`Pretendard-OFL.txt`](assets/fonts/Pretendard-OFL.txt) |
| `assets/providers/*.svg` | [lobe-icons](https://github.com/lobehub/lobe-icons) | MIT — [`LICENSE-lobe-icons.txt`](assets/providers/LICENSE-lobe-icons.txt) |

Provider logos are the trademarks of their respective owners (Anthropic, OpenAI, Google) and
are used solely to identify the services whose usage QuotaDock displays.

---

<sub>keywords: Claude usage monitor · Claude Code rate limit · OpenAI Codex quota · Gemini
Antigravity usage · AI quota tracker · desktop widget · system tray · Windows · Go · Fyne</sub>
