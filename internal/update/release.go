// Package update implements QuotaDock's credential-free release update flow.
package update

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jungdosa/QuotaDock/internal/security"
)

const LatestReleaseURL = "https://api.github.com/repos/jungdosa/QuotaDock/releases/latest"

// ReleaseFetcher separates release lookup from policy so tests never need the network.
type ReleaseFetcher interface {
	Latest(context.Context) (Release, error)
}

type Release struct {
	TagName    string  `json:"tag_name"`
	HTMLURL    string  `json:"html_url"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
	Size               int64  `json:"size"`
}

type HTTPReleaseFetcher struct {
	Client  *http.Client
	Version string
}

func NewHTTPReleaseFetcher(version string, client *http.Client) *HTTPReleaseFetcher {
	return &HTTPReleaseFetcher{Client: client, Version: version}
}

func (f *HTTPReleaseFetcher) Latest(ctx context.Context) (Release, error) {
	var release Release
	if !security.IsAllowedUpdateURL(LatestReleaseURL) {
		return release, errors.New("latest release URL is not allowed")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, LatestReleaseURL, nil)
	if err != nil {
		return release, fmt.Errorf("create update request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "QuotaDock/"+f.Version)
	response, err := updateHTTPClient(f.Client).Do(request)
	if err != nil {
		return release, fmt.Errorf("request latest release: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return release, fmt.Errorf("latest release returned HTTP %d", response.StatusCode)
	}
	if err := security.DecodeJSONLimited(response.Body, security.DefaultMaxJSONSize, &release); err != nil {
		return Release{}, fmt.Errorf("decode latest release: %w", err)
	}
	return release, nil
}

func updateHTTPClient(base *http.Client) *http.Client {
	var client http.Client
	if base != nil {
		client = *base
	} else {
		client.Timeout = 15 * time.Second
	}
	previousRedirect := client.CheckRedirect
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("too many update redirects")
		}
		if !security.IsAllowedUpdateURL(request.URL.String()) {
			return fmt.Errorf("update redirect URL is not allowed: %s", request.URL.Hostname())
		}
		if previousRedirect != nil {
			return previousRedirect(request, via)
		}
		return nil
	}
	return &client
}

type CheckStatus int

const (
	CheckUnavailable CheckStatus = iota
	CheckUpToDate
	CheckAvailable
)

type CheckResult struct {
	Status  CheckStatus
	Release Release
}

type Checker struct {
	Fetcher        ReleaseFetcher
	CurrentVersion string
}

func (c Checker) Check(ctx context.Context) CheckResult {
	if c.Fetcher == nil {
		return CheckResult{Status: CheckUnavailable}
	}
	release, err := c.Fetcher.Latest(ctx)
	if err != nil {
		slog.Debug("update information is unavailable", "error", err)
		return CheckResult{Status: CheckUnavailable}
	}
	if release.Draft || release.Prerelease {
		return CheckResult{Status: CheckUpToDate}
	}
	newer, valid := versionIsNewer(release.TagName, c.CurrentVersion)
	if !valid || !newer {
		return CheckResult{Status: CheckUpToDate}
	}
	return CheckResult{Status: CheckAvailable, Release: release}
}

func DisplayVersion(tag string) string {
	return strings.TrimPrefix(strings.TrimPrefix(tag, "v"), "V")
}

func versionIsNewer(candidate, current string) (bool, bool) {
	candidateVersion, candidateOK := parseVersion(candidate)
	currentVersion, currentOK := parseVersion(current)
	if !candidateOK || !currentOK {
		return false, false
	}
	for index := range candidateVersion {
		if candidateVersion[index] != currentVersion[index] {
			return candidateVersion[index] > currentVersion[index], true
		}
	}
	return false, true
}

func parseVersion(raw string) ([3]int, bool) {
	var parsed [3]int
	raw = DisplayVersion(strings.TrimSpace(raw))
	parts := strings.Split(raw, ".")
	if len(parts) != len(parsed) {
		return parsed, false
	}
	for index, part := range parts {
		if part == "" {
			return [3]int{}, false
		}
		for _, character := range part {
			if character < '0' || character > '9' {
				return [3]int{}, false
			}
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			return [3]int{}, false
		}
		parsed[index] = value
	}
	return parsed, true
}
