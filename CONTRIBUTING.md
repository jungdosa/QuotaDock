# Contributing

Thanks for looking. QuotaDock is a small project maintained by one person, so the most useful
things you can send are usually bug reports and questions rather than large patches.

## Reporting a bug

Open an [issue](https://github.com/jungdosa/QuotaDock/issues). What helps most:

- Your Windows version (10 22H2, 11, etc.)
- Which providers you have signed in to, and which one misbehaves
- What the row showed versus what you expected
- Whether it happens every time or only sometimes

If the app crashed or the window came up blank, there may be something useful in
`%LOCALAPPDATA%\QuotaDock\quotadock.log` or `crash.log` next to it. Secrets and email
addresses are redacted before those files are written, but have a look before you attach one.

For anything security-sensitive, use the private report form described in
[SECURITY.md](SECURITY.md) instead of a public issue.

## Building

Fyne uses CGO, so Go on its own is not enough.

- Go 1.26 or newer
- A C compiler. MinGW-w64 on Windows, Xcode Command Line Tools on macOS.

```sh
git clone https://github.com/jungdosa/QuotaDock.git
cd QuotaDock
go build ./cmd/quotadock
```

Before sending a change:

```sh
go test ./...
go vet ./...
```

Release artifacts come from `build/windows/build-release.ps1`. Please do not run it as part
of ordinary work.

## Sending a change

Open an issue first if the change is more than a few lines. It saves you writing something I
was already planning differently.

A few things worth knowing before you start:

- **Windows is the supported platform.** macOS is planned; Linux is not. Windows-specific code
  sits behind build tags, and the fallbacks for other platforms need to stay intact.
- **`cmd/quotadock` is assembly and entry point only.** Provider, security, settings and UI
  logic belong in their existing `internal/` packages.
- **Never introduce cookie extraction, session-token entry, or web scraping.** Usage has to
  come from the official local interfaces. Reading usage must not cost the user any quota or
  credits.
- **Touching a UI string means touching every locale.** All twelve files under
  `internal/i18n/locales/` plus the completeness test.
- **Changing the settings schema means checking backward compatibility** against existing
  user settings, with safe defaults.

## Commit messages

English, please, and Conventional Commits style:

```
feat: add a start-minimized option
fix: repaint the window when it comes up blank
docs: describe how to verify a download
```

`feat`, `fix`, `docs`, `refactor`, `test`, `chore`. One logical change per commit. Explain in
the body why the change was needed if it is not obvious from the subject.

## Translations

Twelve locales live in `internal/i18n/locales/`. Corrections from native speakers are very
welcome, especially where a string reads like it was translated rather than written. Keep
placeholders intact and try to match the length of the original, since several strings sit in
tight layouts.
