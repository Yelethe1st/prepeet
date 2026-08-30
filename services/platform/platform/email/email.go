// Package email speaks SMTP.
//
// SMTP rather than a provider SDK, and that is a reversibility decision as
// much as a simplicity one: SES, Postmark, Mailgun and a local Mailpit all
// accept the same conversation, so switching providers is a configuration
// change and ADR-0001's vendor-confinement rule has nothing to confine.
//
// Implements the transport half of INT-01.
package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// Config is what connecting and identifying takes.
type Config struct {
	// Address is host:port of the SMTP endpoint.
	Address string
	// From is the sender address on every message.
	From string
	// Username and Password authenticate where the endpoint requires it;
	// both empty means unauthenticated, which only a local endpoint accepts.
	Username string
	Password string
	// AllowPlaintext permits sending to an endpoint that offers no STARTTLS.
	//
	// The transport upgrades whenever the endpoint offers it, and used to send
	// in the clear when it did not, refusing only if a password would have
	// gone with it. That protected the relay's credential and not the message,
	// and the message is a credential: it carries the magic link or the
	// verification token that signs somebody in.
	//
	// Mailpit in the local stack offers no STARTTLS, so this exists rather
	// than a flat refusal. It is a declaration, not a default, and it never
	// covers a password: see Send.
	AllowPlaintext bool
}

// SMTP sends mail over one configured endpoint.
type SMTP struct {
	config Config
}

// New validates the configuration and builds the transport.
func New(config Config) (*SMTP, error) {
	if config.Address == "" {
		return nil, errors.New("email: an SMTP address is required")
	}
	if config.From == "" {
		return nil, errors.New("email: a from address is required")
	}
	return &SMTP{config: config}, nil
}

// sendTimeout bounds one SMTP conversation. Generous for a slow relay, far
// below the queue's claim visibility so a hung conversation cannot make a
// claimed email look abandoned while still being worked on.
const sendTimeout = 30 * time.Second

// Send delivers one plain-text message.
//
// The header block is assembled here and nowhere else, and the recipient and
// subject are refused if they carry line breaks: a subject with an embedded
// CRLF is the header-injection attack, and this is the one place it can be
// stopped for every caller.
func (s *SMTP) Send(ctx context.Context, recipient, subject, body string) error {
	for name, value := range map[string]string{"recipient": recipient, "subject": subject} {
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("email: the %s contains a line break, which would inject a header", name)
		}
	}

	deadline := time.Now().Add(sendTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}

	conn, err := net.DialTimeout("tcp", s.config.Address, sendTimeout)
	if err != nil {
		return fmt.Errorf("email: connecting to %s: %w", s.config.Address, err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("email: setting the conversation deadline: %w", err)
	}

	host, _, err := net.SplitHostPort(s.config.Address)
	if err != nil {
		return fmt.Errorf("email: reading the SMTP host: %w", err)
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("email: greeting %s: %w", host, err)
	}
	defer client.Close()

	// STARTTLS whenever the endpoint offers it. When it does not, the send is
	// refused unless the deployment declared plaintext acceptable, and it is
	// refused regardless once a password would go with it: the declaration is
	// about this message crossing a trusted network, and a relay credential
	// crossing the same network is replayable by anyone who sees it.
	upgraded := false
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("email: starting TLS: %w", err)
		}
		upgraded = true
	}
	if !upgraded {
		if s.config.Username != "" {
			return errors.New("email: the endpoint offers no STARTTLS and credentials are configured; " +
				"refusing to send a password in the clear")
		}
		if !s.config.AllowPlaintext {
			// The body carries a sign-in link often enough that this is a
			// credential leak, not a privacy preference.
			return errors.New("email: the endpoint offers no STARTTLS; refusing to send a message " +
				"that may carry a sign-in link over an unencrypted connection")
		}
	}

	if s.config.Username != "" {
		auth := smtp.PlainAuth("", s.config.Username, s.config.Password, host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("email: authenticating: %w", err)
		}
	}

	if err := client.Mail(s.config.From); err != nil {
		return fmt.Errorf("email: stating the sender: %w", err)
	}
	if err := client.Rcpt(recipient); err != nil {
		return fmt.Errorf("email: stating the recipient: %w", err)
	}

	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("email: opening the message: %w", err)
	}
	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s",
		s.config.From, recipient, subject, body)
	if _, err := writer.Write([]byte(message)); err != nil {
		return fmt.Errorf("email: writing the message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("email: finishing the message: %w", err)
	}

	return client.Quit()
}
