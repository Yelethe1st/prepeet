package email_test

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/platform/email"
)

// relay is an SMTP endpoint that speaks just enough of the conversation to be
// refused or completed, and never offers STARTTLS.
//
// A real relay is the only way to answer the question this file asks, which is
// what the transport does when the endpoint cannot be upgraded. The magic link
// and the verification token in these messages are credentials: a message
// carrying one across a network in the clear is that credential published to
// anything on the path.
func relay(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go converse(conn)
		}
	}()
	return listener.Addr().String()
}

// converse answers the commands the client sends, advertising no extensions.
func converse(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	reader := bufio.NewReader(conn)
	write := func(line string) bool {
		_, err := conn.Write([]byte(line + "\r\n"))
		return err == nil
	}
	if !write("220 relay.test ESMTP") {
		return
	}
	inData := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		command := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case inData:
			if strings.TrimSpace(line) == "." {
				inData = false
				write("250 accepted")
			}
		case strings.HasPrefix(command, "EHLO"):
			// No STARTTLS line: this endpoint cannot be upgraded. AUTH is
			// offered and accepted so that a test about credentials fails on
			// the transport's own refusal rather than on this relay declining
			// a command it never implemented. Go's client permits PLAIN over
			// an unencrypted connection to a loopback address, so nothing but
			// the transport stands between the password and the wire here.
			write("250-relay.test")
			write("250-AUTH PLAIN LOGIN")
			write("250 SIZE 10240000")
		case strings.HasPrefix(command, "AUTH"):
			write("235 authenticated")
		case strings.HasPrefix(command, "HELO"):
			write("250 relay.test")
		case strings.HasPrefix(command, "MAIL"), strings.HasPrefix(command, "RCPT"):
			write("250 ok")
		case strings.HasPrefix(command, "DATA"):
			inData = true
			write("354 go ahead")
		case strings.HasPrefix(command, "QUIT"):
			write("221 bye")
			return
		default:
			write("250 ok")
		}
	}
}

func send(t *testing.T, config email.Config) error {
	t.Helper()
	transport, err := email.New(config)
	if err != nil {
		return err
	}
	return transport.Send(context.Background(), "candidate@example.com",
		"Your sign-in link", "https://prepeet.test/magic/mgc_aaaaaaaaaaaaaaaaaaaa")
}

func TestAnUnupgradableRelayIsRefusedByDefault(t *testing.T) {
	t.Parallel()

	// The gap this closes: with no credentials configured there was nothing to
	// protect a password, so the send went ahead in the clear. The message
	// body is a credential too.
	err := send(t, email.Config{Address: relay(t), From: "noreply@prepeet.test"})

	if err == nil {
		t.Fatal("a message went out over a connection that was never encrypted")
	}
	if !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("the error does not name the cause: %v", err)
	}
}

func TestAnUnupgradableRelayIsAllowedWhenDeclared(t *testing.T) {
	t.Parallel()

	// Mailpit in the local stack offers no STARTTLS, and `make dev` must keep
	// working. The declaration is how a deployment says the endpoint is
	// reached over a network it trusts.
	err := send(t, email.Config{
		Address:        relay(t),
		From:           "noreply@prepeet.test",
		AllowPlaintext: true,
	})
	if err != nil {
		t.Fatalf("a declared plaintext relay was still refused: %v", err)
	}
}

func TestCredentialsAreNeverSentInTheClearEvenWhenDeclared(t *testing.T) {
	t.Parallel()

	// The declaration covers the message. It does not cover a password, which
	// is a credential for the relay itself and would be replayable by anyone
	// on the path.
	err := send(t, email.Config{
		Address:        relay(t),
		From:           "noreply@prepeet.test",
		Username:       "prepeet",
		Password:       "hunter2",
		AllowPlaintext: true,
	})

	if err == nil {
		t.Fatal("a password was sent over an unencrypted connection")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Fatal("the refusal quotes the password it was protecting")
	}
}
