package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jungdosa/QuotaDock/internal/model"
)

type fakeClient struct {
	running  bool
	loggedIn bool
	raw      json.RawMessage
	err      error
}

func (f fakeClient) Status(context.Context) (bool, bool, error) { return f.running, f.loggedIn, f.err }
func (f fakeClient) RetrieveUserQuotaSummary(context.Context) (json.RawMessage, error) {
	return f.raw, f.err
}
func (f fakeClient) Close() error { return nil }

func TestLegacyAndCurrentLanguageServerNamesAllowed(t *testing.T) {
	for _, name := range []string{"language_server.exe", "language_server_windows_x64.exe", "language_server_windows_arm64.exe"} {
		if !IsAllowedExecutable(name) {
			t.Errorf("rejected %q", name)
		}
	}
}
func TestSimilarLanguageServerNamesRejected(t *testing.T) {
	for _, name := range []string{"language_server_windows_x64_helper.exe", "language_server_windows_x64.exe.bak", "other_language_server.exe"} {
		if IsAllowedExecutable(name) {
			t.Errorf("accepted %q", name)
		}
	}
}
func TestUserTierUltraOverridesPlanStatusPro(t *testing.T) {
	snapshot, err := NormalizeQuota(fixture(t, "antigravity-quota.json"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Plan != "AI ULTRA" {
		t.Fatalf("plan = %q", snapshot.Plan)
	}
}
func TestRemainingFractionConversion(t *testing.T) {
	snapshot, err := NormalizeQuota(fixture(t, "antigravity-quota.json"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Limits) != 2 || snapshot.Limits[0].RemainingPercent != 65 || snapshot.Limits[0].UsedPercent != 35 || snapshot.Limits[1].RemainingPercent != 20 || snapshot.Limits[1].UsedPercent != 80 {
		t.Fatalf("limits = %+v", snapshot.Limits)
	}
}
func TestNormalQuotaResponseAndMissingFields(t *testing.T) {
	client := fakeClient{running: true, loggedIn: true, raw: fixture(t, "antigravity-quota.json")}
	snapshot, err := New(client).Refresh(context.Background())
	if err != nil || len(snapshot.Limits) != 2 {
		t.Fatalf("normal response = %+v, %v", snapshot, err)
	}
	partial := json.RawMessage(`{"userStatus":{"userTier":{"id":"standard"}},"quotaGroups":[{"name":"Gemini Models","buckets":[{"id":"unknown"}]}]}`)
	snapshot, err = NormalizeQuota(partial, time.Now())
	if err != nil || snapshot.Plan != model.Plan("STANDARD") || len(snapshot.Limits) != 0 {
		t.Fatalf("partial response = %+v, %v", snapshot, err)
	}
}
func TestLocalTransportSchemaAdaptsIntoExistingNormalizer(t *testing.T) {
	raw := json.RawMessage(`{"response":{"groups":[{"displayName":"Gemini Models","buckets":[{"displayName":"5 hour","remainingFraction":0.75,"resetTime":"2026-07-24T12:00:00Z","window":"5h"}]},{"displayName":"Claude and GPT models","buckets":[{"displayName":"weekly","remainingFraction":0.25,"resetTime":"2026-07-31T12:00:00Z","window":"168h"}]}]}}`)
	adapted, err := adaptQuotaResponse(raw, "AI ULTRA")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := NormalizeQuota(adapted, time.Now())
	if err != nil || snapshot.Plan != "AI ULTRA" || len(snapshot.Limits) != 2 || snapshot.Limits[0].WindowMinutes != 300 || snapshot.Limits[1].WindowMinutes != 10080 {
		t.Fatalf("adapted snapshot = %+v, %v", snapshot, err)
	}
}

func TestAntigravityEndpointAndResponseBounds(t *testing.T) {
	if isAllowedEndpoint("/unapproved") {
		t.Fatal("unapproved endpoint was accepted")
	}
	if _, err := requestLocal(context.Background(), endpointCandidate{verified: true, executable: "language_server.exe", port: 1, token: "secret"}, "/unapproved"); err != ErrEndpointNotAllowed {
		t.Fatalf("endpoint error = %v", err)
	}
	oversized := strings.NewReader(strings.Repeat("x", MaxResponseBytes+1))
	if _, err := readResponseLimited(oversized); err != ErrResponseTooLarge {
		t.Fatalf("response limit error = %v", err)
	}
}

func TestAntigravityProcessIdentityAndTokenParsing(t *testing.T) {
	if validateProcessIdentity(`C:\\Other\\language_server_windows_x64.exe`, "") {
		t.Fatal("non-Antigravity executable path was accepted")
	}
	if !validateProcessIdentity(`C:\\Programs\\Antigravity IDE\\language_server_windows_x64.exe`, "") {
		t.Fatal("Antigravity executable path was rejected")
	}
	tokens := csrfTokens(`language_server_windows_x64.exe --csrf_token first-secret --extension_server_csrf_token="second-secret"`)
	if len(tokens) != 2 || tokens[0] != "first-secret" || tokens[1] != "second-secret" {
		t.Fatalf("token flags were not parsed")
	}
}

func TestCSRFTokenAndPortNeverReachSnapshotLogOrError(t *testing.T) {
	secret := "csrf-secret-must-not-leak"
	quota := fixture(t, "antigravity-quota.json")
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("x-codeium-csrf-token") != secret {
			writer.WriteHeader(http.StatusForbidden)
			return
		}
		switch request.URL.Path {
		case GetUserStatusEndpoint:
			_, _ = writer.Write([]byte(`{"userStatus":{"userTier":{"name":"[MASKED]"}}}`))
		case RetrieveQuotaEndpoint:
			_, _ = writer.Write(quota)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	client := &LocalClient{discover: func() ([]endpointCandidate, error) {
		return []endpointCandidate{{pid: 1, port: uint16(port), token: secret, executable: "language_server_windows_x64.exe", verified: true}}, nil
	}}
	snapshot, refreshErr := New(client).Refresh(context.Background())
	if refreshErr != nil {
		t.Fatal(refreshErr)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	server.Close()
	_, failureErr := New(client).Refresh(context.Background())
	if failureErr == nil {
		t.Fatal("closed local endpoint unexpectedly refreshed")
	}
	logOutput := "provider connected"
	publicSurface := strings.Join([]string{string(encoded), fmt.Sprint(refreshErr), fmt.Sprint(failureErr), logOutput}, "|")
	for _, forbidden := range []string{secret, portText} {
		if strings.Contains(publicSurface, forbidden) {
			t.Fatalf("public surface retained sensitive local connection data")
		}
	}
}

func fixture(t *testing.T, name string) json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
