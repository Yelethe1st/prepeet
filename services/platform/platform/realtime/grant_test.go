package realtime_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/platform/realtime"
)

// The room grant, decoded and verified from the outside: what SES-02 calls
// "short-lived, session-scoped and non-reusable" has to be readable in the
// token's own claims, because the SFU enforces exactly what the claims say
// and nothing we meant but did not write.

func minter(t *testing.T) *realtime.Grants {
	t.Helper()
	grants, err := realtime.NewGrants(realtime.Config{
		URL: "wss://rtc.local", APIKey: "devkey", APISecret: "a-secret-of-sufficient-length",
	})
	if err != nil {
		t.Fatalf("NewGrants: %v", err)
	}
	return grants
}

// decode splits and decodes the JWT without a library, the way the SFU will.
func decode(t *testing.T, token string) (map[string]any, []byte, string) {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decoding claims: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("claims: %v", err)
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decoding signature: %v", err)
	}
	return claims, signature, parts[0] + "." + parts[1]
}

func TestTheGrantIsScopedToOneRoomAndOneIdentity(t *testing.T) {
	grant, err := minter(t).MintJoin(realtime.JoinRequest{
		Room: "ses_1", Identity: "usr_1", TTL: 2 * time.Minute,
	})
	if err != nil {
		t.Fatalf("MintJoin: %v", err)
	}

	claims, _, _ := decode(t, grant.Token)
	if claims["iss"] != "devkey" || claims["sub"] != "usr_1" {
		t.Fatalf("claims = %v", claims)
	}
	video, ok := claims["video"].(map[string]any)
	if !ok {
		t.Fatalf("no video grant in %v", claims)
	}
	if video["room"] != "ses_1" || video["roomJoin"] != true {
		t.Fatalf("video grant = %v", video)
	}
	// Scoped to joining THIS room: no room-create, no admin, nothing wider.
	for _, wider := range []string{"roomCreate", "roomAdmin", "roomList"} {
		if _, present := video[wider]; present {
			t.Errorf("the grant carries %s, which start has no business minting", wider)
		}
	}
	if grant.URL != "wss://rtc.local" || grant.Room != "ses_1" {
		t.Fatalf("grant = %+v", grant)
	}
}

func TestTheGrantIsShortLived(t *testing.T) {
	before := time.Now()
	grant, err := minter(t).MintJoin(realtime.JoinRequest{
		Room: "ses_1", Identity: "usr_1", TTL: 2 * time.Minute,
	})
	if err != nil {
		t.Fatalf("MintJoin: %v", err)
	}

	claims, _, _ := decode(t, grant.Token)
	exp := time.Unix(int64(claims["exp"].(float64)), 0)
	if exp.After(before.Add(3*time.Minute)) || exp.Before(before.Add(time.Minute)) {
		t.Fatalf("exp = %v, want about two minutes out", exp)
	}
	if !grant.ExpiresAt.Equal(exp) {
		t.Fatalf("the struct says %v, the token says %v; the client would trust the wrong clock", grant.ExpiresAt, exp)
	}
}

func TestTheSignatureIsRealHS256(t *testing.T) {
	grant, err := minter(t).MintJoin(realtime.JoinRequest{
		Room: "ses_1", Identity: "usr_1", TTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("MintJoin: %v", err)
	}

	_, signature, signed := decode(t, grant.Token)
	mac := hmac.New(sha256.New, []byte("a-secret-of-sufficient-length"))
	mac.Write([]byte(signed))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		t.Fatal("the signature does not verify against the configured secret")
	}
}

func TestMisconfigurationRefusesAtConstruction(t *testing.T) {
	cases := []realtime.Config{
		{URL: "", APIKey: "k", APISecret: "s-------------------------"},
		{URL: "wss://x", APIKey: "", APISecret: "s-------------------------"},
		{URL: "wss://x", APIKey: "k", APISecret: ""},
		{URL: "wss://x", APIKey: "k", APISecret: "short"},
	}
	for i, cfg := range cases {
		if _, err := realtime.NewGrants(cfg); err == nil {
			t.Errorf("case %d constructed with a broken config", i)
		}
	}
}

func TestARequestMissingItsScopeIsRefused(t *testing.T) {
	grants := minter(t)
	if _, err := grants.MintJoin(realtime.JoinRequest{Room: "", Identity: "u", TTL: time.Minute}); err == nil {
		t.Error("a grant without a room is a grant to nothing in particular")
	}
	if _, err := grants.MintJoin(realtime.JoinRequest{Room: "r", Identity: "", TTL: time.Minute}); err == nil {
		t.Error("a grant without an identity is anonymous authority")
	}
	if _, err := grants.MintJoin(realtime.JoinRequest{Room: "r", Identity: "u", TTL: 0}); err == nil {
		t.Error("a grant without a TTL never expires")
	}
	if _, err := grants.MintJoin(realtime.JoinRequest{Room: "r", Identity: "u", TTL: 24 * time.Hour}); err == nil {
		t.Error("a day-long join window is not short-lived under any reading")
	}
}
