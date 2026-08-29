package oidc_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/platform/oidc"
)

// IAM-08's provider half, against a provider we control. The properties worth
// asserting are the ones that decide who somebody is: that the verifier is
// presented, and that an unverified or absent email_verified is never read as
// verified.

// provider is a stand-in that answers like a real one and records what it was
// asked.
type provider struct {
	server    *httptest.Server
	tokenForm url.Values
	authHead  string
	userinfo  string
	tokenCode int
	tokenBody string
}

func newProvider(t *testing.T, userinfo string) *provider {
	t.Helper()
	p := &provider{userinfo: userinfo, tokenCode: http.StatusOK,
		tokenBody: `{"access_token":"at_1234"}`}
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		p.tokenForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(p.tokenCode)
		_, _ = w.Write([]byte(p.tokenBody))
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		p.authHead = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(p.userinfo))
	})
	p.server = httptest.NewServer(mux)
	t.Cleanup(p.server.Close)
	return p
}

func (p *provider) client() *oidc.Client {
	return oidc.New(oidc.Config{
		AuthorizeURL: p.server.URL + "/authorize",
		TokenURL:     p.server.URL + "/token",
		UserInfoURL:  p.server.URL + "/userinfo",
		ClientID:     "client-id", ClientSecret: "client-secret",
		RedirectURI: "https://app.example/auth/callback",
	})
}

func TestTheAuthorizationUrlCarriesTheChallengeAndTheMethod(t *testing.T) {
	client := newProvider(t, "{}").client()

	raw := client.AuthorizationURL("the-state", "the-challenge")

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	query := parsed.Query()
	if query.Get("state") != "the-state" {
		t.Fatalf("state = %q", query.Get("state"))
	}
	if query.Get("code_challenge") != "the-challenge" {
		t.Fatalf("challenge = %q", query.Get("code_challenge"))
	}
	// S256, never plain: a "plain" challenge is the verifier itself, so it
	// protects against nothing, and providers accept it.
	if query.Get("code_challenge_method") != "S256" {
		t.Fatalf("method = %q, want S256", query.Get("code_challenge_method"))
	}
	// The verifier must not be here. It is the half held back.
	if strings.Contains(raw, "code_verifier") {
		t.Fatalf("the verifier was sent to the authorization endpoint: %s", raw)
	}
}

func TestExchangePresentsTheVerifierAndReadsTheClaims(t *testing.T) {
	p := newProvider(t, `{"sub":"1234","email":"daniel@example.com","email_verified":true}`)

	got, err := p.client().Exchange(context.Background(), "the-code", "the-verifier")
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}

	if p.tokenForm.Get("code_verifier") != "the-verifier" {
		t.Fatalf("the verifier was not presented: %v", p.tokenForm)
	}
	if p.tokenForm.Get("grant_type") != "authorization_code" {
		t.Fatalf("grant_type = %q", p.tokenForm.Get("grant_type"))
	}
	if p.authHead != "Bearer at_1234" {
		t.Fatalf("userinfo was called as %q", p.authHead)
	}
	if got.Subject != "1234" || got.Email != "daniel@example.com" || !got.EmailVerified {
		t.Fatalf("claims came back as %+v", got)
	}
}

// The line the whole linking rule rests on.
func TestAnAbsentEmailVerifiedIsNotVerified(t *testing.T) {
	p := newProvider(t, `{"sub":"1234","email":"daniel@example.com"}`)

	got, err := p.client().Exchange(context.Background(), "the-code", "the-verifier")
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}

	// Microsoft omits this for personal accounts, where the address may be one
	// the holder simply typed. Reading absence as true would let anybody set an
	// address at such a provider and link to somebody else's account here.
	if got.EmailVerified {
		t.Fatal("an absent email_verified was read as verified")
	}
}

func TestAnExplicitlyUnverifiedEmailIsNotVerified(t *testing.T) {
	p := newProvider(t, `{"sub":"1234","email":"daniel@example.com","email_verified":false}`)

	got, err := p.client().Exchange(context.Background(), "the-code", "the-verifier")
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}

	if got.EmailVerified {
		t.Fatal("email_verified:false was read as verified")
	}
}

func TestARefusedCodeCarriesTheProvidersOwnWords(t *testing.T) {
	p := newProvider(t, "{}")
	p.tokenCode = http.StatusBadRequest
	p.tokenBody = `{"error":"invalid_grant","error_description":"Code was already redeemed."}`

	_, err := p.client().Exchange(context.Background(), "the-code", "the-verifier")

	if err == nil {
		t.Fatal("a refused code succeeded")
	}
	// invalid_grant on a reused code reads very differently from
	// redirect_uri_mismatch on a misconfigured deployment, and an operator
	// needs to be able to tell them apart.
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Fatalf("the provider's reason was lost: %v", err)
	}
}

func TestUserinfoWithoutASubjectIsRefused(t *testing.T) {
	p := newProvider(t, `{"email":"daniel@example.com","email_verified":true}`)

	_, err := p.client().Exchange(context.Background(), "the-code", "the-verifier")

	// Without a subject there is nothing stable to key the link on, and
	// falling back to the email is exactly what the schema forbids.
	if err == nil {
		t.Fatal("userinfo without a subject was accepted")
	}
}

func TestAHalfConfiguredProviderIsNotConfigured(t *testing.T) {
	for _, missing := range []oidc.Config{
		{TokenURL: "t", UserInfoURL: "u", AuthorizeURL: "a", ClientSecret: "s", RedirectURI: "r"},
		{ClientID: "c", UserInfoURL: "u", AuthorizeURL: "a", ClientSecret: "s", RedirectURI: "r"},
		{ClientID: "c", TokenURL: "t", UserInfoURL: "u", AuthorizeURL: "a", RedirectURI: "r"},
		{ClientID: "c", TokenURL: "t", UserInfoURL: "u", AuthorizeURL: "a", ClientSecret: "s"},
	} {
		if missing.Configured() {
			t.Fatalf("a provider missing a field reported itself configured: %+v", missing)
		}
	}

	complete := oidc.Config{
		ClientID: "c", ClientSecret: "s", AuthorizeURL: "a",
		TokenURL: "t", UserInfoURL: "u", RedirectURI: "r",
	}
	if !complete.Configured() {
		t.Fatal("a complete provider reported itself unconfigured")
	}
}
