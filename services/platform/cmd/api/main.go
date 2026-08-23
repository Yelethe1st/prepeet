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

	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
	"github.com/Yelethe1st/prepeet/services/platform/platform/health"
	"github.com/Yelethe1st/prepeet/services/platform/platform/httpserver"
)

// shutdownGrace bounds how long a terminating process waits for in-flight
// requests. It is deliberately shorter than a typical orchestrator termination
// grace period so the process exits on its own terms rather than being killed.
const shutdownGrace = 20 * time.Second

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		// Configuration failure is not recoverable and must not be retried. A
		// process that starts with configuration it cannot use fails later and
		// further from its cause.
		log.Error("configuration is not usable", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Dependency checks are registered here as each adapter lands. Readiness
	// with no registered dependencies reports ready, which is correct for a
	// process that has none yet.
	checks := health.NewRegistry()

	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           httpserver.New(httpserver.Config{Health: checks}),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("api shutdown was not clean", slog.String("error", err.Error()))
		os.Exit(1)
	}
	log.Info("api stopped")
}
