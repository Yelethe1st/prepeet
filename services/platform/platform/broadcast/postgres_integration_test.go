//go:build integration

// Broadcaster tests against real PostgreSQL.
//
// The shared contract in broadcaster_suite_test.go covers the behaviour every
// broadcaster must have. This file adds only what cannot be tested any other
// way: two broadcasters standing in for two ECS tasks, which is the entire
// reason this implementation exists, and recovery from a dropped listener,
// which is the failure a long-lived listening connection actually has.
//
// The in-memory broadcaster passes the whole shared contract and fails the
// first of these, which is the argument for not deploying it.
package broadcast_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Yelethe1st/prepeet/services/platform/platform/broadcast"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("prepeet"),
		tcpostgres.WithUsername("prepeet_migrator"),
		tcpostgres.WithPassword("migrator-password"),
		// Not ForListeningPort. PostgreSQL accepts TCP connections before it
		// will answer them, so that strategy returns while the server is still
		// replying "the database system is starting up" and the first
		// connection fails. It made this suite flaky rather than broken, which
		// is worse: a failure that looks like the code under test.
		//
		// The occurrence matters as much as the log line. The official image
		// starts a temporary server to run its initialisation scripts and logs
		// readiness for that one too, so waiting for the first occurrence waits
		// for a server that is about to be shut down.
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

// quiet returns a logger that keeps expected warnings out of the test output.
// Reconnection is provoked deliberately below, and its warning is the point
// rather than a problem.
func quiet() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func newBus(t *testing.T) *broadcast.Postgres {
	t.Helper()

	bus, err := broadcast.NewPostgres(context.Background(), pool, quiet())
	if err != nil {
		t.Fatalf("NewPostgres: %v", err)
	}
	t.Cleanup(func() { _ = bus.Close() })
	return bus
}

// Postgres satisfies the same contract as every other broadcaster.
func TestPostgresSatisfiesTheBroadcasterContract(t *testing.T) {
	runBroadcasterContract(t, func(t *testing.T) broadcast.Broadcaster {
		return newBus(t)
	})
}

// What follows is specific to fanning out between processes.

// The reason this implementation exists. Two broadcasters standing in for two
// ECS tasks must see each other's messages, or a candidate's progress reaches
// only the browsers that happened to land on the task producing it.
func TestTwoInstancesSeeEachOthersMessages(t *testing.T) {
	taskOne, taskTwo := newBus(t), newBus(t)

	const topic = "session_two_instances"

	subscription, err := taskTwo.Subscribe(context.Background(), topic)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = subscription.Close() }()

	if err := taskOne.Publish(context.Background(), topic, []byte("evt_01a03")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case message := <-subscription.Messages():
		if string(message.Payload) != "evt_01a03" {
			t.Errorf("payload = %q, want evt_01a03", message.Payload)
		}
	case <-time.After(arrival):
		t.Fatal("a message published by one instance never reached the other, " +
			"which is the only thing this implementation is for")
	}
}

// Every instance receives every message and filters locally, which is the
// design decision in channelName. This asserts the filtering rather than
// assuming it: an instance with no subscriber for a topic must not deliver it
// to a subscriber of a different one.
func TestAnInstanceFiltersTopicsItDoesNotSubscribeTo(t *testing.T) {
	publisher, receiver := newBus(t), newBus(t)

	mine, err := receiver.Subscribe(context.Background(), "session_mine")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = mine.Close() }()

	if err := publisher.Publish(context.Background(), "session_theirs", []byte("not for me")); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := publisher.Publish(context.Background(), "session_mine", []byte("for me")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// The second message arriving proves the first had time to arrive too, so
	// this is an ordering argument rather than a sleep.
	select {
	case message := <-mine.Messages():
		if string(message.Payload) != "for me" {
			t.Errorf("received %q first, so a message for another topic was delivered", message.Payload)
		}
	case <-time.After(arrival):
		t.Fatal("no message arrived")
	}
}

// A binary payload must survive the round trip. A NOTIFY payload is text, so
// this is what the base64 encoding in envelope is protecting: without it, this
// would fail at the database and only for some inputs.
func TestABinaryPayloadSurvivesTheRoundTrip(t *testing.T) {
	bus := newBus(t)
	const topic = "session_binary"

	subscription, err := bus.Subscribe(context.Background(), topic)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = subscription.Close() }()

	sent := []byte{0x00, 0x01, 0xff, 0xfe, '\n', '\'', '"', 0x7f}
	if err := bus.Publish(context.Background(), topic, sent); err != nil {
		t.Fatalf("Publish of a binary payload returned %v", err)
	}

	select {
	case message := <-subscription.Messages():
		if string(message.Payload) != string(sent) {
			t.Errorf("payload = %#v, want %#v", message.Payload, sent)
		}
	case <-time.After(arrival):
		t.Fatal("a binary payload never arrived")
	}
}

// The listening connection is long lived, so it will be dropped: a failover, a
// connection reaper, an idle timeout. A broadcaster that stopped listening
// after one of those would leave every browser watching a live interview
// silently stuck, which looks like a frozen page rather than an error.
func TestTheListenerRecoversFromADroppedConnection(t *testing.T) {
	bus := newBus(t)
	const topic = "session_dropped"

	subscription, err := bus.Subscribe(context.Background(), topic)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = subscription.Close() }()

	// Prove it works before breaking it, so a failure after the drop cannot be
	// blamed on it never having worked.
	if err := bus.Publish(context.Background(), topic, []byte("before")); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	select {
	case <-subscription.Messages():
	case <-time.After(arrival):
		t.Fatal("the listener was not working before the connection was dropped")
	}

	// Terminate every backend that is listening. This is what a failover looks
	// like from the client's side.
	//
	// The count is checked because a query matching nothing would make this
	// whole test vacuous: it would pass by never having dropped anything.
	var terminated int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM (
			SELECT pg_terminate_backend(pid)
			FROM pg_stat_activity
			WHERE datname = current_database()
			  AND pid <> pg_backend_pid()
			  AND query LIKE 'LISTEN%'
		) AS killed`).Scan(&terminated); err != nil {
		t.Fatalf("terminating the listener backend: %v", err)
	}
	if terminated == 0 {
		t.Fatal("no listening backend was terminated, so this test proves nothing about recovery")
	}

	// Recovery waits for the backoff plus room for the reconnect itself.
	deadline := time.Now().Add(30 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("the listener never recovered from a dropped connection, so live progress " +
				"stops permanently after any failover")
		}

		if err := bus.Publish(context.Background(), topic, []byte("after")); err != nil {
			t.Fatalf("Publish: %v", err)
		}
		select {
		case message := <-subscription.Messages():
			if string(message.Payload) == "after" {
				return
			}
		case <-time.After(time.Second):
		}
	}
}

// Close must actually stop the listener, or a process that shuts a broadcaster
// down keeps a connection checked out of the pool for the life of the process.
func TestCloseReleasesTheListenerConnection(t *testing.T) {
	before := pool.Stat().AcquiredConns()

	bus, err := broadcast.NewPostgres(context.Background(), pool, quiet())
	if err != nil {
		t.Fatalf("NewPostgres: %v", err)
	}

	if during := pool.Stat().AcquiredConns(); during != before+1 {
		t.Errorf("acquired connections = %d, want %d: the listener holds exactly one", during, before+1)
	}

	if err := bus.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := bus.Close(); err != nil {
		t.Errorf("the second Close returned %v; shutdown paths close things twice", err)
	}

	// Release happens as the listener goroutine unwinds, so this allows for the
	// handoff rather than reading the pool immediately.
	deadline := time.Now().Add(5 * time.Second)
	for pool.Stat().AcquiredConns() > before && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if after := pool.Stat().AcquiredConns(); after != before {
		t.Errorf("acquired connections = %d after Close, want %d: the listener connection leaked", after, before)
	}
}
