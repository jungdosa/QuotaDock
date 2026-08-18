package claude

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/jungdosa/QuotaDock/internal/webview"
)

// webUsageURL builds the account usage endpoint for one organization. The
// host is claude.ai (the web app), not the OAuth API, because this path reuses
// the browser sign-in session rather than a local credential.
const (
	webOrganizationsURL = "https://claude.ai/api/organizations"
)

// WebAuthFetcher reads Claude usage through the embedded browser sign-in
// instead of the local CLI credential. It implements oauthUsageFetcher, so the
// existing normalization and plan logic are reused unchanged; only the source
// of the raw JSON differs. The browser session lives in the WebView2 profile
// folder, and this code never reads the cookie store itself.
type WebAuthFetcher struct {
	userDataDir string
	newSession  func(dir string) webSession
}

// webSession is the slice of webview.Session this fetcher needs, named so it
// can be faked in tests without a real browser.
type webSession interface {
	Fetch(ctx context.Context, url string) (string, error)
	Close() error
}

// NewWebAuthFetcher points the fetcher at the shared WebView2 profile folder.
func NewWebAuthFetcher(userDataDir string) *WebAuthFetcher {
	return &WebAuthFetcher{
		userDataDir: userDataDir,
		newSession:  func(dir string) webSession { return webview.NewSession(dir) },
	}
}

// Available reports whether the web path can even be attempted: the runtime
// must be present and the user must have signed in at least once, which is
// what creates the profile folder. It never opens a window, so it is cheap
// enough for the connection inspection path.
func (w *WebAuthFetcher) Available() bool {
	if w == nil || w.userDataDir == "" {
		return false
	}
	if !webview.DetectRuntime().Present {
		return false
	}
	info, err := os.Stat(w.userDataDir)
	return err == nil && info.IsDir()
}

type webOrganization struct {
	UUID          string   `json:"uuid"`
	Capabilities  []string `json:"capabilities"`
	RateLimitTier string   `json:"rate_limit_tier"`
	BillingType   string   `json:"billing_type"`
}

// Fetch runs the account requests inside the browser session and returns the
// raw usage JSON in the same shape the OAuth API returns, so the caller
// normalizes it with NormalizeOAuthUsage. A signed-out session yields
// errOAuthReauthentication so the lane asks the user to sign in again.
func (w *WebAuthFetcher) Fetch(ctx context.Context) (oauthResult, error) {
	session := w.newSession(w.userDataDir)
	defer session.Close()

	orgsRaw, err := session.Fetch(ctx, webOrganizationsURL)
	if err != nil {
		return oauthResult{}, err
	}
	org, ok := chatOrganization(orgsRaw)
	if !ok {
		// The endpoint answered but not with a usable organization list —
		// almost always a signed-out redirect to HTML or a challenge page.
		return oauthResult{}, errOAuthReauthentication
	}
	usageRaw, err := session.Fetch(ctx, webOrganizationsURL+"/"+org.UUID+"/usage")
	if err != nil {
		return oauthResult{}, err
	}
	if !json.Valid([]byte(usageRaw)) {
		return oauthResult{}, errOAuthReauthentication
	}
	return oauthResult{
		raw:              json.RawMessage(usageRaw),
		rateLimitTier:    org.RateLimitTier,
		subscriptionType: org.BillingType,
	}, nil
}

// chatOrganization picks the organization that backs the Claude app (the one
// with the "chat" capability); an API-only organization returns usage errors.
func chatOrganization(raw string) (webOrganization, bool) {
	var orgs []webOrganization
	if err := json.Unmarshal([]byte(raw), &orgs); err != nil {
		return webOrganization{}, false
	}
	for _, org := range orgs {
		if org.UUID == "" {
			continue
		}
		for _, capability := range org.Capabilities {
			if strings.EqualFold(capability, "chat") {
				return org, true
			}
		}
	}
	return webOrganization{}, false
}

// errWebAuthUnavailable is returned when a sign-in is requested but the
// embedded browser cannot run at all.
var errWebAuthUnavailable = errors.New("the embedded sign-in browser is unavailable")

// SignIn opens the visible sign-in window and blocks until the user reaches
// the signed-in application, closes the window, or ctx ends. It is a
// user-triggered action, never part of the refresh loop.
func (w *WebAuthFetcher) SignIn(ctx context.Context) error {
	if w == nil || w.userDataDir == "" {
		return errWebAuthUnavailable
	}
	if !webview.DetectRuntime().Present {
		return errWebAuthUnavailable
	}
	session := webview.NewSession(w.userDataDir)
	defer session.Close()
	return session.SignIn(ctx, "https://claude.ai/login", webview.SignedInAt)
}
