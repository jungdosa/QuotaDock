package update

import (
	"context"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"

	"github.com/jungdosa/QuotaDock/internal/security"
)

const ReleasesPageURL = "https://github.com/jungdosa/QuotaDock/releases/latest"

func ParseSHA256Digest(raw string) (string, bool) {
	const prefix = "sha256:"
	if len(raw) != len(prefix)+64 || !strings.EqualFold(raw[:len(prefix)], prefix) {
		return "", false
	}
	digest := raw[len(prefix):]
	decoded, err := hex.DecodeString(digest)
	if err != nil || len(decoded) != 32 {
		return "", false
	}
	return strings.ToLower(digest), true
}

func SelectInstallAsset(release Release) (Asset, bool) {
	for _, asset := range release.Assets {
		if !isSafeSetupName(asset.Name) {
			continue
		}
		if _, valid := ParseSHA256Digest(asset.Digest); !valid {
			return Asset{}, false
		}
		return asset, true
	}
	return Asset{}, false
}

func HasPortableAsset(release Release) bool {
	for _, asset := range release.Assets {
		if strings.HasSuffix(strings.ToLower(asset.Name), "-portable.exe") {
			return true
		}
	}
	return false
}

type InstallRunner interface {
	Install(context.Context, Asset, func(Progress)) error
}

type ApplyResult int

const (
	ReleasePageOpened ApplyResult = iota
	InstallerLaunched
)

type Flow struct {
	Portable    bool
	Installer   InstallRunner
	OpenRelease func(string) error
}

func (f Flow) CanAutoInstall(release Release) bool {
	if f.Portable {
		return false
	}
	_, available := SelectInstallAsset(release)
	return available
}

func (f Flow) Apply(ctx context.Context, release Release, report func(Progress)) (ApplyResult, error) {
	asset, automatic := SelectInstallAsset(release)
	if f.Portable || !automatic {
		if f.OpenRelease == nil {
			return ReleasePageOpened, errors.New("release page opener is unavailable")
		}
		return ReleasePageOpened, f.OpenRelease(releasePageURL(release.HTMLURL))
	}
	if f.Installer == nil {
		return InstallerLaunched, errors.New("installer is unavailable")
	}
	if err := f.Installer.Install(ctx, asset, report); err != nil {
		return InstallerLaunched, err
	}
	return InstallerLaunched, nil
}

func releasePageURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err == nil && security.IsAllowedExternalURL(raw) {
		host := strings.ToLower(parsed.Hostname())
		if host == "github.com" || strings.HasSuffix(host, ".github.com") {
			return raw
		}
	}
	return ReleasesPageURL
}
