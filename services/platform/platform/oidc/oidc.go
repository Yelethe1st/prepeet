// Package oidc is one OpenID Connect client, configured per provider.
//
// IAM-08 asks that adding a provider be configuration and a test rather than
// a release, so there is no Google type and no Microsoft type: there is one
// client and a Config that names the endpoints. Google and Microsoft differ in
// their URLs and in almost nothing else that matters here.
//
// It deliberately does not verify an ID token. The code is exchanged over TLS
// at the provider's own token endpoint and the claims are then read from the
// provider's own userinfo endpoint over TLS with the access token, so the
// answers arrive from the issuer directly rather than in a bearer artefact
// that has to be validated. That trades one round trip for not shipping a JWT
// verifier, a JWKS cache and a clock-skew policy, all of which are ways to be
// subtly wrong about who somebody is.
package oidc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config is one provider, as a deployment describes it.
type Config struct {
	// AuthorizeURL, TokenURL and UserInfoURL are the provider's endpoints.
	AuthorizeURL string
	TokenURL     string
	UserInfoURL  string

	ClientID     string
	ClientSecret string
	// RedirectURI must match what is registered with the provider exactly,
	// including its scheme and any trailing slash. Providers compare strings.
	RedirectURI string
	// Scopes defaults to openid, email and profile: the least that yields a
	// subject and an address.
	Scopes []string
}

// Configured reports whether this provider can be used.
//
// A provider missing its client id or secret is not half-configured, it is
// absent: offering the button and failing at the token endpoint is a worse
// experience than not offering it, and this is what keeps such a provider out
// of the list the sign-in screen draws from.
func (c Config) Configured() bool {
	return c.ClientID != "" && c.ClientSecret != "" &&
		c.AuthorizeURL != "" && c.TokenURL != "" && c.UserInfoURL != "" && c.RedirectURI != ""
}

// Client talks to one provider.
type Client struct {
	config Config
	http   *http.Client
}

// New builds a client. The timeout is short on purpose: a person is waiting on
// a redirect, and a provider that has not answered in ten seconds is not going
// to answer usefully.
func New(config Config) *Client {
	if len(config.Scopes) == 0 {
		config.Scopes = []string{"openid", "email", "profile"}
	}
	return &Client{config: config, http: &http.Client{Timeout: 10 * time.Second}}
}

// AuthorizationURL is where to send the browser.
func (c *Client) AuthorizationURL(state, codeChallenge string) string {
	query := url.Values{}
	query.Set("client_id", c.config.ClientID)
	query.Set("redirect_uri", c.config.RedirectURI)
	query.Set("response_type", "code")
	query.Set("scope", strings.Join(c.config.Scopes, " "))
	query.Set("state", state)
	query.Set("code_challenge", codeChallenge)
	// S256 rather than plain. A "plain" challenge is the verifier, so it
	// protects against nothing, and providers accept it.
	query.Set("code_challenge_method", "S256")

	separator := "?"
	if strings.Contains(c.config.AuthorizeURL, "?") {
		separator = "&"
	}
	return c.config.AuthorizeURL + separator + query.Encode()
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

type userInfo struct {
	Subject string `json:"sub"`
	Email   string `json:"email"`
	// A pointer, so "absent" is distinguishable from "present and false".
	// They are treated the same way, and the distinction still matters: it is
	// the difference between a provider that says no and one that was never
	// asked, and only the first is a fact about the account.
	EmailVerified *bool `json:"email_verified"`
}

// Identity is what the provider told us.
type Identity struct {
	Subject       string
	Email         string
	EmailVerified bool
}

// Exchange turns the authorisation code into the provider's claims.
func (c *Client) Exchange(ctx context.Context, code, codeVerifier string) (Identity, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("code_verifier", codeVerifier)
	form.Set("client_id", c.config.ClientID)
	form.Set("client_secret", c.config.ClientSecret)
	form.Set("redirect_uri", c.config.RedirectURI)

	token, err := c.token(ctx, form)
	if err != nil {
		return Identity{}, err
	}
	return c.claims(ctx, token)
}

func (c *Client) token(ctx context.Context, form url.Values) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.TokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("oidc: building the token request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")

	response, err := c.http.Do(request)
	if err != nil {
		return "", fmt.Errorf("oidc: reaching the token endpoint: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	// Bounded: a provider that answers with a gigabyte is a provider that
	// takes the process down, and nothing here needs more than a few hundred
	// bytes.
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("oidc: reading the token response: %w", err)
	}

	var decoded tokenResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", fmt.Errorf("oidc: the token response was not json: %w", err)
	}
	if response.StatusCode != http.StatusOK || decoded.Error != "" {
		// The provider's own words, which are what makes this diagnosable:
		// invalid_grant on a reused code reads very differently from
		// redirect_uri_mismatch on a misconfigured deployment.
		return "", fmt.Errorf("oidc: the provider refused the code: %s %s",
			decoded.Error, decoded.Description)
	}
	if decoded.AccessToken == "" {
		return "", fmt.Errorf("oidc: the provider returned no access token")
	}
	return decoded.AccessToken, nil
}

func (c *Client) claims(ctx context.Context, accessToken string) (Identity, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.config.UserInfoURL, nil)
	if err != nil {
		return Identity{}, fmt.Errorf("oidc: building the userinfo request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Accept", "application/json")

	response, err := c.http.Do(request)
	if err != nil {
		return Identity{}, fmt.Errorf("oidc: reaching the userinfo endpoint: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return Identity{}, fmt.Errorf("oidc: reading the userinfo response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return Identity{}, fmt.Errorf("oidc: userinfo answered %d", response.StatusCode)
	}

	var decoded userInfo
	if err := json.Unmarshal(body, &decoded); err != nil {
		return Identity{}, fmt.Errorf("oidc: the userinfo response was not json: %w", err)
	}
	if decoded.Subject == "" {
		return Identity{}, fmt.Errorf("oidc: userinfo carried no subject")
	}

	// Fails closed, and this is the line the whole linking rule rests on.
	//
	// A provider that does not send email_verified has not verified anything
	// we know about. Microsoft in particular omits it for personal accounts,
	// where the address may be one the holder simply typed. Reading absence as
	// "true" would let anybody set an address at such a provider and have it
	// link to somebody else's account here.
	verified := decoded.EmailVerified != nil && *decoded.EmailVerified

	return Identity{Subject: decoded.Subject, Email: decoded.Email, EmailVerified: verified}, nil
}
