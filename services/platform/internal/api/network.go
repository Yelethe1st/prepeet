package api

// Who the caller is, for the purpose of counting attempts.
//
// A rate limit keyed on something the caller chooses is not a rate limit,
// so this is deliberately careful about X-Forwarded-For. The header is
// attacker-supplied: anyone can send one, and trusting its first entry
// would let an attacker mint a fresh identity per request and never be
// limited at all.
//
// The rule here: the header is consulted only when the deployment says it
// sits behind a proxy it trusts, and then only its LAST entry, which is
// the address our own edge observed and appended. Everything to the left
// of that may be invention. Without that configuration the transport's
// own remote address is used, which nobody can forge.

import (
	"net"
	"net/http"
	"strings"
)

// networkPrefix reduces an address to the network it belongs to: a /24 for
// IPv4, a /64 for IPv6.
//
// The prefix rather than the address, because one attacker with many
// addresses is the ordinary case: a host churning through a subnet, or an
// IPv6 client with more addresses than it could ever exhaust, would defeat
// a per-address count while a per-network count still holds.
func networkPrefix(address string) string {
	parsed := net.ParseIP(address)
	if parsed == nil {
		return ""
	}
	if four := parsed.To4(); four != nil {
		return (&net.IPNet{IP: four.Mask(net.CIDRMask(24, 32)), Mask: net.CIDRMask(24, 32)}).String()
	}
	return (&net.IPNet{IP: parsed.Mask(net.CIDRMask(64, 128)), Mask: net.CIDRMask(64, 128)}).String()
}

// clientNetwork answers the network to count this request against.
//
// Empty when nothing usable can be determined, which the limiter refuses
// rather than collapsing every such caller into one bucket.
func clientNetwork(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			entries := strings.Split(forwarded, ",")
			// The last entry is what the trusted proxy appended: the
			// address it actually observed. Earlier entries came from the
			// caller and may be anything.
			last := strings.TrimSpace(entries[len(entries)-1])
			if prefix := networkPrefix(last); prefix != "" {
				return prefix
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return networkPrefix(strings.TrimSpace(host))
}
