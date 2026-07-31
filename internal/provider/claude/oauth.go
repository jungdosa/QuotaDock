package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jungdosa/QuotaDock/internal/model"
	"github.com/jungdosa/QuotaDock/internal/security"
)

const (
	claudeUsageURL       = "https://api.anthropic.com/api/oauth/usage"
	claudeTokenURL       = "https://platform.claude.com/v1/oauth/token"
	claudeOAuthClientID  = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	claudeOAuthBeta      = "oauth-2025-04-20"
	oauthRequestTimeout  = 10 * time.Second
	oauthExpiryBuffer    = 5 * time.Minute
	defaultRetryBackoff  = 5 * time.Minute
	defaultAccessTokenTT = time.Hour
)

var (
	errOAuthCredentialsUnavailable = errors.New("Claude OAuth credentials are unavailable")
	errOAuthReauthentication       = errors.New("Claude OAuth reauthentication is required")
	errOAuthRateLimited            = errors.New("Claude OAuth usage is rate limited")
	errOAuthInvalidResponse        = errors.New("Claude OAuth response is invalid")
	errOAuthUnavailable            = errors.New("Claude OAuth usage is unavailable")
)

type oauthCredentials struct {
	accessToken      string
	refreshToken     string
	expiresAt        time.Time
	scopes           []string
	rateLimitTier    string
	subscriptionType string
}

func (c oauthCredentials) expiresWithin(now time.Time, buffer time.Duration) bool {
	return !c.expiresAt.IsZero() && !c.expiresAt.After(now.Add(buffer))
}

type credentialEnvelope struct {
	ClaudeAIOAuth struct {
		AccessToken      string   `json:"accessToken"`
		RefreshToken     string   `json:"refreshToken"`
		ExpiresAt        int64    `json:"expiresAt"`
		Scopes           []string `json:"scopes"`
		RateLimitTier    string   `json:"rateLimitTier"`
		SubscriptionType string   `json:"subscriptionType"`
	} `json:"claudeAiOauth"`
}

func parseOAuthCredentials(raw []byte) (oauthCredentials, error) {
	var envelope credentialEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return oauthCredentials{}, errOAuthInvalidResponse
	}
	entry := envelope.ClaudeAIOAuth
	accessToken := strings.TrimSpace(entry.AccessToken)
	if accessToken == "" {
		return oauthCredentials{}, errOAuthCredentialsUnavailable
	}
	credentials := oauthCredentials{
		accessToken:      accessToken,
		refreshToken:     strings.TrimSpace(entry.RefreshToken),
		scopes:           append([]string(nil), entry.Scopes...),
		rateLimitTier:    entry.RateLimitTier,
		subscriptionType: entry.SubscriptionType,
	}
	if entry.ExpiresAt > 0 {
		credentials.expiresAt = time.UnixMilli(entry.ExpiresAt).UTC()
	}
	return credentials, nil
}

type oauthResult struct {
	raw              json.RawMessage
	rateLimitTier    string
	subscriptionType string
	cached           bool
}

type oauthUsageFetcher interface {
	Available() bool
	Fetch(context.Context) (oauthResult, error)
}

// OAuthClient reads only Claude Code's credentials file (or the optional
// environment override). It never queries an OS keyring.
type OAuthClient struct {
	mu                   sync.Mutex
	httpClient           *http.Client
	credentialsPath      string
	usageURL             string
	tokenURL             string
	now                  func() time.Time
	getenv               func(string) string
	allowURL             func(string) bool
	backoffUntil         time.Time
	lastSuccess          oauthResult
	reportRefreshFailure func()
}

func NewOAuthClient() *OAuthClient {
	path := ""
	if home, err := os.UserHomeDir(); err == nil {
		path = filepath.Join(home, ".claude", ".credentials.json")
	}
	client := &http.Client{
		Timeout: oauthRequestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &OAuthClient{
		httpClient:      client,
		credentialsPath: path,
		usageURL:        claudeUsageURL,
		tokenURL:        claudeTokenURL,
		now:             time.Now,
		getenv:          os.Getenv,
		allowURL:        security.IsAllowedProviderRequestURL,
		reportRefreshFailure: func() {
			slog.Warn("Claude OAuth credential refresh failed; existing credential fallback engaged")
		},
	}
}

func (c *OAuthClient) Available() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, _, err := c.loadCredentials()
	return err == nil
}

func (c *OAuthClient) Fetch(ctx context.Context) (oauthResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	credentials, fromFile, err := c.loadCredentials()
	if err != nil {
		return oauthResult{}, err
	}
	now := c.now()
	if now.Before(c.backoffUntil) {
		if len(c.lastSuccess.raw) != 0 {
			result := c.lastSuccess
			result.cached = true
			return result, nil
		}
		return oauthResult{}, errOAuthRateLimited
	}

	credentials, refreshErr := c.ensureFreshCredentials(ctx, credentials, fromFile)
	if refreshErr != nil && c.reportRefreshFailure != nil {
		c.reportRefreshFailure()
	}
	raw, retryAfter, err := c.fetchUsage(ctx, credentials)
	if errors.Is(err, errOAuthRateLimited) {
		c.backoffUntil = now.Add(retryAfter)
		if len(c.lastSuccess.raw) != 0 {
			result := c.lastSuccess
			result.cached = true
			return result, nil
		}
		return oauthResult{}, err
	}
	if err != nil {
		return oauthResult{}, err
	}

	result := oauthResult{
		raw:              raw,
		rateLimitTier:    credentials.rateLimitTier,
		subscriptionType: credentials.subscriptionType,
	}
	c.backoffUntil = time.Time{}
	c.lastSuccess = result
	return result, nil
}

func (c *OAuthClient) loadCredentials() (oauthCredentials, bool, error) {
	if token := strings.TrimSpace(c.getenv("CLAUDE_CODE_OAUTH_TOKEN")); token != "" {
		return oauthCredentials{accessToken: token}, false, nil
	}
	if c.credentialsPath == "" {
		return oauthCredentials{}, false, errOAuthCredentialsUnavailable
	}
	raw, err := os.ReadFile(c.credentialsPath)
	if errors.Is(err, os.ErrNotExist) {
		return oauthCredentials{}, false, errOAuthCredentialsUnavailable
	}
	if err != nil {
		return oauthCredentials{}, false, errOAuthUnavailable
	}
	if len(raw) > int(security.DefaultMaxJSONSize) {
		return oauthCredentials{}, false, errOAuthInvalidResponse
	}
	credentials, err := parseOAuthCredentials(raw)
	return credentials, true, err
}

func (c *OAuthClient) ensureFreshCredentials(ctx context.Context, credentials oauthCredentials, fromFile bool) (oauthCredentials, error) {
	if !credentials.expiresWithin(c.now(), oauthExpiryBuffer) || !fromFile {
		return credentials, nil
	}

	// The Claude CLI shares this file. Re-read immediately before refreshing so
	// a token already rotated by the CLI or another poll is adopted.
	raw, err := os.ReadFile(c.credentialsPath)
	if err == nil && len(raw) <= int(security.DefaultMaxJSONSize) {
		if disk, parseErr := parseOAuthCredentials(raw); parseErr == nil {
			if !disk.expiresWithin(c.now(), oauthExpiryBuffer) {
				return disk, nil
			}
			credentials = disk
		}
	}
	if credentials.refreshToken == "" {
		return credentials, errOAuthUnavailable
	}

	refreshed, err := c.refreshCredentials(ctx, credentials)
	if err != nil {
		return credentials, err
	}
	if err := persistOAuthCredentials(c.credentialsPath, refreshed); err != nil {
		// The refreshed in-memory credential is still usable. Atomic persistence
		// guarantees that the original file remains intact on failure.
		return refreshed, err
	}
	return refreshed, nil
}

type refreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
}

func (c *OAuthClient) refreshCredentials(ctx context.Context, current oauthCredentials) (oauthCredentials, error) {
	body, err := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": current.refreshToken,
		"client_id":     claudeOAuthClientID,
	})
	if err != nil {
		return oauthCredentials{}, errOAuthUnavailable
	}
	request, err := c.newRequest(ctx, http.MethodPost, c.tokenURL, bytes.NewReader(body))
	if err != nil {
		return oauthCredentials{}, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("anthropic-beta", claudeOAuthBeta)
	response, err := c.httpClient.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return oauthCredentials{}, context.DeadlineExceeded
		}
		return oauthCredentials{}, errOAuthUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return oauthCredentials{}, errOAuthUnavailable
	}
	var payload refreshResponse
	if err := security.DecodeJSONLimited(response.Body, security.DefaultMaxJSONSize, &payload); err != nil {
		return oauthCredentials{}, errOAuthInvalidResponse
	}
	payload.AccessToken = strings.TrimSpace(payload.AccessToken)
	if payload.AccessToken == "" {
		return oauthCredentials{}, errOAuthInvalidResponse
	}
	refreshed := current
	refreshed.accessToken = payload.AccessToken
	if token := strings.TrimSpace(payload.RefreshToken); token != "" {
		refreshed.refreshToken = token
	}
	ttl := time.Duration(payload.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = defaultAccessTokenTT
	}
	refreshed.expiresAt = c.now().Add(ttl).UTC()
	if scopes := strings.Fields(payload.Scope); len(scopes) > 0 {
		refreshed.scopes = scopes
	}
	return refreshed, nil
}

func (c *OAuthClient) fetchUsage(ctx context.Context, credentials oauthCredentials) (json.RawMessage, time.Duration, error) {
	request, err := c.newRequest(ctx, http.MethodGet, c.usageURL, nil)
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("Authorization", "Bearer "+credentials.accessToken)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("anthropic-beta", claudeOAuthBeta)
	response, err := c.httpClient.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, 0, context.DeadlineExceeded
		}
		return nil, 0, errOAuthUnavailable
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusUnauthorized:
		return nil, 0, errOAuthReauthentication
	case http.StatusTooManyRequests:
		return nil, retryAfterDuration(response.Header.Get("Retry-After"), c.now()), errOAuthRateLimited
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, 0, errOAuthUnavailable
	}
	var raw json.RawMessage
	if err := security.DecodeJSONLimited(response.Body, security.DefaultMaxJSONSize, &raw); err != nil {
		return nil, 0, errOAuthInvalidResponse
	}
	return raw, 0, nil
}

func (c *OAuthClient) newRequest(ctx context.Context, method, rawURL string, body io.Reader) (*http.Request, error) {
	if !c.allowURL(rawURL) {
		return nil, errOAuthUnavailable
	}
	request, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, errOAuthUnavailable
	}
	return request, nil
}

func retryAfterDuration(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if deadline, err := http.ParseTime(value); err == nil && deadline.After(now) {
		return deadline.Sub(now)
	}
	return defaultRetryBackoff
}

func persistOAuthCredentials(path string, credentials oauthCredentials) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return errOAuthUnavailable
	}
	info, err := os.Stat(path)
	if err != nil {
		return errOAuthUnavailable
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(raw, &root) != nil {
		return errOAuthInvalidResponse
	}
	var oauth map[string]json.RawMessage
	if json.Unmarshal(root["claudeAiOauth"], &oauth) != nil {
		return errOAuthInvalidResponse
	}
	oauth["accessToken"], _ = json.Marshal(credentials.accessToken)
	oauth["refreshToken"], _ = json.Marshal(credentials.refreshToken)
	oauth["expiresAt"] = json.RawMessage(strconv.FormatInt(credentials.expiresAt.UnixMilli(), 10))
	if len(credentials.scopes) > 0 {
		oauth["scopes"], _ = json.Marshal(credentials.scopes)
	}
	root["claudeAiOauth"], _ = json.Marshal(oauth)
	serialized, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return errOAuthInvalidResponse
	}
	serialized = append(serialized, '\n')

	parent := filepath.Dir(path)
	temp, err := os.CreateTemp(parent, ".credentials.json.quotadock-*")
	if err != nil {
		return errOAuthUnavailable
	}
	tempPath := temp.Name()
	keep := false
	defer func() {
		_ = temp.Close()
		if !keep {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(info.Mode().Perm()); err != nil {
		return errOAuthUnavailable
	}
	if _, err := temp.Write(serialized); err != nil {
		return errOAuthUnavailable
	}
	if err := temp.Sync(); err != nil {
		return errOAuthUnavailable
	}
	if err := temp.Close(); err != nil {
		return errOAuthUnavailable
	}
	if err := os.Rename(tempPath, path); err != nil {
		return errOAuthUnavailable
	}
	keep = true
	return nil
}

type oauthUsageWindow struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    string   `json:"resets_at"`
}

type oauthScopedLimit struct {
	Kind     string   `json:"kind"`
	Group    string   `json:"group"`
	Percent  *float64 `json:"percent"`
	ResetsAt string   `json:"resets_at"`
	Scope    struct {
		Model struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"model"`
	} `json:"scope"`
}

type oauthMoney struct {
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
	Exponent    int    `json:"exponent"`
}

type oauthExtraUsage struct {
	IsEnabled bool `json:"is_enabled"`
}

type oauthSpend struct {
	Used    *oauthMoney `json:"used"`
	Limit   *oauthMoney `json:"limit"`
	Percent *float64    `json:"percent"`
	Enabled bool        `json:"enabled"`
}

type oauthUsageResponse struct {
	FiveHour      *oauthUsageWindow  `json:"five_hour"`
	SevenDay      *oauthUsageWindow  `json:"seven_day"`
	SevenDayFable *oauthUsageWindow  `json:"seven_day_fable"`
	Fable         *oauthUsageWindow  `json:"fable"`
	Limits        []oauthScopedLimit `json:"limits"`
	ExtraUsage    json.RawMessage    `json:"extra_usage"`
	Spend         json.RawMessage    `json:"spend"`
}

func NormalizeOAuthUsage(raw json.RawMessage, rateLimitTier, subscriptionType string, fetchedAt time.Time) (model.UsageSnapshot, error) {
	var payload oauthUsageResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return model.UsageSnapshot{}, err
	}
	snapshot := model.UsageSnapshot{
		Provider:  model.ProviderClaude,
		Plan:      NormalizeClaudeOAuthPlan(rateLimitTier, subscriptionType),
		FetchedAt: fetchedAt.UTC(),
	}
	appendWindow := func(id, label string, windowMinutes int, window *oauthUsageWindow) {
		if window == nil || window.Utilization == nil || !finite(*window.Utilization) {
			return
		}
		used, remaining := model.PercentPair(*window.Utilization, false)
		snapshot.Limits = append(snapshot.Limits, model.UsageLimit{
			ID:               id,
			Label:            label,
			UsedPercent:      used,
			RemainingPercent: remaining,
			WindowMinutes:    windowMinutes,
			ResetsAt:         parseISOTime(window.ResetsAt),
		})
	}
	appendWindow("five_hour", "Session", 300, payload.FiveHour)

	weekly := payload.SevenDay
	if scoped := weeklyAllLimit(payload.Limits); scoped != nil {
		weekly = scoped
	}
	appendWindow("seven_day", "Weekly", 10080, weekly)

	fable := payload.SevenDayFable
	if fable == nil {
		fable = payload.Fable
	}
	if fable == nil {
		fable = fableLimit(payload.Limits)
	}
	appendWindow("seven_day_fable", "Fable", 10080, fable)
	snapshot.Credits = normalizeOAuthCredits(payload.ExtraUsage, payload.Spend)
	return snapshot, nil
}

func normalizeOAuthCredits(extraUsageRaw, spendRaw json.RawMessage) *model.Credits {
	if len(extraUsageRaw) == 0 || len(spendRaw) == 0 {
		return nil
	}
	var extraUsage oauthExtraUsage
	if err := json.Unmarshal(extraUsageRaw, &extraUsage); err != nil || !extraUsage.IsEnabled {
		return nil
	}
	var spend oauthSpend
	if err := json.Unmarshal(spendRaw, &spend); err != nil || !spend.Enabled || spend.Used == nil || spend.Limit == nil || spend.Limit.AmountMinor <= 0 || spend.Percent == nil || !finite(*spend.Percent) {
		return nil
	}
	used, usedOK := amountFromMinor(spend.Used.AmountMinor, spend.Used.Exponent)
	limit, limitOK := amountFromMinor(spend.Limit.AmountMinor, spend.Limit.Exponent)
	if !usedOK || !limitOK {
		return nil
	}
	currency := strings.ToUpper(strings.TrimSpace(spend.Used.Currency))
	if currency == "" {
		currency = strings.ToUpper(strings.TrimSpace(spend.Limit.Currency))
	}
	return &model.Credits{Spend: &model.CreditSpend{
		Used:     used,
		Limit:    limit,
		Currency: currency,
		Percent:  *spend.Percent,
	}}
}

func amountFromMinor(amountMinor int64, exponent int) (float64, bool) {
	divisor := math.Pow10(exponent)
	if divisor == 0 || math.IsNaN(divisor) || math.IsInf(divisor, 0) {
		return 0, false
	}
	amount := float64(amountMinor) / divisor
	return amount, finite(amount)
}

func weeklyAllLimit(limits []oauthScopedLimit) *oauthUsageWindow {
	for i := range limits {
		limit := &limits[i]
		if limit.Kind != "weekly_all" || (limit.Group != "" && limit.Group != "weekly") || limit.Percent == nil || !finite(*limit.Percent) {
			continue
		}
		return &oauthUsageWindow{Utilization: limit.Percent, ResetsAt: limit.ResetsAt}
	}
	return nil
}

func fableLimit(limits []oauthScopedLimit) *oauthUsageWindow {
	for i := range limits {
		limit := &limits[i]
		if limit.Kind != "weekly_scoped" || limit.Group != "weekly" || limit.Percent == nil || !finite(*limit.Percent) {
			continue
		}
		identity := strings.ToLower(limit.Scope.Model.ID + " " + limit.Scope.Model.DisplayName)
		if !strings.Contains(identity, "fable") {
			continue
		}
		return &oauthUsageWindow{Utilization: limit.Percent, ResetsAt: limit.ResetsAt}
	}
	return nil
}

func NormalizeClaudeOAuthPlan(rateLimitTier, subscriptionType string) model.Plan {
	for _, raw := range []string{rateLimitTier, subscriptionType} {
		token := strings.ToLower(strings.Join(strings.Fields(strings.NewReplacer("-", "_", " ", "_").Replace(raw)), "_"))
		switch token {
		case "default_claude_max_20x", "claude_max_20x", "max_20x", "max20x":
			return "MAX 20X"
		case "default_claude_max_5x", "claude_max_5x", "max_5x", "max5x":
			return "MAX 5X"
		case "max", "claude_max":
			return "MAX"
		case "pro", "claude_pro", "default_claude_pro":
			return "PRO"
		case "team", "claude_team":
			return "TEAM"
		case "enterprise", "claude_enterprise":
			return "ENTERPRISE"
		case "free", "claude_free":
			return "FREE"
		}
	}
	return model.PlanUnknown
}

func parseISOTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
