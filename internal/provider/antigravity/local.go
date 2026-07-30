package antigravity

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jungdosa/QuotaDock/internal/model"
)

const (
	RetrieveQuotaEndpoint = "/exa.language_server_pb.LanguageServerService/RetrieveUserQuotaSummary"
	GetUserStatusEndpoint = "/exa.language_server_pb.LanguageServerService/GetUserStatus"
	MaxResponseBytes      = 1 << 20
	RequestTimeout        = 5 * time.Second
)

var (
	ErrEndpointNotAllowed = errors.New("Antigravity endpoint is not allowed")
	ErrResponseTooLarge   = errors.New("Antigravity response exceeds limit")
	ErrLocalRequest       = errors.New("Antigravity local request failed")
)

var allowedEndpoints = map[string]struct{}{
	RetrieveQuotaEndpoint: {},
	GetUserStatusEndpoint: {},
}

type endpointCandidate struct {
	pid        uint32
	port       uint16
	token      string
	executable string
	verified   bool
}

type discoverFunc func() ([]endpointCandidate, error)

type LocalClient struct {
	mu       sync.Mutex
	discover discoverFunc
	current  *endpointCandidate
	tier     string
}

func NewLocalClient() *LocalClient {
	return &LocalClient{discover: discoverLocalEndpoints}
}

func (c *LocalClient) Status(ctx context.Context) (bool, bool, error) {
	candidates, err := c.discover()
	if err != nil {
		return false, false, ErrLocalRequest
	}
	if len(candidates) == 0 {
		return false, false, nil
	}
	for _, candidate := range candidates {
		raw, requestErr := requestLocal(ctx, candidate, GetUserStatusEndpoint)
		if requestErr != nil {
			continue
		}
		c.mu.Lock()
		copy := candidate
		c.current = &copy
		c.tier = statusTier(raw)
		c.mu.Unlock()
		return true, statusIsLoggedIn(raw), nil
	}
	return true, false, ErrLocalRequest
}

func statusIsLoggedIn(raw json.RawMessage) bool {
	var status struct {
		LoggedIn   *bool     `json:"loggedIn"`
		UserStatus *struct{} `json:"userStatus"`
		User       *struct{} `json:"user"`
	}
	if json.Unmarshal(raw, &status) != nil {
		return false
	}
	if status.LoggedIn != nil {
		return *status.LoggedIn
	}
	return status.UserStatus != nil || status.User != nil
}

func statusTier(raw json.RawMessage) string {
	var status struct {
		UserStatus struct {
			UserTier struct {
				Name string `json:"name"`
				ID   string `json:"id"`
			} `json:"userTier"`
		} `json:"userStatus"`
	}
	if json.Unmarshal(raw, &status) != nil {
		return ""
	}
	value := status.UserStatus.UserTier.Name
	if value == "" {
		value = status.UserStatus.UserTier.ID
	}
	plan := model.NormalizePlan(model.ProviderAntigravity, canonicalTier(value))
	if plan == model.PlanUnknown {
		return ""
	}
	return string(plan)
}

func (c *LocalClient) RetrieveUserQuotaSummary(ctx context.Context) (json.RawMessage, error) {
	c.mu.Lock()
	current := c.current
	c.mu.Unlock()
	if current != nil {
		if raw, err := requestLocal(ctx, *current, RetrieveQuotaEndpoint); err == nil {
			c.mu.Lock()
			tier := c.tier
			c.mu.Unlock()
			return adaptQuotaResponse(raw, tier)
		}
	}
	candidates, err := c.discover()
	if err != nil {
		return nil, ErrLocalRequest
	}
	for _, candidate := range candidates {
		statusRaw, statusErr := requestLocal(ctx, candidate, GetUserStatusEndpoint)
		if statusErr != nil || !statusIsLoggedIn(statusRaw) {
			continue
		}
		raw, requestErr := requestLocal(ctx, candidate, RetrieveQuotaEndpoint)
		if requestErr == nil {
			c.mu.Lock()
			copy := candidate
			c.current = &copy
			c.tier = statusTier(statusRaw)
			tier := c.tier
			c.mu.Unlock()
			return adaptQuotaResponse(raw, tier)
		}
	}
	return nil, ErrLocalRequest
}

type transportQuotaResponse struct {
	Response struct {
		Groups []struct {
			DisplayName string `json:"displayName"`
			Description string `json:"description"`
			Buckets     []struct {
				RemainingFraction *float64 `json:"remainingFraction"`
				ResetTime         string   `json:"resetTime"`
				Window            string   `json:"window"`
				DisplayName       string   `json:"displayName"`
			} `json:"buckets"`
		} `json:"groups"`
	} `json:"response"`
}

func adaptQuotaResponse(raw json.RawMessage, tierName string) (json.RawMessage, error) {
	var legacy struct {
		QuotaGroups json.RawMessage `json:"quotaGroups"`
	}
	if json.Unmarshal(raw, &legacy) != nil {
		return nil, ErrLocalRequest
	}
	if len(legacy.QuotaGroups) > 0 {
		return raw, nil
	}
	var source transportQuotaResponse
	if json.Unmarshal(raw, &source) != nil || source.Response.Groups == nil {
		return nil, ErrLocalRequest
	}
	var target quotaEnvelope
	target.UserStatus.UserTier.Name = tierName
	for _, group := range source.Response.Groups {
		name := safeQuotaGroupName(group.DisplayName)
		if name == "" {
			name = safeQuotaGroupName(group.Description)
		}
		if name == "" {
			continue
		}
		converted := quotaGroup{Name: name}
		for _, sourceBucket := range group.Buckets {
			if sourceBucket.RemainingFraction == nil {
				continue
			}
			window := quotaWindowMinutes(sourceBucket.Window, sourceBucket.DisplayName)
			converted.Buckets = append(converted.Buckets, bucket{
				ID:                safeBucketID(window),
				RemainingFraction: sourceBucket.RemainingFraction,
				WindowMinutes:     window,
				ResetTime:         resetUnix(sourceBucket.ResetTime),
			})
		}
		target.QuotaGroups = append(target.QuotaGroups, converted)
	}
	encoded, err := json.Marshal(target)
	if err != nil {
		return nil, ErrLocalRequest
	}
	return encoded, nil
}

func safeQuotaGroupName(value string) string {
	value = strings.ToLower(value)
	switch {
	case strings.Contains(value, "gemini"):
		return "Gemini Models"
	case strings.Contains(value, "claude") || strings.Contains(value, "gpt"):
		return "Claude and GPT models"
	default:
		return ""
	}
}

func quotaWindowMinutes(values ...string) int {
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if duration, err := time.ParseDuration(normalized); err == nil && duration > 0 {
			return int(duration.Minutes())
		}
		switch {
		case strings.Contains(normalized, "5") && strings.Contains(normalized, "hour"):
			return 300
		case strings.Contains(normalized, "7") && strings.Contains(normalized, "day"):
			return 10080
		case strings.Contains(normalized, "week"):
			return 10080
		}
	}
	return 0
}

func safeBucketID(window int) string {
	switch window {
	case 300:
		return "five_hour"
	case 10080:
		return "weekly"
	default:
		return "quota"
	}
}

func resetUnix(value string) int64 {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return 0
	}
	return parsed.Unix()
}

func (c *LocalClient) Close() error {
	c.mu.Lock()
	c.current = nil
	c.tier = ""
	c.mu.Unlock()
	return nil
}

func isAllowedEndpoint(endpoint string) bool {
	_, ok := allowedEndpoints[endpoint]
	return ok
}

func requestLocal(ctx context.Context, candidate endpointCandidate, endpoint string) (json.RawMessage, error) {
	if !isAllowedEndpoint(endpoint) || !candidate.verified || candidate.port == 0 || candidate.token == "" || !IsAllowedExecutable(candidate.executable) {
		return nil, ErrEndpointNotAllowed
	}
	requestCtx, cancel := context.WithTimeout(ctx, RequestTimeout)
	defer cancel()
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(int(candidate.port)))
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: RequestTimeout}).DialContext(ctx, "tcp", address)
		},
		// The candidate is already bound to a verified Antigravity PID and an
		// owned loopback listener. No other host or endpoint reaches this transport.
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   RequestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return ErrEndpointNotAllowed
		},
	}
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, "https://"+address+endpoint, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, ErrLocalRequest
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Connect-Protocol-Version", "1")
	request.Header.Set("x-codeium-csrf-token", candidate.token)
	response, err := client.Do(request)
	if err != nil {
		return nil, ErrLocalRequest
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, ErrLocalRequest
	}
	return readResponseLimited(response.Body)
}

func readResponseLimited(reader io.Reader) (json.RawMessage, error) {
	data, err := io.ReadAll(io.LimitReader(reader, MaxResponseBytes+1))
	if err != nil {
		return nil, ErrLocalRequest
	}
	if len(data) > MaxResponseBytes {
		return nil, ErrResponseTooLarge
	}
	if !json.Valid(data) {
		return nil, ErrLocalRequest
	}
	return append(json.RawMessage(nil), data...), nil
}

var tokenPattern = regexp.MustCompile(`(?i)(?:^|\s)--(?:extension_server_csrf_token|csrf_token)(?:=|\s+)(?:"([^"]+)"|([^\s"]+))`)

func csrfTokens(commandLine string) []string {
	matches := tokenPattern.FindAllStringSubmatch(commandLine, -1)
	seen := make(map[string]struct{})
	var tokens []string
	for _, match := range matches {
		value := match[1]
		if value == "" {
			value = match[2]
		}
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		tokens = append(tokens, value)
	}
	return tokens
}

func validateProcessIdentity(executablePath, commandLine string) bool {
	if !IsAllowedExecutable(strings.ToLower(filepath.Base(executablePath))) {
		return false
	}
	path := strings.ReplaceAll(strings.ToLower(filepath.Clean(executablePath)), "\\", "/")
	command := strings.ReplaceAll(strings.ToLower(commandLine), "\\", "/")
	belongs := func(value string) bool {
		return strings.Contains(value, "/programs/antigravity ide/") || strings.Contains(value, "/antigravity ide/resources/app/extensions/antigravity/")
	}
	return belongs(path) || belongs(command)
}

var _ Client = (*LocalClient)(nil)
