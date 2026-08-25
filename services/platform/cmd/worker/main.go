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
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	sdkworker "go.temporal.io/sdk/worker"

	"github.com/Yelethe1st/prepeet/services/platform/internal/candidate"
	"github.com/Yelethe1st/prepeet/services/platform/internal/content"
	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/internal/notification"
	"github.com/Yelethe1st/prepeet/services/platform/platform/broadcast"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
	"github.com/Yelethe1st/prepeet/services/platform/platform/email"
	"github.com/Yelethe1st/prepeet/services/platform/platform/objectstore"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
	"github.com/Yelethe1st/prepeet/services/platform/platform/telemetry"
	"github.com/Yelethe1st/prepeet/services/platform/platform/temporal"
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

	// extraction is the document_uploaded route, set only when the candidate
	// task queue is actually being served; nil leaves the type unregistered
	// so those events dead letter visibly instead of vanishing.
	var extraction outbox.HandlerFunc

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

	// Temporal is dialled before the dispatcher starts, so a process that
	// cannot reach it fails at startup rather than when the first workflow is
	// requested. It is not fatal yet, because no workflow exists to run and a
	// worker that refused to start would also stop draining the outbox, which
	// does work.
	workflows, err := temporal.Dial(ctx, temporal.Config{
		Address:   cfg.TemporalAddress,
		Namespace: cfg.TemporalNamespace,
		CertFile:  cfg.TemporalTLSCertFile,
		KeyFile:   cfg.TemporalTLSKeyFile,
		Logger:    log,
	})
	switch {
	case errors.Is(err, temporal.ErrNotConfigured):
		log.Info("no Temporal address is configured; this process will only drain the outbox")
	case err != nil:
		log.Error("Temporal is not reachable", slog.String("error", err.Error()))
	default:
		defer workflows.Close()
		log.Info("connected to Temporal",
			slog.String("address", cfg.TemporalAddress),
			slog.String("namespace", cfg.TemporalNamespace))

		// The interview task queue: composition, and every session workflow
		// after it. Registered only when the intelligence plane is reachable
		// too, because a worker polling the queue without a composer would
		// take tasks it can only fail.
		if cfg.IntelligenceAddress == "" {
			log.Warn("no intelligence address is configured; the interview task queue is not being served")
		} else {
			composer, conn, err := newComposer(cfg.IntelligenceAddress, content.NewStore(pool))
			if err != nil {
				log.Error("the intelligence plane is not usable", slog.String("error", err.Error()))
				os.Exit(1)
			}
			defer conn.Close()

			interviewWorker := sdkworker.New(workflows, interview.TaskQueue, sdkworker.Options{})
			interviewWorker.RegisterWorkflow(interview.CompositionWorkflow)
			activities := interview.NewActivities(interview.NewStore(pool), composer)
			interviewWorker.RegisterActivity(activities.Compose)
			interviewWorker.RegisterActivity(activities.MarkReady)
			interviewWorker.RegisterActivity(activities.MarkFailed)
			if err := interviewWorker.Start(); err != nil {
				log.Error("the interview worker did not start", slog.String("error", err.Error()))
				os.Exit(1)
			}
			defer interviewWorker.Stop()
			log.Info("interview worker started",
				slog.String("task_queue", interview.TaskQueue),
				slog.String("intelligence", cfg.IntelligenceAddress))

			// The candidate task queue: CV extraction. It additionally needs
			// object storage, because the adapter presigns the fetch URL the
			// capability reads the document through. Without a bucket the
			// queue is not served and document_uploaded events dead letter,
			// which is the visible version of "extraction is off".
			if cfg.S3Bucket == "" {
				log.Warn("no S3 bucket is configured; the candidate task queue is not being served")
			} else {
				documents, err := objectstore.NewS3Store(ctx, objectstore.S3Config{
					Endpoint: cfg.S3Endpoint, Region: cfg.S3Region, Bucket: cfg.S3Bucket,
					AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey,
					UsePathStyle: cfg.S3UsePathStyle,
				})
				if err != nil {
					log.Error("object storage is not usable", slog.String("error", err.Error()))
					os.Exit(1)
				}

				candidateWorker := sdkworker.New(workflows, candidate.ExtractionTaskQueue, sdkworker.Options{})
				candidateWorker.RegisterWorkflow(candidate.ExtractionWorkflow)
				extractionActivities := candidate.NewExtractionActivities(
					candidate.NewStore(pool), newExtractor(conn, documents))
				candidateWorker.RegisterActivity(extractionActivities.Extract)
				candidateWorker.RegisterActivity(extractionActivities.StoreFacts)
				candidateWorker.RegisterActivity(extractionActivities.MarkExtractionOutcome)
				if err := candidateWorker.Start(); err != nil {
					log.Error("the candidate worker did not start", slog.String("error", err.Error()))
					os.Exit(1)
				}
				defer candidateWorker.Stop()
				extraction = startExtraction(workflows)
				log.Info("candidate worker started",
					slog.String("task_queue", candidate.ExtractionTaskQueue))
			}
		}
	}

	router := routes(extraction)
	for eventType, disposition := range router.Routes() {
		log.Info("outbox route registered",
			slog.String("event_type", eventType),
			slog.String("disposition", disposition))
	}

	dispatcher := outbox.NewDispatcher(outbox.New(pool), router, wakeupsOrNil(wakeups),
		outbox.DispatcherOptions{Logger: log})

	log.Info("worker started",
		slog.String("environment", string(cfg.Environment)),
		slog.Int("outbox_routes", len(router.Routes())))

	// The email sender drains notification.emails beside the dispatcher. Not
	// starting is loud rather than fatal: a worker that refused to run would
	// also stop draining the outbox, but a worker silently without mail is
	// verification emails silently not arriving, so the warning names exactly
	// what is off.
	var group sync.WaitGroup
	if cfg.SMTPAddress == "" {
		log.Warn("no SMTP address is configured; enqueued email will wait in notification.emails unsent")
	} else {
		transport, err := email.New(email.Config{
			Address:  cfg.SMTPAddress,
			From:     cfg.EmailFrom,
			Username: cfg.SMTPUsername,
			Password: cfg.SMTPPassword,
		})
		if err != nil {
			log.Error("the email transport is not usable", slog.String("error", err.Error()))
			os.Exit(1)
		}
		sender := notification.NewSender(notification.NewQueue(pool), transport, log)
		group.Add(1)
		go func() {
			defer group.Done()
			_ = sender.Run(ctx)
		}()
		log.Info("email sender started", slog.String("smtp", cfg.SMTPAddress))
	}

	// Run returns only when the context is cancelled, so this is the process
	// lifetime.
	if err := dispatcher.Run(ctx); err != nil {
		log.Error("the outbox dispatcher stopped", slog.String("error", telemetry.Scrub(err.Error())))
	}
	group.Wait()

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
// The router fails on an unregistered type rather than treating it as nothing
// to do, so the first person to add a producer cannot forget the consumer. The
// event dead letters and somebody sees it.
//
// Handlers are registered here rather than by the packages that own them,
// because a bounded context must not know that another one consumes its events.
// See ADR-0005.
func routes(extraction outbox.HandlerFunc) *outbox.Router {
	router := outbox.NewRouter()

	// PRO-03: an uploaded document starts its extraction workflow. Registered
	// only when the candidate task queue is being served; otherwise the events
	// dead letter, which is the visible form of "extraction is off here".
	if extraction != nil {
		router.Handle("candidate.document_uploaded.v1", extraction)
	}

	// Further registrations land with the tickets that produce the events:
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
