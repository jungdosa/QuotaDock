# Security

QuotaDock reads usage limits from tools you are already signed in to, and it runs on your own
machine. This page explains what it touches, what it sends, and how to report a problem.

## Reporting a vulnerability

Use GitHub's private reporting form:
[**Report a vulnerability**](https://github.com/jungdosa/QuotaDock/security/advisories/new).
That keeps the report private until there is a fix.

Please do not open a public issue for anything exploitable. For ordinary bugs, a normal
[issue](https://github.com/jungdosa/QuotaDock/issues) is the right place.

This is a single-maintainer project, so treat any timeline as best effort. I will acknowledge
a report as soon as I see it and tell you what I plan to do about it.

## What QuotaDock does with credentials

It does not ask you for any. There is no login screen, no field to paste a session key, and
no cookie extraction.

What it actually does, per provider:

- **Claude.** Reads the OAuth credentials that Claude Code already stored, either from its
  local credentials file or from an environment override. When those file-based credentials
  are close to expiry, QuotaDock sends the refresh token to Anthropic's token endpoint and
  writes the refreshed values back to that same file atomically. It then sends the access
  token to Anthropic's usage endpoint to read your limits.
- **Codex.** Talks to the official Codex CLI app-server over stdio. QuotaDock does not handle
  the credentials itself.
- **Antigravity.** Reads from the IDE's language server on `127.0.0.1`, after verifying the
  process it is talking to.

Tokens, cookies, and raw credential-file contents are never drawn in the UI and never written
to the log. Only normalized usage rates, plan labels validated against an allowlist, and reset
times reach the interface.

## Where it connects

Outbound traffic is checked against an allowlist of exact hostnames over HTTPS. The list lives
in `internal/security/security.go`, which is the thing to read if you want to be sure rather
than take my word for it. At the time of writing it is:

| Host | Purpose |
|---|---|
| `api.anthropic.com` | Claude usage |
| `platform.claude.com` | Claude token refresh |
| `api.github.com` | Update check |
| `github.com` | Release asset download, which redirects to the two hosts below |
| `objects.githubusercontent.com` | Release asset download |
| `release-assets.githubusercontent.com` | Release asset download |

Update requests carry no credentials. Everything else stays on loopback. There is no
telemetry, no analytics, and no crash reporting.

A Grok provider is in development on `main` and adds `grok.com` to the same list. It is not
in any release yet, so it is not in the binary you downloaded.

## Local files

| Path | Contents |
|---|---|
| `%APPDATA%\QuotaDock\settings.json` | Your settings. Survives uninstall. Never synced. |
| `%LOCALAPPDATA%\QuotaDock\quotadock.log` | Diagnostic log, capped at 1 MB. Secrets and email addresses are redacted before writing. |
| `%LOCALAPPDATA%\QuotaDock\crash.log` | Written only after an abnormal exit. |

Neither log is transmitted anywhere. If you attach one to an issue, skim it first.

## Verifying a download

Release binaries are currently **unsigned**, so Windows SmartScreen will warn you the first
time you run one. That warning is expected and I would rather you verify the file than click
through it.

Every release ships `SHA256SUMS.txt`. In PowerShell:

```powershell
Get-FileHash .\QuotaDock-<version>-win-x64-portable.exe -Algorithm SHA256
```

Compare the result with the matching line in `SHA256SUMS.txt` from the same release.

If you would rather not run an installer at all, the portable executable is a single file
with nothing to uninstall.

Code signing is being pursued. Until it is in place, checksums and the public build script in
`build/windows/` are how you can check what you are running.

## Updates

QuotaDock checks GitHub Releases once at startup and whenever you ask it to. There is no
background polling and the check carries no credentials. A downloaded update is verified
against the release asset's SHA-256 twice: after the download completes, and again
immediately before the installer runs.

## Supported versions

Only the latest release gets fixes. There are no maintenance branches.
