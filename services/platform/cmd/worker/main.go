// Command worker runs the Temporal workflow and activity workers.
//
// Composition, evaluation, deletion and outbox delivery all run here rather
// than inside a request, because each has to survive a process restart without
// duplicating its effect. See docs/architecture/session-lifecycle.md.
//
// The worker is a separate binary from the api so that a backlog of evaluation
// work cannot starve the request path, and so the two scale independently.
//
// Wiring lands with PLT-06. This entry point exists now so the deployable is
// real from the first commit rather than appearing later.
package main

import (
	"log/slog"
	"os"

	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
	"github.com/Yelethe1st/prepeet/services/platform/platform/telemetry"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		log.Error("configuration is not usable", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Traces are not set up here yet: this process has no work to trace until
	// PLT-06 registers workflows. The scrubbing logger is wired now regardless,
	// because startup and configuration errors carry connection strings.
	log = telemetry.NewLogger(telemetry.Config{
		ServiceName: "prepeet-worker",
		Environment: string(cfg.Environment),
	}, os.Stdout)

	log.Info("worker started with no registered workflows",
		slog.String("environment", string(cfg.Environment)),
		slog.String("next", "PLT-06 registers the Temporal client and workers"))
}
