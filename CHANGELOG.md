# Changelog

Notable changes to QuotaDock. Every released version also has a
[GitHub release](https://github.com/jungdosa/QuotaDock/releases) carrying the same notes
plus checksums for its binaries.

Versions follow [Semantic Versioning](https://semver.org/). The project reaches 1.0.0 once
Windows feature verification is finished.

## [0.7.38] — 2026-08-29

### Fixed

- The widget no longer disappears without trace after a long run. Reading the desktop work
  area registered a fresh system callback on every call, and those registrations are
  permanent and capped per process. With the refresh cycle calling it once a minute, the cap
  was reached after roughly 33 hours of awake time and the process died on the spot — no
  dialog, no crash log, nothing. Sleep pauses that clock, which is why it looked like a
  wake-up crash rather than an uptime one. The callback is now registered once for the life
  of the process.

## [0.7.37] — 2026-08-28

### Added

- Silent shutdowns now leave evidence behind. A windowed program discards its standard error
  output, so a crash on a thread outside the app's own recovery took the process down with
  nothing written anywhere. Those stacks now land in `fatal.log` in the local app data
  folder and are folded into `crash.log` on the next launch. Nothing is uploaded — the files
  stay on your machine. This is what identified the fault fixed in 0.7.38.

## [0.7.36] — 2026-08-24

### Fixed

- The Claude session meter could show a stale figure just after its five-hour window reset.
  The older field in the usage response sometimes keeps reporting the previous window for a
  moment, so claude.ai showed the fresh, lower number while the widget still showed the old
  one. The session lane now reads the same per-limit entry the weekly lane has always
  preferred, and falls back to the older field only when that entry is absent.

## [0.7.35] — 2026-08-19

### Added

- Grok joins as the fourth provider, with the same meter-and-reset grammar as the others.
  The weekly percentage comes from the account's own billing endpoint, using the credential
  the Grok CLI already stored. Off by default: turn on **Show Grok** in Settings › Provider
  display.
- Claude can now be connected without the CLI. The **Auth** method opens Anthropic's own
  sign-in page in a window inside the app; the password goes straight to Anthropic and the
  app has no hook into the form. The session is stored only in QuotaDock's own browser
  profile — your browser's cookies are never read. An installed CLI always keeps priority;
  the in-app session serves the lane only when the CLI cannot.
- Each provider now remembers which sign-in route you picked. Existing settings are
  unaffected: without a stored choice, everything behaves exactly as before.

### Changed

- The credits line now appears only when it carries information. An account that never
  bought credits reports a zero balance, and the old surface still wrote "Credits 0".

## [0.7.34] — 2026-08-11

### Added

- QuotaDock steps aside for full-screen windows. When another program covers the whole
  monitor QuotaDock sits on, the widget drops below it and returns when the cover goes away.
  It does not wait for that window to have focus, so a paused video keeps its cover.
- The Codex lane counts reset credits alongside the balance.

### Fixed

- Switching a monitor off no longer takes the app down with it. The panic was in
  Fyne: a display that has just been freed still shows up in the monitor list, so
  reading its video mode returns nil and the dereference happens inside the event
  loop, where the app cannot catch it. Four crashes in three days here. Pinned to a
  Fyne fork carrying the missing nil checks until the fix is upstream.
- Crash logs no longer mangle Go module paths. The redactor treated
  "fyne.io/fyne/v2@v2.8.0" as an email address, so every stack frame came out as
  "[REDACTED_EMAIL]/internal/..." and traces were unreadable. Real addresses are
  still redacted.

## [0.7.32] — 2026-08-05

### Changed

- Moved "Start minimized" directly under "Start with Windows" in Settings › General. It only
  does anything when that option is on, and an unrelated row sat between them. "Always on
  top" shifts down a line.

## [0.7.31] — 2026-08-05

### Added

- A "Start minimized" toggle in Settings › General, off by default. Previously, enabling
  launch at Windows startup always sent the widget straight to the tray, so after a reboot
  there was no sign it had started at all. Now it opens visible unless you ask otherwise.
  Existing autostart entries get reconciled on first run; there is nothing to do by hand.

### Fixed

- Autostart is now detected by executable path instead of an exact match on the whole command
  string. Installations carrying an older Run entry used to look as though autostart had been
  turned off.

## [0.7.30] — 2026-08-05

### Fixed

- Opening the window from the tray could produce an empty frame. If QuotaDock was set to
  launch at startup it began hidden, and clicking the desktop or taskbar icon from that state
  sometimes gave you borders with nothing inside. Restarting the app was the only way out.
  The path that revived the window skipped the drawing engine. QuotaDock now watches for the
  moment the window actually becomes visible, reconciles its state then, and repaints if the
  surface is still blank.

## [0.7.26] — 2026-08-04

Ten internal builds since 0.7.15, released together.

### Added

- Claude credit balance in the lane header, with a per-provider toggle in Settings. No extra
  sign-in: the figure was already in the response the app receives.
- Japanese, Simplified Chinese and Traditional Chinese, bringing the total to twelve
  languages. Display language follows Windows, or you can pick one in Settings.
- A usage summary in the tray icon tooltip, so you can read current usage on hover without
  opening the window.

### Fixed

- Codex now recovers a dropped connection by itself rather than waiting for you to press
  Reconnect. A single transient failure gets one grace attempt, and states a reconnect cannot
  fix (CLI missing, outdated, or signed out) are left alone.
- The window comes back onto a visible work area when the monitor layout changes, such as
  after a display is switched off or a resolution changes.
- A blank window repaints itself. Rarely, usually after an unclean shutdown, the window would
  come up with nothing drawn. QuotaDock now checks three times after startup that painting
  really happened, and repaints if it did not.

### Changed

- Resized the display-mode icons so the normal / compact / nano hierarchy reads at a glance.
- Added a local diagnostic log at `%LOCALAPPDATA%\QuotaDock\quotadock.log`. It records
  start and exit, refresh outcomes, and connection-state changes. It never records usage
  figures, accounts, or tokens. Capped at 1 MB, and nothing is sent anywhere.

## [0.7.15] — 2026-07-30

The first release after the repository went public. Earlier versions are listed under
[releases](https://github.com/jungdosa/QuotaDock/releases).

[0.7.38]: https://github.com/jungdosa/QuotaDock/releases/tag/v0.7.38
[0.7.37]: https://github.com/jungdosa/QuotaDock/releases/tag/v0.7.37
[0.7.36]: https://github.com/jungdosa/QuotaDock/releases/tag/v0.7.36
[0.7.35]: https://github.com/jungdosa/QuotaDock/releases/tag/v0.7.35
[0.7.34]: https://github.com/jungdosa/QuotaDock/releases/tag/v0.7.34
[0.7.32]: https://github.com/jungdosa/QuotaDock/releases/tag/v0.7.32
[0.7.31]: https://github.com/jungdosa/QuotaDock/releases/tag/v0.7.31
[0.7.30]: https://github.com/jungdosa/QuotaDock/releases/tag/v0.7.30
[0.7.26]: https://github.com/jungdosa/QuotaDock/releases/tag/v0.7.26
[0.7.15]: https://github.com/jungdosa/QuotaDock/releases/tag/v0.7.15
