// Command opsctl is the operator's surface onto the work backlog.
//
// OPS-03 needs somewhere a person can see what has failed and act on it. This
// is that place, and it is a command line rather than a screen deliberately:
// the moment it is used is an incident, the operators using it are the people
// who already have a shell open, and a console that has to be deployed before
// it can be used is a console that is unavailable exactly when it is needed.
// The web surface, when it comes, consumes the same operations context and
// inherits the same guarantees; nothing here is a decision, only a way in.
//
// Four subcommands, and no more:
//
//	opsctl backlog                        how much work is waiting and how old
//	opsctl failed [limit]                 what has failed and why
//	opsctl retry <id> <reason>            put one failed item back in the queue
//	opsctl discard <id> <reason>          decide one item must never be delivered
//
// The acting operator arrives by environment, as PREPEET_OPERATOR, because
// every action here writes an audit row bound to a real account. There is no
// flag to skip it: an action nobody can be named for is one that must not
// happen, and the console refuses it.
//
// Implements part of OPS-03.
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Yelethe1st/prepeet/services/platform/cmd/wiring"
	"github.com/Yelethe1st/prepeet/services/platform/internal/operations"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "opsctl: %v\n", err)
		os.Exit(1)
	}
}

// usage is printed for anything this tool does not understand. It is the whole
// interface, so it is short on purpose.
const usage = `usage:
  opsctl backlog
  opsctl failed [limit]
  opsctl retry <event-id> <reason>
  opsctl discard <event-id> <reason>

PREPEET_DATABASE_URL and PREPEET_OPERATOR must be set. PREPEET_OPERATOR is the
user id every action here is audited against.`

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%s", usage)
	}

	databaseURL := os.Getenv("PREPEET_DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("PREPEET_DATABASE_URL is required")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connecting: %w", err)
	}
	defer pool.Close()

	console := operations.NewConsole(pool, wiring.NewBacklog(outbox.New(pool)))

	switch args[0] {
	case "backlog":
		return showBacklog(ctx, console)
	case "failed":
		return showFailed(ctx, console, args[1:])
	case "retry", "discard":
		return act(ctx, console, args)
	default:
		return fmt.Errorf("unknown command %q\n%s", args[0], usage)
	}
}

// showBacklog prints the measurement the alert is based on.
//
// The same assessment the monitor makes rather than a second calculation, so an
// operator reading this and a pager firing cannot disagree about whether the
// system is healthy.
func showBacklog(ctx context.Context, console *operations.Console) error {
	assessment, err := console.Backlog(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("pending        %d\n", assessment.Depth.Pending)
	fmt.Printf("failed         %d\n", assessment.Depth.Failed)
	fmt.Printf("oldest pending %s\n", assessment.Depth.OldestPending.Round(time.Second))
	fmt.Printf("budget         %s\n", operations.PendingAgeBudget)
	fmt.Printf("state          %s\n", assessment.Summary())
	return nil
}

// showFailed prints what is waiting for a decision.
func showFailed(ctx context.Context, console *operations.Console, args []string) error {
	limit := 0
	if len(args) > 0 {
		parsed, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("limit %q is not a number", args[0])
		}
		limit = parsed
	}

	items, err := console.Failed(ctx, limit)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Println("nothing has failed")
		return nil
	}

	for _, item := range items {
		fmt.Printf("%s  %s  attempts=%d  failed=%s\n    %s\n",
			item.ID, item.Kind, item.Attempts,
			item.FailedAt.UTC().Format(time.RFC3339), item.LastError)
	}
	return nil
}

// act performs a retry or a discard.
//
// The reason is a positional argument rather than a flag, so it cannot be
// forgotten and then supplied as an empty string by a script. The console
// refuses a blank one anyway; this makes the refusal unlikely rather than
// merely survivable.
func act(ctx context.Context, console *operations.Console, args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("%s needs an event id and a reason\n%s", args[0], usage)
	}

	operator := os.Getenv("PREPEET_OPERATOR")
	if operator == "" {
		return fmt.Errorf("PREPEET_OPERATOR is required: every action here is audited against a real account")
	}

	itemID := args[1]
	reason := strings.Join(args[2:], " ")
	// A request identifier so this action can be found in the traces beside
	// everything else that happened at that moment. Generated here because
	// nothing upstream of a shell has one.
	acting := operations.Operator{UserID: operator, RequestID: id.New().String()}

	switch args[0] {
	case "retry":
		if err := console.Retry(ctx, acting, itemID, reason); err != nil {
			return err
		}
		fmt.Printf("%s is back in the queue; the dispatcher will deliver it\n", itemID)
	case "discard":
		if err := console.Discard(ctx, acting, itemID, reason); err != nil {
			return err
		}
		fmt.Printf("%s will never be delivered; the row and the reason remain\n", itemID)
	}
	return nil
}
