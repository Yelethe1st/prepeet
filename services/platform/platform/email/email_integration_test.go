//go:build integration

// The SMTP conversation against a real server.
//
// Mailpit accepts SMTP and exposes what it received over HTTP, so the
// assertion is on what a mailbox actually got: recipient, subject, body,
// sender. A unit test around net/smtp would assert the conversation we
// believe we are having, which is exactly the thing worth doubting.
package email_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Yelethe1st/prepeet/services/platform/platform/email"
)

var (
	smtpAddress string
	apiBase     string
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "axllent/mailpit:v1.21",
			ExposedPorts: []string{"1025/tcp", "8025/tcp"},
			WaitingFor:   wait.ForListeningPort("8025/tcp").WithStartupTimeout(time.Minute),
		},
		Started: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "starting Mailpit: %v\n", err)
		os.Exit(1)
	}

	host, err := container.Host(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "container host: %v\n", err)
		os.Exit(1)
	}
	smtpPort, err := container.MappedPort(ctx, "1025/tcp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "smtp port: %v\n", err)
		os.Exit(1)
	}
	apiPort, err := container.MappedPort(ctx, "8025/tcp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "api port: %v\n", err)
		os.Exit(1)
	}
	smtpAddress = fmt.Sprintf("%s:%s", host, smtpPort.Port())
	apiBase = fmt.Sprintf("http://%s:%s", host, apiPort.Port())

	code := m.Run()
	if err := container.Terminate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "terminating Mailpit: %v\n", err)
	}
	os.Exit(code)
}

// received is the slice of Mailpit's message listing these tests read.
type received struct {
	Messages []struct {
		To      []struct{ Address string }
		From    struct{ Address string }
		Subject string
	}
}

func TestAMessageArrivesAsComposed(t *testing.T) {
	transport, err := email.New(email.Config{
		Address: smtpAddress,
		From:    "noreply@prepeet.test",
	})
	if err != nil {
		t.Fatalf("building the transport: %v", err)
	}

	if err := transport.Send(t.Context(),
		"candidate@example.test",
		"Verify your email address",
		"Confirm your email address:\n\nhttps://app.example.test/verify?token=T\n"); err != nil {
		t.Fatalf("sending: %v", err)
	}

	resp, err := http.Get(apiBase + "/api/v1/messages")
	if err != nil {
		t.Fatalf("reading the mailbox: %v", err)
	}
	defer resp.Body.Close()

	var inbox received
	if err := json.NewDecoder(resp.Body).Decode(&inbox); err != nil {
		t.Fatalf("decoding the mailbox: %v", err)
	}
	if len(inbox.Messages) == 0 {
		t.Fatal("the mailbox is empty; the conversation reported success for a message that never arrived")
	}

	message := inbox.Messages[0]
	if message.To[0].Address != "candidate@example.test" {
		t.Errorf("delivered to %q", message.To[0].Address)
	}
	if message.From.Address != "noreply@prepeet.test" {
		t.Errorf("from %q", message.From.Address)
	}
	if !strings.Contains(message.Subject, "Verify") {
		t.Errorf("subject %q", message.Subject)
	}
}
