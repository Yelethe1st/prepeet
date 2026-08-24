package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/Yelethe1st/prepeet/services/platform/platform/ratelimit/db"
)

// Counter counts requests against a key and decides whether one may proceed.
//
// Two implementations. Memory is correct while one instance runs and wrong the
// moment two do, so it is for tests and for local development. Postgres is what
// a deployment uses, because ADR-0001 runs more than one task and a counter
// each task keeps to itself is a limit multiplied by the task count.
type Counter interface {
	Allow(ctx context.Context, key string) (Decision, error)
}

// Postgres counts in the database, so every instance shares one count.
//
// The same store as the credentials it protects, deliberately. A separate store
// can be down while the database is up, and then somebody has to choose between
// locking every user out and letting every attacker through. Here there is no
// such state: if this database is unreachable then authentication cannot happen
// at all, so failing open costs nothing that is not already lost.
//
// The SQL lives in db/queries.sql and the access code beside it is generated,
// per ADR-0008.
type Postgres struct {
	pool *pgxpool.Pool
	q    *db.Queries
	rule Rule
	now  func() time.Time
}

// NewPostgres builds a database-backed counter.
//
// It panics on an unusable rule, for the same reason Memory does: a limit of
// zero refuses every request and a window of zero counts over no time, and both
// are configuration mistakes that lock out every user.
func NewPostgres(pool *pgxpool.Pool, rule Rule, now func() time.Time) *Postgres {
	if rule.Limit <= 0 {
		panic(fmt.Sprintf("ratelimit: limit is %d, which would refuse every request", rule.Limit))
	}
	if rule.Window <= 0 {
		panic(fmt.Sprintf("ratelimit: window is %s, which counts over no time at all", rule.Window))
	}
	return &Postgres{pool: pool, q: db.New(pool), rule: rule, now: now}
}

// Allow counts one request against key and reports whether it may proceed.
//
// One statement, not a read followed by a write. `INSERT ... ON CONFLICT DO
// UPDATE ... RETURNING` increments and reads the new value atomically, so two
// requests landing at the same instant cannot both see the count before either
// writes it. A read-then-write would make the limit a suggestion under exactly
// the load that matters.
//
// A database failure returns Allowed with the error attached. The caller
// proceeds and alerts: refusing instead would turn a database blip into a total
// lockout, and would protect nothing, since the credential check needs the same
// database.
func (p *Postgres) Allow(ctx context.Context, key string) (Decision, error) {
	if key == "" {
		// Counting an empty key would collapse every caller without one into a
		// single bucket, so the first of them would exhaust the limit for all
		// the rest.
		return Decision{Allowed: false, RetryAfter: p.rule.Window}, nil
	}

	now := p.now()
	windowStart := now.Truncate(p.rule.Window)
	windowEnd := windowStart.Add(p.rule.Window)

	counted, err := p.q.Increment(ctx, db.IncrementParams{Key: key, WindowStart: windowStart})
	if err != nil {
		return Decision{Allowed: true}, fmt.Errorf("ratelimit: counting %q: %w", key, err)
	}
	count := int(counted)

	if count > p.rule.Limit {
		return Decision{
			Allowed:    false,
			RetryAfter: roundUpToSecond(windowEnd.Sub(now)),
			Remaining:  0,
		}, nil
	}

	return Decision{Allowed: true, Remaining: p.rule.Limit - count}, nil
}

// Sweep deletes windows that have closed and returns how many rows went.
//
// Rows nobody will read again are only rows somebody has to store, and the keys
// here are email and network addresses, which is personal data under
// docs/security/data-classification.md. One window of slack is kept so a
// request arriving at a boundary does not find its counter deleted underneath
// it.
//
// This is the "run once per interval across every instance" shape, so it wants
// a scheduler rather than a goroutine per task. Running it more than once is
// harmless, which is what makes it safe to schedule crudely until Temporal
// arrives.
func (p *Postgres) Sweep(ctx context.Context) (int64, error) {
	cutoff := p.now().Add(-2 * p.rule.Window)

	removed, err := p.q.Sweep(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("ratelimit: sweeping counters: %w", err)
	}
	return removed, nil
}

// Both implementations satisfy Counter. Checked here so a signature change
// breaks the build rather than a wiring line.
var (
	_ Counter = (*Postgres)(nil)
	_ Counter = (*Memory)(nil)
)

// ErrUnavailable is returned wrapped when counting could not happen. The
// decision that accompanies it always allows, and the caller is expected to
// alert rather than to refuse.
var ErrUnavailable = errors.New("ratelimit: counter unavailable")
