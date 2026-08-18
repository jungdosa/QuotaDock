package grok

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jungdosa/QuotaDock/internal/model"
	shared "github.com/jungdosa/QuotaDock/internal/provider"
)

type httpClientFunc func(*http.Request) (*http.Response, error)

func (function httpClientFunc) Do(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestProviderReturnsResetOnlySnapshotAndRequiredRequest(t *testing.T) {
	credentialPath := writeCredentialFile(t, "request-token", time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC), credentialScopePrefix+"profile")
	client := httpClientFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.String() != billingEndpoint {
			t.Errorf("request = %s %s", request.Method, request.URL)
		}
		if request.Header.Get("Authorization") != "Bearer request-token" ||
			request.Header.Get("Content-Type") != "application/grpc-web+proto" ||
			request.Header.Get("x-grpc-web") != "1" ||
			request.Header.Get("x-user-agent") != "connect-es/2.1.1" ||
			request.Header.Get("Origin") != "https://grok.com" ||
			request.Header.Get("Referer") != "https://grok.com/?_s=usage" {
			t.Errorf("required headers missing: %v", request.Header)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil || !bytes.Equal(body, emptyRequestFrame()) {
			t.Errorf("request body = %v, %v", body, err)
		}
		return response(http.StatusOK, fixture(t, "grok-billing-week.bin")), nil
	})
	provider := New(client, credentialPath)
	provider.now = func() time.Time { return time.Date(2030, 1, 2, 0, 0, 0, 0, time.UTC) }
	snapshot, err := provider.Refresh(context.Background())
	if err != nil || snapshot.Provider != model.ProviderGrok || snapshot.Plan != model.PlanUnknown || len(snapshot.Limits) != 1 {
		t.Fatalf("snapshot = %+v, err = %v", snapshot, err)
	}
	// This fixture carries no usage field, so the lane reports the reset
	// window and marks usage unknown rather than inventing a zero.
	if snapshot.Limits[0].UsedPercent != 0 || !snapshot.Limits[0].UsageUnknown {
		t.Fatalf("a usage-less response should read unknown: %+v", snapshot.Limits[0])
	}
}

func TestProviderErrorMappings(t *testing.T) {
	freshPath := writeCredentialFile(t, "mapping-token", time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC), credentialScopePrefix+"profile")
	expiredPath := writeCredentialFile(t, "expired-mapping-token", time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), credentialScopePrefix+"profile")
	missingPath := filepath.Join(t.TempDir(), "missing.json")
	tests := []struct {
		name       string
		path       string
		statusCode int
		body       []byte
		want       model.ErrorCode
	}{
		{name: "missing credential", path: missingPath, statusCode: http.StatusOK, body: fixture(t, "grok-billing-week.bin"), want: model.ErrNotLoggedIn},
		{name: "expired credential", path: expiredPath, statusCode: http.StatusOK, body: fixture(t, "grok-billing-week.bin"), want: model.ErrNotLoggedIn},
		{name: "empty body", path: freshPath, statusCode: http.StatusOK, body: fixture(t, "grok-billing-empty.bin"), want: model.ErrNotLoggedIn},
		{name: "HTTP 401", path: freshPath, statusCode: http.StatusUnauthorized, body: nil, want: model.ErrUnavailable},
		{name: "HTTP 500", path: freshPath, statusCode: http.StatusInternalServerError, body: nil, want: model.ErrUnavailable},
		{name: "truncated frame", path: freshPath, statusCode: http.StatusOK, body: fixture(t, "grok-billing-truncated.bin"), want: model.ErrInvalidResponse},
		{name: "garbage protobuf", path: freshPath, statusCode: http.StatusOK, body: fixture(t, "grok-billing-garbage.bin"), want: model.ErrInvalidResponse},
		{name: "trailer only", path: freshPath, statusCode: http.StatusOK, body: grpcWebFrameBytes(0x80, []byte("grpc-status: 0\r\n")), want: model.ErrInvalidResponse},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := httpClientFunc(func(*http.Request) (*http.Response, error) {
				return response(test.statusCode, test.body), nil
			})
			_, err := New(client, test.path).Refresh(context.Background())
			var safe model.SafeError
			if !errors.As(err, &safe) || safe.Code != test.want {
				t.Fatalf("error = %v, want %s", err, test.want)
			}
		})
	}
}

func TestProviderTransportErrorMappings(t *testing.T) {
	path := writeCredentialFile(t, "transport-token", time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC), credentialScopePrefix+"profile")
	tests := []struct {
		name string
		err  error
		want model.ErrorCode
	}{
		{name: "timeout", err: context.DeadlineExceeded, want: model.ErrTimeout},
		{name: "unavailable", err: errors.New("synthetic transport failure"), want: model.ErrUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := New(httpClientFunc(func(*http.Request) (*http.Response, error) {
				return nil, test.err
			}), path)
			_, err := provider.Refresh(context.Background())
			var safe model.SafeError
			if !errors.As(err, &safe) || safe.Code != test.want {
				t.Fatalf("error = %v, want %s", err, test.want)
			}
		})
	}
}

func TestProviderInvalidWindowReturnsSnapshotWithoutLane(t *testing.T) {
	now := time.Date(2030, 1, 2, 0, 0, 0, 0, time.UTC)
	payload := protoBillingPayload(now.Add(-8*24*time.Hour), now.Add(-25*time.Hour))
	body := grpcWebFrameBytes(0, payload)
	credentialPath := writeCredentialFile(t, "range-token", time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC), credentialScopePrefix+"profile")
	provider := New(httpClientFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusOK, body), nil
	}), credentialPath)
	provider.now = func() time.Time { return now }
	snapshot, err := provider.Refresh(context.Background())
	if err != nil || len(snapshot.Limits) != 0 || snapshot.Provider != model.ProviderGrok {
		t.Fatalf("snapshot = %+v, err = %v", snapshot, err)
	}
}

func TestProviderCredentialNeverReachesErrorOrLogAttributes(t *testing.T) {
	secret := "grok-secret-must-not-leak"
	credentialPath := writeCredentialFile(t, secret, time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC), credentialScopePrefix+"profile")
	provider := New(httpClientFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusInternalServerError, []byte(secret)), nil
	}), credentialPath)

	var logOutput bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logOutput, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	outcomes := (shared.Coordinator{Providers: map[model.ProviderID]model.Provider{
		model.ProviderGrok: provider,
	}}).RefreshAll(context.Background())
	err := outcomes[model.ProviderGrok].Err
	public := fmt.Sprintf("error=%v logs=%s snapshot=%+v", err, logOutput.String(), outcomes[model.ProviderGrok].Snapshot)
	if err == nil || strings.Contains(public, secret) {
		t.Fatalf("secret leaked or safe error missing: %s", public)
	}
}

func TestProviderInspectReconnectCloseAndDefaultTimeout(t *testing.T) {
	path := writeCredentialFile(t, "lifecycle-token", time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC), credentialScopePrefix+"profile")
	client := httpClientFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusOK, fixture(t, "grok-billing-week.bin")), nil
	})
	provider := New(client, path)
	provider.now = func() time.Time { return time.Date(2030, 1, 2, 0, 0, 0, 0, time.UTC) }
	if state := provider.Inspect(context.Background()); state.Status != model.StatusConnected || state.Source != "Grok CLI" {
		t.Fatalf("inspect state = %+v", state)
	}
	if _, err := provider.Reconnect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := provider.Close(); err != nil {
		t.Fatal(err)
	}

	defaultProvider := New(nil, path)
	httpClient, ok := defaultProvider.client.(*http.Client)
	if !ok || httpClient.Timeout != requestTimeout {
		t.Fatalf("default HTTP client = %#v", defaultProvider.client)
	}
}

func response(statusCode int, body []byte) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}
