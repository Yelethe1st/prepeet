// Package realtime mints LiveKit room grants, to ADR-0012.
//
// A grant is a signed statement of exactly what the SFU may allow: one
// identity, one room, one short join window. Go mints it at session start
// (SES-02); the browser presents it; the SFU enforces the claims and
// nothing else. There is deliberately no wider grant in this package - no
// room creation, no admin, no listing - because start has no business
// minting authority start does not need.
//
// The token is a plain HS256 JWT built with the standard library. A JWT
// dependency would buy nothing here: the shape is fixed by LiveKit's
// contract, and the whole value of the token is that its claims are exactly
// what we wrote.
package realtime

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// maxJoinTTL bounds the join window. A grant is for joining now, not for
// keeping in a drawer; reconnection mints a fresh one.
const maxJoinTTL = 10 * time.Minute

// minSecretLength refuses secrets that are effectively guessable. LiveKit's
// own tooling generates far longer.
const minSecretLength = 16

// Config is the SFU's address and the signing credentials.
type Config struct {
	// URL is what the browser dials, such as wss://rtc.example.com.
	URL       string
	APIKey    string
	APISecret string
}

// JoinRequest names the one thing the grant admits.
type JoinRequest struct {
	// Room is the room name; SES-02 uses the session id, so a grant cannot
	// name a room that is not a session.
	Room     string
	Identity string
	// TTL is the join window, bounded by maxJoinTTL.
	TTL time.Duration
	// Metadata rides the participant, visible to the agent: attempt
	// numbers, accommodation flags. Optional.
	Metadata string
}

// Grant is what the browser needs to join, and nothing more.
type Grant struct {
	URL       string
	Room      string
	Token     string
	ExpiresAt time.Time
}

// Grants mints join tokens.
type Grants struct {
	config Config
	// now is swappable for tests.
	now func() time.Time
}

// NewGrants validates the configuration once, at wiring time, so a broken
// deployment fails at startup rather than at the first interview.
func NewGrants(config Config) (*Grants, error) {
	switch {
	case config.URL == "":
		return nil, errors.New("realtime: a URL is required")
	case config.APIKey == "":
		return nil, errors.New("realtime: an API key is required")
	case len(config.APISecret) < minSecretLength:
		return nil, fmt.Errorf("realtime: the API secret must be at least %d characters", minSecretLength)
	}
	return &Grants{config: config, now: time.Now}, nil
}

// MintJoin signs one join grant.
func (g *Grants) MintJoin(request JoinRequest) (Grant, error) {
	switch {
	case request.Room == "":
		return Grant{}, errors.New("realtime: a grant without a room is a grant to nothing in particular")
	case request.Identity == "":
		return Grant{}, errors.New("realtime: a grant without an identity is anonymous authority")
	case request.TTL <= 0:
		return Grant{}, errors.New("realtime: a grant without a TTL never expires")
	case request.TTL > maxJoinTTL:
		return Grant{}, fmt.Errorf("realtime: the join window is at most %s", maxJoinTTL)
	}

	issued := g.now().Truncate(time.Second)
	expires := issued.Add(request.TTL)

	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	yes := true
	claims := map[string]any{
		"iss": g.config.APIKey,
		"sub": request.Identity,
		// A small backdated nbf absorbs clock skew between us and the SFU.
		"nbf": issued.Add(-10 * time.Second).Unix(),
		"exp": expires.Unix(),
		"video": map[string]any{
			"room":         request.Room,
			"roomJoin":     yes,
			"canPublish":   yes,
			"canSubscribe": yes,
		},
	}
	if request.Metadata != "" {
		claims["metadata"] = request.Metadata
	}

	signed, err := signHS256(header, claims, g.config.APISecret)
	if err != nil {
		return Grant{}, err
	}
	return Grant{
		URL: g.config.URL, Room: request.Room,
		Token: signed, ExpiresAt: expires,
	}, nil
}

func signHS256(header map[string]string, claims map[string]any, secret string) (string, error) {
	encode := func(v any) (string, error) {
		raw, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("realtime: encoding token: %w", err)
		}
		return base64.RawURLEncoding.EncodeToString(raw), nil
	}
	head, err := encode(header)
	if err != nil {
		return "", err
	}
	body, err := encode(claims)
	if err != nil {
		return "", err
	}
	signing := head + "." + body
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signing))
	return signing + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
