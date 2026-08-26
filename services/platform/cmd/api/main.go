// Command api serves the public HTTP API to the browser.
//
// It is the only process the Next.js application talks to. The browser never
// reaches PostgreSQL, Temporal, object storage or the Python intelligence
// service directly, which is what keeps authorization decidable in one place.
//
// Implements part of PLT-01.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/internal/candidate"
	"github.com/Yelethe1st/prepeet/services/platform/internal/catalog"
	"github.com/Yelethe1st/prepeet/services/platform/internal/content"
	"github.com/Yelethe1st/prepeet/services/platform/internal/identity"
	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/internal/notification"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
	"github.com/Yelethe1st/prepeet/services/platform/platform/health"
	"github.com/Yelethe1st/prepeet/services/platform/platform/objectstore"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
	"github.com/Yelethe1st/prepeet/services/platform/platform/ratelimit"
	"github.com/Yelethe1st/prepeet/services/platform/platform/telemetry"
)

// shutdownGrace bounds how long a terminating process waits for in-flight
// requests. It is deliberately shorter than a typical orchestrator termination
// grace period so the process exits on its own terms rather than being killed.
const shutdownGrace = 20 * time.Second

func main() {
	// A plain logger covers the window before configuration is read, which is
	// the window a configuration failure lands in.
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		// Configuration failure is not recoverable and must not be retried. A
		// process that starts with configuration it cannot use fails later and
		// further from its cause.
		log.Error("configuration is not usable", slog.String("error", err.Error()))
		os.Exit(1)
	}

	telemetryConfig := telemetry.Config{
		ServiceName: "prepeet-api",
		Environment: string(cfg.Environment),
		Endpoint:    cfg.OTLPEndpoint,
		SampleRatio: cfg.TraceSampleRatio,
	}

	// From here on every log line is scrubbed and carries its trace.
	log = telemetry.NewLogger(telemetryConfig, os.Stdout)

	shutdownTelemetry, err := telemetry.Setup(context.Background(), telemetryConfig)
	if err != nil {
		// Telemetry that cannot start is a startup failure rather than a
		// degradation. A process running unobserved looks identical to a
		// healthy one until something goes wrong in it.
		log.Error("telemetry is not usable", slog.String("error", err.Error()))
		os.Exit(1)
	}

	if cfg.DatabaseURL == "" {
		log.Error("PREPEET_DATABASE_URL is required: every request this process serves reads or writes it")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("the database is not reachable", slog.String("error", telemetry.Scrub(err.Error())))
		os.Exit(1)
	}
	defer pool.Close()

	checks := health.NewRegistry()
	// Readiness reports whether this process can serve traffic, so it checks the
	// dependency every request needs. The check returns a bare error rather than
	// a description: readiness output is public, and a message naming what failed
	// is an invitation to map the deployment.
	checks.Register("database", func(ctx context.Context) error {
		if err := pool.Ping(ctx); err != nil {
			return errors.New("unavailable")
		}
		return nil
	})

	// The document flows need object storage; without a bucket the API
	// refuses to start, because a candidate offered an upload button wired to
	// nothing would lose their file politely.
	if cfg.S3Bucket == "" {
		log.Error("PREPEET_S3_BUCKET is required: the document surface has nowhere to store")
		os.Exit(1)
	}
	uploads, err := objectstore.NewS3Store(ctx, objectstore.S3Config{
		Endpoint: cfg.S3Endpoint, Region: cfg.S3Region, Bucket: cfg.S3Bucket,
		AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey,
		UsePathStyle: cfg.S3UsePathStyle,
	})
	if err != nil {
		log.Error("object storage is not usable", slog.String("error", err.Error()))
		os.Exit(1)
	}
	candidateStore := candidate.NewStore(pool)
	candidates := candidateAdapter{
		service:   candidate.NewService(candidateStore),
		documents: candidate.NewDocuments(candidateStore, uploads, outbox.New(pool)),
	}

	handler, err := api.NewServer(api.ServerConfig{
		Identity: identityAdapter{service: identity.NewService(identity.NewRepository(pool), time.Now).
			WithTokenFlows(identity.TokenFlows{
				Mailer: queueMailer{queue: notification.NewQueue(pool)},
				// One email per address per flow per minute. The Postgres
				// counter, so every task shares the count and the cooldown a
				// person sees is the cooldown that holds.
				Resend:  ratelimit.NewPostgres(pool, ratelimit.Rule{Limit: 1, Window: time.Minute}, time.Now),
				BaseURL: cfg.WebBaseURL,
			})},
		Candidates: candidates,
		Documents:  candidates,
		Catalog:    newCatalogAdapter(content.NewStore(pool)),
		Interviews: interviewAdapter{
			catalogue: catalog.NewService(registrySource{registry: content.NewStore(pool)}),
			sessions:  interview.NewStore(pool),
			registry:  content.NewStore(pool),
		},
		Environment: cfg.Environment,
		Health:      checks,
	})
	if err != nil {
		// A wiring mistake is a startup failure rather than a nil dereference on
		// whichever request first needs the missing piece.
		log.Error("the API could not be built", slog.String("error", err.Error()))
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Info("api listening",
			slog.String("address", cfg.Address),
			slog.String("environment", string(cfg.Environment)))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("api stopped serving", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("api shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()

	serverErr := server.Shutdown(shutdownCtx)

	// Telemetry is flushed after the server stops and before the process exits,
	// because the spans buffered at this point describe whatever caused the
	// shutdown. Losing them loses the record of the incident.
	if err := shutdownTelemetry(shutdownCtx); err != nil {
		log.Error("telemetry did not flush", slog.String("error", err.Error()))
	}

	if serverErr != nil {
		log.Error("api shutdown was not clean", slog.String("error", serverErr.Error()))
		os.Exit(1)
	}
	log.Info("api stopped")
}
