package update

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLiveGitHubUpdateFlow exercises the real update path against the published
// GitHub release: anonymous API fetch, version comparison, asset selection,
// download, and SHA-256 verification. It stops short of running the installer
// by injecting a fake launcher.
//
//	QUOTADOCK_LIVE_UPDATE=1 go test ./internal/update -run TestLiveGitHubUpdateFlow -v
//
// QUOTADOCK_LIVE_UPDATE_FROM sets the version the check pretends to run as; it
// must be older than the published release for the available path to trigger.
func TestLiveGitHubUpdateFlow(t *testing.T) {
	if os.Getenv("QUOTADOCK_LIVE_UPDATE") != "1" {
		t.Skip("set QUOTADOCK_LIVE_UPDATE=1 to test against the live GitHub release API")
	}
	from := os.Getenv("QUOTADOCK_LIVE_UPDATE_FROM")
	if from == "" {
		t.Fatal("set QUOTADOCK_LIVE_UPDATE_FROM to the installed version under test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	fetcher := NewHTTPReleaseFetcher(from, nil)
	release, err := fetcher.Latest(ctx)
	if err != nil {
		t.Fatalf("anonymous release fetch failed: %v", err)
	}
	t.Logf("live release tag=%s draft=%v prerelease=%v assets=%d", release.TagName, release.Draft, release.Prerelease, len(release.Assets))

	result := Checker{Fetcher: fetcher, CurrentVersion: from}.Check(ctx)
	if result.Status != CheckAvailable {
		t.Fatalf("check from %s: status=%v, want CheckAvailable (release %s)", from, result.Status, release.TagName)
	}
	t.Logf("update detected: %s -> %s", from, DisplayVersion(result.Release.TagName))

	asset, ok := SelectInstallAsset(result.Release)
	if !ok {
		t.Fatal("no installer asset in the live release")
	}
	digest, ok := ParseSHA256Digest(asset.Digest)
	if !ok {
		t.Fatalf("live digest %q did not parse", asset.Digest)
	}
	t.Logf("asset=%s size=%d sha256=%s", asset.Name, asset.Size, digest)

	launcher := &fakeLauncher{}
	directory := t.TempDir()
	installer := &Installer{
		DownloadDir:    directory,
		Version:        from,
		ExecutablePath: func() (string, error) { return filepath.Join(directory, "quotadock.exe"), nil },
		Launcher:       launcher,
	}
	var stages []Stage
	if err := installer.Install(ctx, asset, func(progress Progress) {
		if len(stages) == 0 || stages[len(stages)-1] != progress.Stage {
			stages = append(stages, progress.Stage)
		}
	}); err != nil {
		t.Fatalf("live download and verification failed: %v", err)
	}
	t.Logf("stages=%v launcher calls=%d", stages, launcher.calls)

	if launcher.calls != 1 {
		t.Fatalf("launcher calls=%d, want 1", launcher.calls)
	}
	staged := filepath.Join(directory, stagedInstallerName)
	info, statErr := os.Stat(staged)
	if statErr != nil {
		t.Fatalf("staged installer missing: %v", statErr)
	}
	if info.Size() != asset.Size {
		t.Fatalf("staged size=%d, release asset size=%d", info.Size(), asset.Size)
	}
	if err := verifyInstallerHash(staged, digest); err != nil {
		t.Fatalf("independent re-verification of the staged installer failed: %v", err)
	}
}
