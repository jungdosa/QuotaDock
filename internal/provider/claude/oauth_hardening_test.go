package claude

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jungdosa/QuotaDock/internal/model"
)

func TestOAuthRateLimitExponentialFloorAndRecovery(t *testing.T) {
	for _, header := range []string{"0", "1", "", "invalid"} {
		t.Run("header_"+header, func(t *testing.T) {
			now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
			path := filepath.Join(t.TempDir(), ".credentials.json")
			writeCredentials(t, path, "test-access", "test-refresh", now.Add(24*time.Hour))
			limited, calls := false, 0
			client, _ := testOAuthClient(t, path, func(w http.ResponseWriter, _ *http.Request) {
				calls++
				if limited {
					w.Header().Set("Retry-After", header)
					w.WriteHeader(http.StatusTooManyRequests)
					return
				}
				_, _ = w.Write([]byte(syntheticUsage))
			})
			client.now = func() time.Time { return now }
			if _, err := client.Fetch(context.Background()); err != nil {
				t.Fatal(err)
			}
			limited = true
			for _, minutes := range []int{5, 10, 20, 40, 60, 60} {
				result, err := client.Fetch(context.Background())
				if err != nil || !result.cached || string(result.raw) != syntheticUsage {
					t.Fatal("rate limit did not preserve the last successful response")
				}
				deadline := client.sourceCache[OAuthCredentialDefault].backoffUntil
				if got := deadline.Sub(now); got != time.Duration(minutes)*time.Minute {
					t.Fatalf("backoff = %v, want %d minutes", got, minutes)
				}
				before := calls
				now = deadline.Add(-time.Nanosecond)
				if result, err := client.Fetch(context.Background()); err != nil || !result.cached || calls != before {
					t.Fatal("usage was requested before the backoff elapsed")
				}
				now = deadline
			}
			limited = false
			if result, err := client.Fetch(context.Background()); err != nil || result.cached {
				t.Fatal("success did not resume live usage")
			}
			limited = true
			if _, err := client.Fetch(context.Background()); err != nil {
				t.Fatal(err)
			}
			if client.sourceCache[OAuthCredentialDefault].backoffUntil.Sub(now) != 5*time.Minute {
				t.Fatal("success did not reset the exponential floor")
			}
		})
	}
}

func TestOAuthRateLimitHeaderBoundsWithoutCache(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	for name, tc := range map[string]struct {
		header string
		want   time.Duration
	}{
		"zero":      {"0", 5 * time.Minute},
		"long":      {"7200", time.Hour},
		"overflow":  {"9223372036854775807", time.Hour},
		"date":      {now.Add(15 * time.Minute).Format(http.TimeFormat), 15 * time.Minute},
		"long date": {now.Add(2 * time.Hour).Format(http.TimeFormat), time.Hour},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".credentials.json")
			writeCredentials(t, path, "test-access", "test-refresh", now.Add(24*time.Hour))
			calls := 0
			client, _ := testOAuthClient(t, path, func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.Header().Set("Retry-After", tc.header)
				w.WriteHeader(http.StatusTooManyRequests)
			})
			client.now = func() time.Time { return now }
			for range 2 {
				if _, err := client.Fetch(context.Background()); !errors.Is(err, errOAuthRateLimited) {
					t.Fatalf("rate limit error = %v", err)
				}
			}
			if calls != 1 || client.sourceCache[OAuthCredentialDefault].backoffUntil.Sub(now) != tc.want {
				t.Fatal("header bound or uncached cooldown was not honored")
			}
		})
	}
}

func TestOAuthExpiredRefreshFailureClassification(t *testing.T) {
	for _, status := range []int{400, 401, 503} {
		for _, cached := range []bool{false, true} {
			t.Run(http.StatusText(status)+map[bool]string{false: "", true: "_cached"}[cached], func(t *testing.T) {
				now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
				path := filepath.Join(t.TempDir(), ".credentials.json")
				writeCredentials(t, path, "test-access", "test-refresh", now)
				tokens, usage := 0, 0
				client, _ := testOAuthClient(t, path, func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/token" {
						tokens++
						w.WriteHeader(status)
					} else {
						usage++
						w.WriteHeader(http.StatusUnauthorized)
					}
				})
				client.now = func() time.Time { return now }
				if cached {
					client.sourceCache[OAuthCredentialDefault].lastSuccess = oauthResult{raw: json.RawMessage(syntheticUsage)}
				}
				for range 2 {
					result, err := client.Fetch(context.Background())
					switch {
					case status == 400 || status == 401:
						if !errors.Is(err, errOAuthReauthentication) {
							t.Fatalf("rejected refresh error = %v", err)
						}
					case cached:
						if err != nil || !result.cached || string(result.raw) != syntheticUsage {
							t.Fatal("transient refresh failure lost cached usage")
						}
					default:
						if !errors.Is(err, errOAuthUnavailable) || errors.Is(err, errOAuthReauthentication) {
							t.Fatalf("transient refresh error = %v", err)
						}
					}
				}
				if tokens != 1 || usage != 0 {
					t.Fatalf("token calls=%d usage calls=%d", tokens, usage)
				}
				if status == 503 && !cached {
					cli := &fakeClient{version: MinimumCLIVersion, auth: json.RawMessage(`{"loggedIn":true,"subscriptionType":"pro"}`), limitsErr: ErrRateLimitsUnavailable}
					provider := newProvider(cli, client, MinimumCLIVersion)
					_, err := provider.Refresh(context.Background())
					var safe model.SafeError
					if !errors.As(err, &safe) || safe.Code != model.ErrUsageUnavailable || provider.state.Current().Status == model.StatusLoggedOut {
						t.Fatal("CLI auth-status fallback incorrectly reported logged out")
					}
				}
			})
		}
	}
}

func TestOAuthRejectedRefreshKeepsUsingUnexpiredBearer(t *testing.T) {
	for _, status := range []int{400, 401} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
			path := filepath.Join(t.TempDir(), ".credentials.json")
			// Inside the refresh buffer but not yet expired.
			writeCredentials(t, path, "test-access", "test-refresh", now.Add(2*time.Minute))
			usage := 0
			client, _ := testOAuthClient(t, path, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/token" {
					w.WriteHeader(status)
					return
				}
				usage++
				_, _ = w.Write([]byte(syntheticUsage))
			})
			client.now = func() time.Time { return now }
			result, err := client.Fetch(context.Background())
			if err != nil || result.cached || string(result.raw) != syntheticUsage || usage != 1 {
				t.Fatalf("rejected refresh with a still-valid bearer: err=%v usage calls=%d", err, usage)
			}
			now = now.Add(3 * time.Minute)
			if _, err := client.Fetch(context.Background()); !errors.Is(err, errOAuthReauthentication) {
				t.Fatalf("expired bearer after a rejected refresh: err=%v", err)
			}
		})
	}
}

func TestOAuthRefreshCooldownRecoversAndAdoptsRotatedToken(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), ".credentials.json")
	writeCredentials(t, path, "test-access", "test-refresh", now)
	fail, calls := true, 0
	client, _ := testOAuthClient(t, path, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			calls++
			if fail {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"fresh-test-access","expires_in":3600}`))
			return
		}
		_, _ = w.Write([]byte(syntheticUsage))
	})
	client.now = func() time.Time { return now }
	_, _ = client.Fetch(context.Background())
	now = now.Add(time.Minute - time.Nanosecond)
	_, _ = client.Fetch(context.Background())
	if calls != 1 {
		t.Fatal("refresh retried before one minute")
	}
	now = now.Add(time.Nanosecond)
	_, _ = client.Fetch(context.Background())
	if calls != 2 {
		t.Fatal("refresh did not retry at one minute")
	}
	writeCredentials(t, path, "rotated-test-access", "rotated-test-refresh", now)
	fail = false
	if _, err := client.Fetch(context.Background()); err != nil || calls != 3 {
		t.Fatal("CLI rotation did not bypass the old token cooldown")
	}
	if client.refreshFailure != nil || !client.refreshRetryUntil.IsZero() || client.refreshFailedToken != "" {
		t.Fatal("successful refresh did not clear cooldown")
	}
}

func TestOAuthRefreshPersistenceFailureStillUsesFreshToken(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), ".credentials.json")
	writeCredentials(t, path, "test-access", "test-refresh", now)
	usage := 0
	client, _ := testOAuthClient(t, path, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			// Make persistence fail after credentials have already been loaded.
			if err := os.WriteFile(path, []byte("invalid"), 0600); err != nil {
				t.Error(err)
			}
			_, _ = w.Write([]byte(`{"access_token":"fresh-test-access","expires_in":3600}`))
			return
		}
		usage++
		if r.Header.Get("Authorization") != "Bearer fresh-test-access" {
			t.Error("usage did not use refreshed credentials")
		}
		_, _ = w.Write([]byte(syntheticUsage))
	})
	client.now = func() time.Time { return now }
	warned := false
	client.reportRefreshFailure = func() { warned = true }
	if result, err := client.Fetch(context.Background()); err != nil || result.cached || usage != 1 || !warned {
		t.Fatal("persistence failure prevented fresh-token usage")
	}
}

type refreshFailureTransport struct{ err error }

func (r refreshFailureTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, r.err }

func TestOAuthRefreshTransportFailures(t *testing.T) {
	for _, failure := range []error{context.DeadlineExceeded, errors.New("network unavailable")} {
		client := NewOAuthClient()
		client.httpClient = &http.Client{Transport: refreshFailureTransport{err: failure}}
		_, err := client.refreshCredentials(context.Background(), oauthCredentials{refreshToken: "test-refresh"})
		want := errOAuthUnavailable
		if errors.Is(failure, context.DeadlineExceeded) {
			want = context.DeadlineExceeded
		}
		if !errors.Is(err, want) {
			t.Fatalf("transport classification = %v, want %v", err, want)
		}
	}
}
