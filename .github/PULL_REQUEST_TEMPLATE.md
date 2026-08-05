## What this changes

<!-- One or two sentences. Link the issue if there is one. -->

## Why

<!-- What was wrong, or what was missing. -->

## Checks

- [ ] `go test ./...` passes
- [ ] `go vet ./...` passes
- [ ] If a UI string changed, all twelve locales under `internal/i18n/locales/` were updated
- [ ] If the settings schema changed, existing settings files still load with safe defaults
- [ ] Commit messages are in English and follow Conventional Commits

## Tested on

<!-- Windows version, which providers, installer or portable. -->
