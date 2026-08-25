// Command contentctl publishes git-authored artifacts into the registry.
//
// ADR-0011's publishing tool: it reads services/intelligence/artifacts -
// content that was reviewed where it was authored, in git - and walks each
// file through the registry's own lifecycle to published. It is not a
// runtime dependency; the api and worker never read a file, only the
// registry rows this tool produced. Run it on deploy and whenever content
// changes: it is idempotent, refuses an edited file wearing an already
// published version number, and prints what it did.
//
// The acting principals arrive by environment - PREPEET_CONTENT_AUTHOR and
// PREPEET_CONTENT_PUBLISHER, two existing accounts - because the audit
// trail's foreign keys insist an actor exists, and they should.
//
// Implements part of CAT-03.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Yelethe1st/prepeet/services/platform/internal/catalog"
	"github.com/Yelethe1st/prepeet/services/platform/internal/content"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "contentctl: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	databaseURL := os.Getenv("PREPEET_DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("PREPEET_DATABASE_URL is required")
	}
	author := os.Getenv("PREPEET_CONTENT_AUTHOR")
	publisher := os.Getenv("PREPEET_CONTENT_PUBLISHER")
	if author == "" || publisher == "" || author == publisher {
		return fmt.Errorf("PREPEET_CONTENT_AUTHOR and PREPEET_CONTENT_PUBLISHER must name two distinct accounts")
	}
	dir := "services/intelligence/artifacts"
	if len(os.Args) > 2 && os.Args[1] == "-dir" {
		dir = os.Args[2]
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connecting: %w", err)
	}
	defer pool.Close()

	// Each artifact type's validation is the reading context's own parse,
	// injected here because content must not import the contexts that read
	// its artifacts. A type with no validator publishes unchecked, which is
	// the plan type's current state and a known floor.
	validators := map[string]content.Validator{
		"catalogue": func(body json.RawMessage) error {
			_, err := catalog.Parse(body)
			return err
		},
	}

	loader := content.NewLoader(content.NewStore(pool), validators, author, publisher)
	outcomes, err := loader.LoadDirectory(ctx, os.DirFS(dir))
	for _, outcome := range outcomes {
		fmt.Printf("%-10s %s@%s\n", outcome.Action, outcome.Reference, outcome.Version)
	}
	return err
}
