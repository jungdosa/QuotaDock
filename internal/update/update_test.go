package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeReleaseFetcher struct {
	release Release
	err     error
}

func (f fakeReleaseFetcher) Latest(context.Context) (Release, error) {
	return f.release, f.err
}

func TestVersionComparison(t *testing.T) {
	tests := []struct {
		candidate string
		current   string
		newer     bool
		valid     bool
	}{
		{"0.7.10", "0.7.9", true, true},
		{"0.8.0", "0.7.10", true, true},
		{"0.7.10", "0.7.10", false, true},
		{"v0.7.10", "0.7.9", true, true},
		{"0.7.x", "0.7.9", false, false},
		{"0.7", "0.7.9", false, false},
	}
	for _, test := range tests {
		newer, valid := versionIsNewer(test.candidate, test.current)
		if newer != test.newer || valid != test.valid {
			t.Errorf("versionIsNewer(%q, %q) = %v, %v; want %v, %v", test.candidate, test.current, newer, valid, test.newer, test.valid)
		}
	}
	checker := Checker{
		Fetcher:        fakeReleaseFetcher{release: Release{TagName: "v0.7.10"}},
		CurrentVersion: "0.7.9",
	}
	if result := checker.Check(context.Background()); result.Status != CheckAvailable {
		t.Fatalf("injected release fetcher status = %v, want available", result.Status)
	}
}

func TestDigestParsing(t *testing.T) {
	valid := strings.Repeat("aB", 32)
	if got, ok := ParseSHA256Digest("SHA256:" + valid); !ok || got != strings.ToLower(valid) {
		t.Fatalf("valid digest = %q, %v", got, ok)
	}
	for _, digest := range []string{
		"md5:" + strings.Repeat("a", 64),
		"sha256:" + strings.Repeat("a", 63),
		"sha256:" + strings.Repeat("z", 64),
	} {
		if _, ok := ParseSHA256Digest(digest); ok {
			t.Errorf("invalid digest accepted: %q", digest)
		}
	}
}

func TestAssetSelection(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	setup := Asset{Name: "QuotaDock-0.7.10-win-x64-Setup.exe", Digest: digest}
	portable := Asset{Name: "QuotaDock-0.7.10-win-x64-Portable.exe", Digest: digest}
	if got, ok := SelectInstallAsset(Release{Assets: []Asset{portable, setup}}); !ok || got.Name != setup.Name {
		t.Fatalf("setup selection = %+v, %v", got, ok)
	}
	portableOnly := Release{Assets: []Asset{portable}}
	if _, ok := SelectInstallAsset(portableOnly); ok || !HasPortableAsset(portableOnly) {
		t.Fatal("portable-only release was selected for automatic installation")
	}
	setup.Digest = ""
	if _, ok := SelectInstallAsset(Release{Assets: []Asset{setup}}); ok {
		t.Fatal("digest-less setup was selected for automatic installation")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestCheckFailuresAreUnavailable(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		err    error
	}{
		{name: "404", status: http.StatusNotFound},
		{name: "403", status: http.StatusForbidden},
		{name: "timeout", err: context.DeadlineExceeded},
		{name: "invalid JSON", status: http.StatusOK, body: "{not-json"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.Header.Get("Authorization") != "" {
					t.Fatal("update request carried Authorization")
				}
				if request.Header.Get("Accept") != "application/vnd.github+json" || request.Header.Get("User-Agent") != "QuotaDock/0.7.9" {
					t.Fatalf("unexpected update headers: %v", request.Header)
				}
				if test.err != nil {
					return nil, test.err
				}
				return &http.Response{
					StatusCode: test.status,
					Body:       io.NopCloser(strings.NewReader(test.body)),
					Header:     make(http.Header),
					Request:    request,
				}, nil
			})}
			checker := Checker{Fetcher: NewHTTPReleaseFetcher("0.7.9", client), CurrentVersion: "0.7.9"}
			if result := checker.Check(context.Background()); result.Status != CheckUnavailable {
				t.Fatalf("failure status = %v, want unavailable", result.Status)
			}
		})
	}
}

type fakeLauncher struct {
	calls int
}

func (f *fakeLauncher) Launch(string, string) error {
	f.calls++
	return nil
}

func TestHashMismatchDeletesDownloadAndDoesNotLaunch(t *testing.T) {
	payload := []byte("not the expected installer")
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "" {
			t.Fatal("download request carried Authorization")
		}
		return &http.Response{
			StatusCode:    http.StatusOK,
			Body:          io.NopCloser(bytes.NewReader(payload)),
			ContentLength: int64(len(payload)),
			Header:        make(http.Header),
			Request:       request,
		}, nil
	})}
	launcher := &fakeLauncher{}
	directory := t.TempDir()
	installer := &Installer{
		Client:         client,
		DownloadDir:    directory,
		Version:        "0.7.9",
		ExecutablePath: func() (string, error) { return os.Executable() },
		Launcher:       launcher,
	}
	asset := Asset{
		Name:               "QuotaDock-0.7.10-win-x64-Setup.exe",
		BrowserDownloadURL: "https://objects.githubusercontent.com/update.exe",
		Digest:             "sha256:" + strings.Repeat("0", sha256.Size*2),
		Size:               int64(len(payload)),
	}
	err := installer.Install(context.Background(), asset, nil)
	if !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("install error = %v, want hash mismatch", err)
	}
	if launcher.calls != 0 {
		t.Fatalf("launcher calls = %d, want 0", launcher.calls)
	}
	entries, readErr := os.ReadDir(directory)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("hash mismatch left downloaded files: %v", entries)
	}
}

func TestInstallerReverifiesImmediatelyBeforeLaunch(t *testing.T) {
	payload := []byte("verified installer payload")
	digest := sha256.Sum256(payload)
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Body:          io.NopCloser(bytes.NewReader(payload)),
			ContentLength: int64(len(payload)),
			Header:        make(http.Header),
			Request:       request,
		}, nil
	})}
	launcher := &fakeLauncher{}
	directory := t.TempDir()
	name := "QuotaDock-0.7.10-win-x64-Setup.exe"
	stagedPath := filepath.Join(directory, stagedInstallerName)
	installer := &Installer{
		Client:      client,
		DownloadDir: directory,
		Version:     "0.7.9",
		ExecutablePath: func() (string, error) {
			if err := os.WriteFile(stagedPath, []byte("changed after first verification"), 0o600); err != nil {
				return "", err
			}
			return os.Executable()
		},
		Launcher: launcher,
	}
	asset := Asset{
		Name:               name,
		BrowserDownloadURL: "https://release-assets.githubusercontent.com/update.exe",
		Digest:             "sha256:" + hex.EncodeToString(digest[:]),
		Size:               int64(len(payload)),
	}
	err := installer.Install(context.Background(), asset, nil)
	if !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("second verification error = %v, want hash mismatch", err)
	}
	if launcher.calls != 0 {
		t.Fatalf("launcher calls = %d, want 0", launcher.calls)
	}
	if _, statErr := os.Stat(stagedPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("changed installer was not deleted: %v", statErr)
	}
}

type fakeInstallRunner struct {
	calls int
}

func (f *fakeInstallRunner) Install(context.Context, Asset, func(Progress)) error {
	f.calls++
	return nil
}

func TestPortableFlowOpensReleasePageWithoutInstalling(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	release := Release{
		HTMLURL: "https://github.com/jungdosa/QuotaDock/releases/tag/v0.7.10",
		Assets: []Asset{{
			Name:   "QuotaDock-0.7.10-win-x64-Setup.exe",
			Digest: digest,
		}},
	}
	runner := &fakeInstallRunner{}
	opened := ""
	flow := Flow{
		Portable:  true,
		Installer: runner,
		OpenRelease: func(raw string) error {
			opened = raw
			return nil
		},
	}
	result, err := flow.Apply(context.Background(), release, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != ReleasePageOpened || runner.calls != 0 || opened != release.HTMLURL {
		t.Fatalf("portable result=%v installer calls=%d opened=%q", result, runner.calls, opened)
	}
}
