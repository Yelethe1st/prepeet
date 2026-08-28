package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The address a limit counts against, which an attacker must not be able
// to choose.

func request(remote string, forwarded string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	r.RemoteAddr = remote
	if forwarded != "" {
		r.Header.Set("X-Forwarded-For", forwarded)
	}
	return r
}

func TestWithoutATrustedProxyTheHeaderIsIgnored(t *testing.T) {
	// The whole point: a caller that can set the key can evade the limit,
	// so an untrusted deployment never reads the header at all.
	got := clientNetwork(request("203.0.113.9:51000", "198.51.100.1"), false)
	want := clientNetwork(request("203.0.113.9:51000", ""), false)
	if got != want || got != "203.0.113.0/24" {
		t.Fatalf("network = %q (unforwarded %q)", got, want)
	}
}

func TestBehindATrustedProxyTheLastEntryIsUsed(t *testing.T) {
	// An attacker sending their own X-Forwarded-For puts their invention
	// on the left; the proxy appends what it saw on the right. Counting
	// the last entry means the invention changes nothing.
	forged := clientNetwork(request("10.0.0.5:443", "1.2.3.4, 203.0.113.9"), true)
	honest := clientNetwork(request("10.0.0.5:443", "203.0.113.9"), true)
	if forged != honest || forged != "203.0.113.0/24" {
		t.Fatalf("forged %q honest %q", forged, honest)
	}
}

func TestATrustedProxyWithNoHeaderFallsBackToTheConnection(t *testing.T) {
	if got := clientNetwork(request("203.0.113.9:51000", ""), true); got != "203.0.113.0/24" {
		t.Fatalf("network = %q", got)
	}
}

func TestAddressesInOneNetworkShareACount(t *testing.T) {
	// One host churning through a subnet must not get a fresh allowance
	// per address.
	first := clientNetwork(request("203.0.113.9:1", ""), false)
	second := clientNetwork(request("203.0.113.240:1", ""), false)
	if first != second {
		t.Fatalf("%q and %q counted separately", first, second)
	}
	other := clientNetwork(request("203.0.114.9:1", ""), false)
	if other == first {
		t.Fatalf("different networks share a count: %q", other)
	}
}

func TestIPv6IsCountedByItsPrefixNotItsAddress(t *testing.T) {
	// An IPv6 client has more addresses than it could ever exhaust, so
	// per-address counting there is no limit at all.
	first := clientNetwork(request("[2001:db8:1:2::1]:443", ""), false)
	second := clientNetwork(request("[2001:db8:1:2::dead:beef]:443", ""), false)
	if first != second || first != "2001:db8:1:2::/64" {
		t.Fatalf("%q and %q", first, second)
	}
	if elsewhere := clientNetwork(request("[2001:db8:1:3::1]:443", ""), false); elsewhere == first {
		t.Fatal("different IPv6 networks share a count")
	}
}

func TestAnUnreadableAddressYieldsNothing(t *testing.T) {
	// Empty rather than a shared bucket: the limiter refuses an empty key
	// rather than collapsing every unidentifiable caller into one.
	if got := clientNetwork(request("not-an-address", ""), false); got != "" {
		t.Fatalf("network = %q, want empty", got)
	}
	if got := clientNetwork(request("garbage", "also-garbage"), true); got != "" {
		t.Fatalf("network = %q, want empty", got)
	}
}
