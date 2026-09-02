//go:build integration

// The email queue against real PostgreSQL.
//
// What is asserted is what a person depends on without seeing: that an email
// promised by a transaction vanishes with its rollback, that two senders
// cannot both deliver one message, that a sent secret is erased at rest, and
// that an undeliverable email becomes visible rather than retried forever.
package notification_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Yelethe1st/prepeet/services/platform/internal/notification"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("prepeet"),
		tcpostgres.WithUsername("prepeet_migrator"),
		tcpostgres.WithPassword("migrator-password"),
		// The occurrence matters: the image logs readiness for its temporary
		// init server too. See the database package's tests for the history.
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(2*time.Minute)),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "starting PostgreSQL: %v\n", err)
		os.Exit(1)
	}

	adminURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "connection string: %v\n", err)
		os.Exit(1)
	}
	if err := database.Migrate(ctx, adminURL, database.MigrateOptions{AppPassword: "app-password"}); err != nil {
		fmt.Fprintf(os.Stderr, "migrating: %v\n", err)
		os.Exit(1)
	}

	cfg, err := pgx.ParseConfig(adminURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parsing connection string: %v\n", err)
		os.Exit(1)
	}
	pool, err = pgxpool.New(ctx, fmt.Sprintf(
		"postgres://prepeet_app:app-password@%s:%d/%s?sslmode=disable",
		cfg.Host, cfg.Port, cfg.Database))
	if err != nil {
		fmt.Fprintf(os.Stderr, "app pool: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	pool.Close()
	if err := container.Terminate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "terminating PostgreSQL: %v\n", err)
	}
	os.Exit(code)
}

// enqueueOne commits one verification email and returns its id.
func enqueueOne(t *testing.T, queue *notification.Queue, recipient string) string {
	t.Helper()
	ctx := t.Context()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	mailID, err := queue.Enqueue(ctx, tx, recipient,
		notification.VerifyEmail{Link: "https://app.example.test/verify?token=T", ExpiresMinutes: 30})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return mailID
}

// The property the queue exists for.
func TestAnEmailVanishesWithItsTransaction(t *testing.T) {
	ctx := t.Context()
	queue := notification.NewQueue(pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	mailID, err := queue.Enqueue(ctx, tx, "rollback@example.test",
		notification.VerifyEmail{Link: "https://x.test/v?token=T", ExpiresMinutes: 30})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM notification.emails WHERE id = $1", mailID).Scan(&count); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if count != 0 {
		t.Fatal("an email survived the rollback of the transaction that promised it")
	}
}

func TestAClaimedEmailCarriesItsRenderedContent(t *testing.T) {
	queue := notification.NewQueue(pool)
	mailID := enqueueOne(t, queue, "content@example.test")

	claimed := claimAll(t, queue)
	email, ok := claimed[mailID]
	if !ok {
		t.Fatal("the enqueued email was not claimable")
	}

	if email.Recipient != "content@example.test" {
		t.Errorf("recipient = %q", email.Recipient)
	}
	if !strings.Contains(email.Body, "https://app.example.test/verify?token=T") {
		t.Error("the body does not carry the rendered link")
	}
	if !strings.Contains(email.Body, "30 minutes") {
		t.Error("the body does not state the expiry")
	}
}

func TestTwoSendersCannotClaimTheSameEmail(t *testing.T) {
	queue := notification.NewQueue(pool)
	mailID := enqueueOne(t, queue, "contended@example.test")

	// Both claim concurrently; SKIP LOCKED means the id appears exactly once
	// across the two, never twice.
	var mu sync.Mutex
	seen := map[string]int{}
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, err := queue.Claim(context.Background(), 50)
			if err != nil {
				t.Errorf("claim: %v", err)
				return
			}
			mu.Lock()
			for _, email := range claimed {
				seen[email.ID]++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()

	if seen[mailID] != 1 {
		t.Fatalf("the email was claimed %d times; one delivery per message is the whole point", seen[mailID])
	}
}

func TestSendingErasesTheSecret(t *testing.T) {
	ctx := t.Context()
	queue := notification.NewQueue(pool)
	mailID := enqueueOne(t, queue, "erased@example.test")

	if err := queue.MarkSent(ctx, mailID); err != nil {
		t.Fatalf("marking sent: %v", err)
	}

	var subject, body *string
	var sentAt *time.Time
	if err := pool.QueryRow(ctx,
		"SELECT subject, body, sent_at FROM notification.emails WHERE id = $1", mailID).
		Scan(&subject, &body, &sentAt); err != nil {
		t.Fatalf("reading the row: %v", err)
	}
	if sentAt == nil {
		t.Error("the send was not recorded")
	}
	if subject != nil || body != nil {
		t.Error("the content survived the send; a delivered secret has no reason to stay readable at rest")
	}
}

func TestAFailingEmailBacksOffAndThenDies(t *testing.T) {
	ctx := t.Context()
	queue := notification.NewQueue(pool)
	mailID := enqueueOne(t, queue, "undeliverable@example.test")

	for range notification.MaxAttempts {
		if err := queue.MarkFailed(ctx, mailID, "relay refused"); err != nil {
			t.Fatalf("marking failed: %v", err)
		}
	}

	var attempts int
	var deadAt *time.Time
	var lastError *string
	if err := pool.QueryRow(ctx,
		"SELECT attempts, dead_at, last_error FROM notification.emails WHERE id = $1", mailID).
		Scan(&attempts, &deadAt, &lastError); err != nil {
		t.Fatalf("reading the row: %v", err)
	}

	if attempts != notification.MaxAttempts {
		t.Errorf("attempts = %d, want %d", attempts, notification.MaxAttempts)
	}
	if deadAt == nil {
		t.Error("the email was not dead-lettered, so it would be retried forever")
	}
	if lastError == nil || *lastError != "relay refused" {
		t.Error("the failure reason an operator acts on was not recorded")
	}
}

// recordingTransport captures sends and fails on demand.
type recordingTransport struct {
	mu   sync.Mutex
	sent []string
	fail bool
}

func (r *recordingTransport) Send(_ context.Context, recipient, subject, body string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return fmt.Errorf("transport down")
	}
	r.sent = append(r.sent, recipient+"|"+subject+"|"+body)
	return nil
}

// The whole path: enqueue, drain, delivered, erased.
func TestTheSenderDrainsTheQueue(t *testing.T) {
	ctx := t.Context()
	queue := notification.NewQueue(pool)
	mailID := enqueueOne(t, queue, "drained@example.test")

	transport := &recordingTransport{}
	sender := notification.NewSender(queue, transport,
		slog.New(slog.NewTextHandler(os.Stderr, nil)))

	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { _ = sender.Run(runCtx); close(done) }()

	deadline := time.Now().Add(8 * time.Second)
	for {
		var sentAt *time.Time
		if err := pool.QueryRow(ctx,
			"SELECT sent_at FROM notification.emails WHERE id = $1", mailID).Scan(&sentAt); err != nil {
			t.Fatalf("reading: %v", err)
		}
		if sentAt != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the sender never delivered the email")
		}
		time.Sleep(100 * time.Millisecond)
	}
	cancel()
	<-done

	transport.mu.Lock()
	defer transport.mu.Unlock()
	found := false
	for _, sent := range transport.sent {
		if strings.HasPrefix(sent, "drained@example.test|") {
			found = true
		}
	}
	if !found {
		t.Fatal("the row says sent but the transport never saw the message")
	}
}

// claimAll claims everything pending and indexes it by id.
func claimAll(t *testing.T, queue *notification.Queue) map[string]notification.Pending {
	t.Helper()
	claimed, err := queue.Claim(t.Context(), 100)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	byID := map[string]notification.Pending{}
	for _, email := range claimed {
		byID[email.ID] = email
	}
	return byID
}

// Delivery status is read back by the id the enqueue returned, and the
// precedence is the one a recruiter's decision depends on: a bounce or a
// complaint is a fact about the address that outlives a later send, a dead
// letter is terminal for the attempt, sent beats pending, and an id with no
// row is unknown rather than guessed.
func TestDeliveryStatusReadsWhatBecameOfAnEmail(t *testing.T) {
	ctx := t.Context()
	queue := notification.NewQueue(pool)

	setColumn := func(t *testing.T, id, column string) {
		t.Helper()
		// The column name is a fixed test literal, never input, so interpolating
		// it is safe here where a parameter cannot stand in for an identifier.
		if _, err := pool.Exec(ctx,
			"UPDATE notification.emails SET "+column+" = now() WHERE id = $1", id); err != nil {
			t.Fatalf("set %s: %v", column, err)
		}
	}

	t.Run("pending before anything happens", func(t *testing.T) {
		id := enqueueOne(t, queue, "pending@example.test")
		got, err := queue.DeliveryStatus(ctx, id)
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		if got.Status != notification.DeliveryPending {
			t.Fatalf("status = %q, want pending", got.Status)
		}
	})

	t.Run("sent once handed to the transport", func(t *testing.T) {
		id := enqueueOne(t, queue, "sent@example.test")
		setColumn(t, id, "sent_at")
		got, _ := queue.DeliveryStatus(ctx, id)
		if got.Status != notification.DeliverySent {
			t.Fatalf("status = %q, want sent", got.Status)
		}
	})

	t.Run("a bounce wins over a send", func(t *testing.T) {
		id := enqueueOne(t, queue, "bounce@example.test")
		setColumn(t, id, "sent_at")
		setColumn(t, id, "bounced_at")
		got, _ := queue.DeliveryStatus(ctx, id)
		if got.Status != notification.DeliveryBounced {
			t.Fatalf("status = %q, want bounced", got.Status)
		}
	})

	t.Run("a complaint wins over a send", func(t *testing.T) {
		id := enqueueOne(t, queue, "spam@example.test")
		setColumn(t, id, "sent_at")
		setColumn(t, id, "complained_at")
		got, _ := queue.DeliveryStatus(ctx, id)
		if got.Status != notification.DeliveryComplained {
			t.Fatalf("status = %q, want complained", got.Status)
		}
	})

	t.Run("a dead letter reads failed", func(t *testing.T) {
		id := enqueueOne(t, queue, "dead@example.test")
		setColumn(t, id, "dead_at")
		got, _ := queue.DeliveryStatus(ctx, id)
		if got.Status != notification.DeliveryFailed {
			t.Fatalf("status = %q, want failed", got.Status)
		}
	})

	t.Run("an unknown id is unknown, not a guessed delivery", func(t *testing.T) {
		got, err := queue.DeliveryStatus(ctx, "00000000-0000-7000-8000-0000000000ff")
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		if got.Status != notification.DeliveryUnknown {
			t.Fatalf("status = %q, want unknown", got.Status)
		}
	})
}
