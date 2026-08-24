// Command migrate applies database migrations.
//
// Migrations run as their own process rather than on api startup, so that
// several api replicas starting at once cannot race each other, and so a
// migration can be run and verified before the code that depends on it is
// deployed.
//
// Every tenant-scoped table is created together with its row-level security
// policy in the same migration. See ADR-0002 and
// services/platform/platform/database/README.md.
//
// Implements part of PLT-03.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/telemetry"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		log.Error("configuration is not usable", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// A migration runs on the deploy path, where a collector may not be
	// reachable, so this process logs rather than traces. The scrubbing logger
	// matters here more than anywhere: a driver error on a failed connection
	// carries the connection string, password included.
	log = telemetry.NewLogger(telemetry.Config{
		ServiceName: "prepeet-migrate",
		Environment: string(cfg.Environment),
	}, os.Stdout)

	if cfg.DatabaseURL == "" {
		log.Error("PREPEET_DATABASE_URL is required to migrate")
		os.Exit(1)
	}

	// A migration that is interrupted rolls back rather than leaving a
	// half-applied schema, so cancelling is safe and is worth wiring up: a
	// deploy that is being rolled back should not have to wait for DDL it no
	// longer wants.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	migrations, err := database.Migrations()
	if err != nil {
		log.Error("reading migrations", slog.String("error", err.Error()))
		os.Exit(1)
	}
	log.Info("applying migrations",
		slog.String("environment", string(cfg.Environment)),
		slog.Int("available", len(migrations)))

	if err := database.Migrate(ctx, cfg.DatabaseURL, database.MigrateOptions{
		AppPassword: cfg.AppDatabasePassword,
	}); err != nil {
		// The error names the migration that failed and, for a checksum
		// mismatch, says that migrations are forward only. That is the whole
		// message an operator needs at three in the morning.
		log.Error("migration failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	log.Info("migrations applied", slog.Int("total", len(migrations)))
}
