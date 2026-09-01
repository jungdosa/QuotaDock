package claude

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strings"
	"time"

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
// can be faked in tests without a real browser. Fetch takes the whole chain of
// requests rather than one address, so both of them ride a single browser.
type webSession interface {
	Fetch(ctx context.Context, next func(index int, previous string) (string, bool)) ([]string, error)
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

	trace := webAuthTrace{started: time.Now()}
	defer func() { trace.log() }()

	// Both requests are described up front and run inside one browser. The
	// organization list has to come back before the usage address can be built,
	// so the chain is a function of the previous response rather than a list.
	var org webOrganization
	// issued names the round trip that is in flight. A duration cannot stand in
	// for it: a request that finishes inside a millisecond rounds to zero, and
	// the failure would then be blamed on the trip before it.
	issued := 0
	stage := time.Now()
	bodies, err := session.Fetch(ctx, func(index int, previous string) (string, bool) {
		switch index {
		case 0:
			issued = 1
			return webOrganizationsURL, true
		case 1:
			trace.organizations = time.Since(stage).Milliseconds()
			stage = time.Now()
			found, ok := chatOrganization(previous)
			if !ok {
				return "", false
			}
			org = found
			issued = 2
			return webOrganizationsURL + "/" + found.UUID + "/usage", true
		}
		trace.usage = time.Since(stage).Milliseconds()
		return "", false
	})
	if err != nil {
		if issued >= 2 {
			trace.usage = time.Since(stage).Milliseconds()
			trace.fail("usage", err)
		} else {
			trace.organizations = time.Since(stage).Milliseconds()
			trace.fail("organizations", err)
		}
		return oauthResult{}, err
	}
	if len(bodies) < 2 {
		// The endpoint answered but not with a usable organization list —
		// almost always a signed-out redirect to HTML or a challenge page.
		trace.fail("organizations", errOAuthReauthentication)
		return oauthResult{}, errOAuthReauthentication
	}
	usageRaw := bodies[1]
	if !json.Valid([]byte(usageRaw)) {
		trace.fail("usage", errOAuthReauthentication)
		return oauthResult{}, errOAuthReauthentication
	}
	return oauthResult{
		raw:              json.RawMessage(usageRaw),
		rateLimitTier:    org.RateLimitTier,
		subscriptionType: org.BillingType,
	}, nil
}

// webAuthTrace records how one usage read spent its time. Each read costs two
// browser round trips, and the refresh that wraps them has a fixed budget, so
// the useful question when a read times out is which of the two was still
// running — a single line per read answers it without the volume of logging
// each trip separately.
type webAuthTrace struct {
	started       time.Time
	organizations int64
	usage         int64
	stage         string
	reason        string
}

// fail names the round trip that ended the read and classifies why. The error
// is never quoted: it can wrap a browser-profile path that carries the user's
// home directory.
func (t *webAuthTrace) fail(stage string, err error) {
	t.stage = stage
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		t.reason = "deadline"
	case errors.Is(err, context.Canceled):
		t.reason = "cancelled"
	case errors.Is(err, errOAuthReauthentication):
		t.reason = "signed_out"
	default:
		t.reason = "failed"
	}
}

func (t *webAuthTrace) log() {
	slog.Info("webauth.fetch",
		"ok", t.stage == "",
		"stage", t.stage,
		"err", t.reason,
		"orgs_ms", t.organizations,
		"usage_ms", t.usage,
		"ms", time.Since(t.started).Milliseconds(),
	)
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
