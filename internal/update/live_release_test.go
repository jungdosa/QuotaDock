package update

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/jungdosa/QuotaDock/internal/security"
)

// testdata/latest-release.json is a verbatim capture of the GitHub API response
// for the published v0.7.10 release. It guards the parser against the real
// payload shape rather than a hand-written mock: field names, the "sha256:"
// digest format, and asset naming all come from GitHub itself. Refresh it with
//
//	gh api repos/jungdosa/QuotaDock/releases/latest > testdata/latest-release.json
func loadLiveRelease(t *testing.T) Release {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "latest-release.json"))
	if err != nil {
		t.Fatalf("read live release fixture: %v", err)
	}
	var release Release
	if err := security.DecodeJSONLimited(bytes.NewReader(raw), security.DefaultMaxJSONSize, &release); err != nil {
		t.Fatalf("decode live release fixture: %v", err)
	}
	return release
}

func TestLiveReleasePayloadParses(t *testing.T) {
	release := loadLiveRelease(t)
	if release.TagName != "v0.7.10" {
		t.Fatalf("tag=%q, want v0.7.10", release.TagName)
	}
	if release.Draft || release.Prerelease {
		t.Fatalf("published release reported draft=%v prerelease=%v", release.Draft, release.Prerelease)
	}
	if !security.IsAllowedExternalURL(release.HTMLURL) {
		t.Fatalf("release page URL %q is not an allowed external URL", release.HTMLURL)
	}
	if len(release.Assets) == 0 {
		t.Fatal("live release carried no assets")
	}
}

func TestLiveReleaseSelectsVerifiableInstaller(t *testing.T) {
	release := loadLiveRelease(t)

	asset, ok := SelectInstallAsset(release)
	if !ok {
		t.Fatal("no installer asset was selected from the live release")
	}
	if asset.Name != "QuotaDock-0.7.10-win-x64-Setup.exe" {
		t.Fatalf("selected asset=%q", asset.Name)
	}
	if asset.Size <= 0 {
		t.Fatalf("selected asset size=%d", asset.Size)
	}
	if !security.IsAllowedUpdateURL(asset.BrowserDownloadURL) {
		t.Fatalf("download URL %q is not an allowed update URL", asset.BrowserDownloadURL)
	}

	digest, ok := ParseSHA256Digest(asset.Digest)
	if !ok {
		t.Fatalf("live asset digest %q did not parse", asset.Digest)
	}
	if len(digest) != 64 {
		t.Fatalf("parsed digest length=%d", len(digest))
	}

	if !HasPortableAsset(release) {
		t.Fatal("live release is missing the portable asset")
	}
}

func TestLiveReleaseDrivesCheckerDecisions(t *testing.T) {
	fetcher := fakeReleaseFetcher{release: loadLiveRelease(t)}

	tests := []struct {
		current string
		want    CheckStatus
	}{
		{"0.7.9", CheckAvailable},  // string compare would wrongly call 0.7.9 newer
		{"0.7.8", CheckAvailable},
		{"0.7.10", CheckUpToDate},
		{"0.8.0", CheckUpToDate},
		{"0.7.11", CheckUpToDate},
	}
	for _, test := range tests {
		result := Checker{Fetcher: fetcher, CurrentVersion: test.current}.Check(t.Context())
		if result.Status != test.want {
			t.Fatalf("current=%s status=%v, want %v", test.current, result.Status, test.want)
		}
	}
}
