// Command migrate applies database migrations.
//
// Migrations run as their own process rather than on api startup, so that
// several api replicas starting at once cannot race each other, and so a
// migration can be run and verified before the code that depends on it is
// deployed.
//
// Every tenant-scoped table is created together with its row-level security
// policy in the same migration. See services/platform/migrations/README.md.
//
// Wiring lands with PLT-03. This entry point exists now so the deployable is
// real from the first commit rather than appearing later.
package main

import (
	"log/slog"
	"os"

	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		log.Error("configuration is not usable", slog.String("error", err.Error()))
		os.Exit(1)
	}

	log.Info("migrate found no migrations to apply",
		slog.String("environment", string(cfg.Environment)),
		slog.String("next", "PLT-03 adds the schema, RLS policies and the migration runner"))
}
