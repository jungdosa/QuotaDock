package claude

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jungdosa/QuotaDock/internal/model"
	shared "github.com/jungdosa/QuotaDock/internal/provider"
)

// A third account reads through a browser fetcher of its own and reports as
// its own lane. It must never read through the second account's fetcher: the
// two are different sign-ins in different profile folders.
func TestAThirdAccountReadsThroughItsOwnFetcherAndReportsAsItself(t *testing.T) {
	provider := newProvider(&fakeClient{versionErr: shared.ErrNotInstalled}, nil, "2.0.0")
	second := fakeOAuthFetcher{available: true, result: oauthResult{raw: json.RawMessage(`{"five_hour":{"utilization":12,"resets_at":"2030-01-02T03:04:05Z"}}`)}}
	third := fakeOAuthFetcher{available: true, result: oauthResult{raw: json.RawMessage(`{"five_hour":{"utilization":77,"resets_at":"2030-01-02T03:04:05Z"}}`)}}
	provider.SetWebAuth(second)
	provider.SetAccountWebAuth(model.ProviderClaude3, third)

	additional := provider.AdditionalProviders()
	if len(additional) != 2 {
		t.Fatalf("exposed %d accounts, want the second and the third", len(additional))
	}
	account := additional[model.ProviderClaude3]
	if account == nil {
		t.Fatal("the third account is not exposed")
	}
	snapshot, err := account.Refresh(context.Background())
	if err != nil {
		t.Fatalf("third account refresh: %v", err)
	}
	if snapshot.Provider != model.ProviderClaude3 {
		t.Fatalf("third account reports as %s", snapshot.Provider)
	}
	if len(snapshot.Limits) != 1 || snapshot.Limits[0].UsedPercent != 77 {
		t.Fatalf("third account read the wrong figure: %+v", snapshot.Limits)
	}
	if state := account.Inspect(context.Background()); state.Status != model.StatusConnected || state.Source != model.SourceWebSignIn {
		t.Fatalf("third account state = %+v", state)
	}
	// The second account is untouched by the third being added.
	if snapshot, err := additional[model.ProviderClaudeAuth].Refresh(context.Background()); err != nil || snapshot.Limits[0].UsedPercent != 12 {
		t.Fatalf("second account read %+v (%v)", snapshot.Limits, err)
	}
}

// The first two accounts share the root provider's profile through SetWebAuth
// and are refused here, so no path can hand them a second profile by mistake.
func TestTheFirstTwoAccountsAreNotGivenProfilesOfTheirOwn(t *testing.T) {
	provider := newProvider(&fakeClient{versionErr: shared.ErrNotInstalled}, nil, "2.0.0")
	fetcher := fakeOAuthFetcher{available: true}
	provider.SetAccountWebAuth(model.ProviderClaude, fetcher)
	provider.SetAccountWebAuth(model.ProviderClaudeAuth, fetcher)
	provider.SetAccountWebAuth(model.ProviderCodex, fetcher)
	if additional := provider.AdditionalProviders(); additional != nil {
		t.Fatalf("accounts were created for ids that must not have their own profile: %v", additional)
	}
}

// A third account whose profile has never been signed in to says so, rather
// than reporting an error or borrowing the second account's session.
func TestAThirdAccountWithoutASignInAsksForOne(t *testing.T) {
	provider := newProvider(&fakeClient{versionErr: shared.ErrNotInstalled}, nil, "2.0.0")
	provider.SetWebAuth(fakeOAuthFetcher{available: true, result: oauthResult{raw: json.RawMessage(`{"five_hour":{"utilization":12,"resets_at":"2030-01-02T03:04:05Z"}}`)}})
	provider.SetAccountWebAuth(model.ProviderClaude4, fakeOAuthFetcher{available: false})
	account := provider.AdditionalProviders()[model.ProviderClaude4]
	if state := account.Inspect(context.Background()); state.Status != model.StatusLoggedOut || state.Error != model.ErrNotLoggedIn {
		t.Fatalf("an unsigned-in fourth account reports %+v", state)
	}
	if _, err := account.Refresh(context.Background()); err == nil {
		t.Fatal("an unsigned-in fourth account refreshed as if signed in")
	}
}

// Detaching a further account removes its lane; the others stay.
func TestDetachingAFurtherAccountRemovesOnlyThatLane(t *testing.T) {
	provider := newProvider(&fakeClient{versionErr: shared.ErrNotInstalled}, nil, "2.0.0")
	provider.SetWebAuth(fakeOAuthFetcher{available: true})
	provider.SetAccountWebAuth(model.ProviderClaude3, fakeOAuthFetcher{available: true})
	provider.SetAccountWebAuth(model.ProviderClaude5, fakeOAuthFetcher{available: true})
	provider.SetAccountWebAuth(model.ProviderClaude3, nil)
	additional := provider.AdditionalProviders()
	if _, still := additional[model.ProviderClaude3]; still {
		t.Fatal("the detached account is still exposed")
	}
	if additional[model.ProviderClaudeAuth] == nil || additional[model.ProviderClaude5] == nil {
		t.Fatalf("detaching one account removed others: %v", additional)
	}
}
