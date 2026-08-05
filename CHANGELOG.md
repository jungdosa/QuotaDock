# Changelog

Notable changes to QuotaDock. Every released version also has a
[GitHub release](https://github.com/jungdosa/QuotaDock/releases) carrying the same notes
plus checksums for its binaries.

Versions follow [Semantic Versioning](https://semver.org/). The project reaches 1.0.0 once
Windows feature verification is finished.

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

[0.7.32]: https://github.com/jungdosa/QuotaDock/releases/tag/v0.7.32
[0.7.31]: https://github.com/jungdosa/QuotaDock/releases/tag/v0.7.31
[0.7.30]: https://github.com/jungdosa/QuotaDock/releases/tag/v0.7.30
[0.7.26]: https://github.com/jungdosa/QuotaDock/releases/tag/v0.7.26
[0.7.15]: https://github.com/jungdosa/QuotaDock/releases/tag/v0.7.15
