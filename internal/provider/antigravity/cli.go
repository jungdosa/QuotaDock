package antigravity

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jungdosa/QuotaDock/internal/process"
)

const (
	cliVersionTimeout = 3 * time.Second
	cliUsageTimeout   = 20 * time.Second
	cliInterval       = 5 * time.Minute
	cliOutputLimit    = 256 << 10
)

var (
	errCLIUnavailable     = errors.New("Antigravity CLI is unavailable")
	errCLIInvalidResponse = errors.New("Antigravity CLI response was rejected")
	errCLILoggedOut       = errors.New("Antigravity CLI is not logged in")
)

// Production clients share this state for the entire application lifetime.
// Closing/reconnecting a provider must never reset the safety circuit breaker.
type cliClient struct {
	mu             sync.Mutex
	prepare        func() (process.CommandSpec, error)
	run            func(context.Context, process.CommandSpec, process.LogFunc) ([]byte, error)
	now            func() time.Time
	versionChecked bool
	versionOK      bool
	versionPath    string
	tripped        bool
	terminalErr    error
	lastAttempt    time.Time
	lastSuccess    json.RawMessage
	lastErr        error
}

func (c *cliClient) fetch(ctx context.Context) (json.RawMessage, error) {
	// The version probe is included in the total request budget.
	ctx, cancelRequest := context.WithTimeout(ctx, cliUsageTimeout)
	defer cancelRequest()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tripped {
		return nil, c.terminalErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.versionChecked && !c.versionOK {
		return nil, errCLIUnavailable
	}
	if !c.lastAttempt.IsZero() && c.now().Sub(c.lastAttempt) < cliInterval {
		if c.lastSuccess != nil && c.lastErr == nil {
			return append(json.RawMessage(nil), c.lastSuccess...), nil
		}
		return nil, c.lastErr
	}
	// Failed attempts also consume the interval, so polling cannot create a loop.
	c.lastAttempt = c.now()
	c.lastErr = errCLIUnavailable
	spec, err := c.prepare()
	if err != nil {
		return nil, c.lastErr
	}
	if !c.versionChecked {
		c.versionChecked = true
		versionCtx, cancel := context.WithTimeout(ctx, cliVersionTimeout)
		spec.Args = []string{"--version"}
		raw, versionErr := c.run(versionCtx, spec, nil)
		cancel()
		c.versionOK = versionErr == nil && supportedCLIVersion(raw)
		c.versionPath = spec.Name
		if !c.versionOK {
			return nil, errCLIUnavailable
		}
	}
	// A cached gate cannot authorize a different binary selected later from PATH.
	if !strings.EqualFold(spec.Name, c.versionPath) {
		return nil, errCLIUnavailable
	}
	usageCtx, cancel := context.WithTimeout(ctx, cliUsageTimeout)
	defer cancel()
	spec.Args = []string{"-p", "/usage", "--output-format", "json"}
	loginFailure := false
	retryableLoginFailure := false
	raw, runErr := c.run(usageCtx, spec, func(text string) {
		// Only retain booleans; stderr may contain account information.
		loginFailure = loginFailure || cliLoginFailure(text)
		retryableLoginFailure = retryableLoginFailure || cliRetryableLoginFailure(text)
	})
	// Check safety evidence before considering the pre-model login exception.
	// An oversized or truncated response cannot establish a safe login failure.
	if errors.Is(runErr, process.ErrOutputLimit) || len(raw) > cliOutputLimit {
		return nil, c.trip(errCLIInvalidResponse)
	}
	var envelope struct {
		Error    string  `json:"error"`
		NumTurns float64 `json:"num_turns"`
		Usage    struct {
			TotalTokens float64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(raw, &envelope) == nil {
		if envelope.NumTurns > 0 || envelope.Usage.TotalTokens > 0 {
			return nil, c.trip(errCLIInvalidResponse)
		}
		loginFailure = loginFailure || cliLoginFailure(envelope.Error)
		retryableLoginFailure = retryableLoginFailure || cliRetryableLoginFailure(envelope.Error)
	}
	if retryableLoginFailure {
		// These observed messages precede model invocation. Retry after the
		// normal interval so a later login can recover without restarting.
		c.lastErr = errCLILoggedOut
		return nil, c.lastErr
	}
	if runErr != nil {
		switch {
		case errors.Is(runErr, process.ErrProcessExited):
			// RunOutput discards stdout on nonzero exit. Its envelope cannot be
			// verified, so fail closed outside the narrow login exception.
			// Broader sign-in text classifies the failure but cannot allow retry.
			if loginFailure {
				return nil, c.trip(errCLILoggedOut)
			}
			return nil, c.trip(errCLIUnavailable)
		case loginFailure:
			return nil, c.trip(errCLILoggedOut)
		case errors.Is(runErr, process.ErrTimeout), errors.Is(runErr, context.DeadlineExceeded):
			c.lastErr = context.DeadlineExceeded
		default:
			c.lastErr = ErrLocalRequest
		}
		return nil, c.lastErr
	}
	adapted, parseErr := parseCLIQuota(raw)
	if parseErr != nil {
		if loginFailure {
			return nil, c.trip(errCLILoggedOut)
		}
		return nil, c.trip(errCLIInvalidResponse)
	}
	if loginFailure {
		return nil, c.trip(errCLILoggedOut)
	}
	c.lastSuccess = append(json.RawMessage(nil), adapted...)
	c.lastErr = nil
	return adapted, nil
}

func (c *cliClient) trip(err error) error {
	if !c.tripped {
		c.tripped = true
		c.terminalErr = err
		c.lastSuccess = nil
		slog.Warn("antigravity.cli.tripped")
	}
	return c.terminalErr
}

var cliVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func supportedCLIVersion(raw []byte) bool {
	// Accept only the observed release format, not banners or embedded versions.
	match := cliVersionPattern.FindStringSubmatch(strings.TrimSpace(string(raw)))
	if match == nil {
		return false
	}
	var version [3]uint64
	for i := range version {
		value, err := strconv.ParseUint(match[i+1], 10, 64)
		if err != nil {
			return false
		}
		version[i] = value
	}
	return version[0] > 1 || version[0] == 1 && (version[1] > 1 || version[1] == 1 && version[2] >= 11)
}

func cliLoginFailure(text string) bool {
	text = strings.ToLower(text)
	for _, phrase := range []string{"not logged in", "not signed in", "login", "sign in", "sign-in", "log in", "unauthenticated", "not authenticated", "authentication required"} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

func cliRetryableLoginFailure(text string) bool {
	text = strings.ToLower(text)
	for _, phrase := range []string{"not logged in", "not logged into", "not signed in"} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

func cliEnvironment(getenv func(string) string) []string {
	// A non-nil empty slice prevents exec from inheriting the parent environment.
	env := make([]string, 0, 11)
	for _, name := range []string{"PATH", "SYSTEMROOT", "WINDIR", "USERPROFILE", "HOMEDRIVE", "HOMEPATH", "LOCALAPPDATA", "APPDATA", "TEMP", "TMP", "PROGRAMDATA"} {
		if value := getenv(name); value != "" {
			env = append(env, name+"="+value)
		}
	}
	return env
}

func parseCLIQuota(raw []byte) (json.RawMessage, error) {
	var envelope struct {
		Status   string `json:"status"`
		NumTurns *int64 `json:"num_turns"`
		Usage    struct {
			TotalTokens *int64 `json:"total_tokens"`
		} `json:"usage"`
		Command struct {
			Name string `json:"name"`
			Data struct {
				Groups []struct {
					Name    string `json:"name"`
					Buckets []struct {
						ID                string   `json:"id"`
						Name              string   `json:"name"`
						Window            string   `json:"window"`
						RemainingFraction *float64 `json:"remaining_fraction"`
						ResetTime         string   `json:"reset_time"`
						Disabled          bool     `json:"disabled"`
					} `json:"buckets"`
				} `json:"groups"`
			} `json:"data"`
		} `json:"command"`
	}
	// Pointers distinguish a proven zero from an absent/null safety counter.
	if len(raw) > cliOutputLimit || json.Unmarshal(raw, &envelope) != nil ||
		envelope.Status != "SUCCESS" || envelope.Command.Name != "usage" ||
		envelope.NumTurns == nil || *envelope.NumTurns != 0 ||
		envelope.Usage.TotalTokens == nil || *envelope.Usage.TotalTokens != 0 ||
		envelope.Command.Data.Groups == nil {
		return nil, errCLIInvalidResponse
	}
	groups := make([]transportQuotaGroup, 0, len(envelope.Command.Data.Groups))
	for _, group := range envelope.Command.Data.Groups {
		converted := transportQuotaGroup{DisplayName: group.Name}
		for _, entry := range group.Buckets {
			converted.Buckets = append(converted.Buckets, transportQuotaBucket{
				DisplayName: entry.Name, Window: entry.Window,
				RemainingFraction: entry.RemainingFraction, ResetTime: entry.ResetTime, Disabled: entry.Disabled,
			})
		}
		groups = append(groups, converted)
	}
	return adaptQuotaGroups(groups, "")
}
