// Command worker runs background delivery and, from PLT-06, the Temporal
// workflow and activity workers.
//
// Composition, evaluation, deletion and outbox delivery all run here rather
// than inside a request, because each has to survive a process restart without
// duplicating its effect. See docs/architecture/session-lifecycle.md.
//
// The worker is a separate binary from the api so that a backlog of evaluation
// work cannot starve the request path, and so the two scale independently.
//
// Implements part of PLT-01 and the delivery half of INT-02.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Yelethe1st/prepeet/services/platform/platform/broadcast"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
	"github.com/Yelethe1st/prepeet/services/platform/platform/telemetry"
)

// shutdownGrace bounds how long the process waits for in-flight delivery.
// Deliberately shorter than a typical orchestrator termination grace period, so
// the process exits on its own terms rather than being killed mid-delivery.
const shutdownGrace = 20 * time.Second

func main() {
	// A plain logger covers the window before configuration is read, which is
	// the window a configuration failure lands in.
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		log.Error("configuration is not usable", slog.String("error", err.Error()))
		os.Exit(1)
	}

	telemetryConfig := telemetry.Config{
		ServiceName: "prepeet-worker",
		Environment: string(cfg.Environment),
		Endpoint:    cfg.OTLPEndpoint,
		SampleRatio: cfg.TraceSampleRatio,
	}
	log = telemetry.NewLogger(telemetryConfig, os.Stdout)

	if cfg.DatabaseURL == "" {
		log.Error("PREPEET_DATABASE_URL is required: the worker's only job so far is to drain the outbox")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := telemetry.Setup(ctx, telemetryConfig)
	if err != nil {
		log.Error("telemetry is not usable", slog.String("error", err.Error()))
		os.Exit(1)
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("the database is not reachable", slog.String("error", telemetry.Scrub(err.Error())))
		os.Exit(1)
	}
	defer pool.Close()

	// The listener is established at startup rather than lazily, so a
	// permission or connectivity problem is a startup failure instead of a
	// silence somebody notices days later.
	wakeups, err := broadcast.NewPostgres(ctx, pool, log)
	if err != nil {
		// Not fatal. Without wakeups the dispatcher polls, which delivers
		// everything and only costs latency. Refusing to start would trade a
		// working dispatcher for a fast one.
		log.Warn("wakeups are unavailable; the dispatcher will poll only",
			slog.String("error", telemetry.Scrub(err.Error())))
		wakeups = nil
	} else {
		defer func() { _ = wakeups.Close() }()
	}

	router := routes()
	for eventType, disposition := range router.Routes() {
		log.Info("outbox route registered",
			slog.String("event_type", eventType),
			slog.String("disposition", disposition))
	}

	dispatcher := outbox.NewDispatcher(outbox.New(pool), router, wakeupsOrNil(wakeups),
		outbox.DispatcherOptions{Logger: log})

	log.Info("worker started",
		slog.String("environment", string(cfg.Environment)),
		slog.Int("outbox_routes", len(router.Routes())),
		slog.String("next", "PLT-06 registers the Temporal client and workers"))

	// Run returns only when the context is cancelled, so this is the process
	// lifetime.
	if err := dispatcher.Run(ctx); err != nil {
		log.Error("the outbox dispatcher stopped", slog.String("error", telemetry.Scrub(err.Error())))
	}

	log.Info("worker shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := shutdownTelemetry(shutdownCtx); err != nil {
		log.Error("telemetry did not flush", slog.String("error", err.Error()))
	}
	log.Info("worker stopped")
}

// routes is the composition root for event delivery.
//
// It is empty, and that is the correct state: nothing publishes an event yet.
// The router fails on an unregistered type rather than treating it as nothing
// to do, so the first person to add a producer cannot forget the consumer. The
// event dead letters and somebody sees it.
//
// Handlers are registered here rather than by the packages that own them,
// because a bounded context must not know that another one consumes its events.
// See ADR-0005.
func routes() *outbox.Router {
	router := outbox.NewRouter()

	// Registrations land with the tickets that produce the events:
	//   INT-03 webhook delivery, PRC-02 processing, SCR-04 screening.

	return router
}

// wakeupsOrNil converts a typed nil into an untyped one.
//
// A *broadcast.Postgres that is nil stored in a broadcast.Broadcaster interface
// is not equal to nil, so the dispatcher's nil check would pass and it would
// then call a method on nothing. This is Go's most reliably rediscovered
// footgun, and the conversion is here rather than inline so it is visible.
func wakeupsOrNil(bus *broadcast.Postgres) broadcast.Broadcaster {
	if bus == nil {
		return nil
	}
	return bus
}
