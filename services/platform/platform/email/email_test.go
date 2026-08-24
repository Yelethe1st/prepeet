package email

import (
	"context"
	"strings"
	"testing"
)

func TestConfigurationIsValidated(t *testing.T) {
	if _, err := New(Config{From: "noreply@prepeet.test"}); err == nil {
		t.Error("a transport with no address was accepted")
	}
	if _, err := New(Config{Address: "mail:1025"}); err == nil {
		t.Error("a transport with no from address was accepted")
	}
}

// The one attack this package is positioned to stop for every caller.
//
// A subject or recipient carrying CRLF becomes extra headers, and extra
// headers become extra recipients: the classic way a contact form turns into
// a spam relay. Refused before any connection is made, so the test needs no
// server.
func TestALineBreakIsRefusedBeforeAnythingIsSent(t *testing.T) {
	transport, err := New(Config{Address: "nowhere.invalid:1025", From: "noreply@prepeet.test"})
	if err != nil {
		t.Fatalf("building the transport: %v", err)
	}

	cases := map[string][2]string{
		"recipient": {"victim@example.test\r\nBcc: everyone@example.test", "subject"},
		"subject":   {"someone@example.test", "hello\r\nBcc: everyone@example.test"},
	}
	for name, pair := range cases {
		err := transport.Send(context.Background(), pair[0], pair[1], "body")
		if err == nil {
			t.Fatalf("a %s with an embedded header was sent", name)
		}
		if !strings.Contains(err.Error(), "line break") {
			// The refusal has to be the injection check, not a connection
			// failure to nowhere.invalid that would make this test pass while
			// checking nothing.
			t.Fatalf("the %s was refused for the wrong reason: %v", name, err)
		}
	}
}
