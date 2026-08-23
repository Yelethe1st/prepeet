package health_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/platform/health"
)

// A registry with no checks is ready. Readiness is a claim about dependencies,
// and a service with no dependencies has nothing to be unready about.
func TestEmptyRegistryIsReady(t *testing.T) {
	t.Parallel()

	got := health.NewRegistry().Check(context.Background())

	if got.Status != health.StatusReady {
		t.Errorf("Status = %q, want %q", got.Status, health.StatusReady)
	}
	if len(got.Checks) != 0 {
		t.Errorf("Checks = %d, want 0", len(got.Checks))
	}
}

func TestAllPassingChecksAreReady(t *testing.T) {
	t.Parallel()

	r := health.NewRegistry()
	r.Register("database", func(context.Context) error { return nil })
	r.Register("temporal", func(context.Context) error { return nil })

	got := r.Check(context.Background())

	if got.Status != health.StatusReady {
		t.Errorf("Status = %q, want %q", got.Status, health.StatusReady)
	}
	if len(got.Checks) != 2 {
		t.Fatalf("Checks = %d, want 2", len(got.Checks))
	}
	for _, c := range got.Checks {
		if c.Status != health.StatusReady {
			t.Errorf("check %q Status = %q, want %q", c.Name, c.Status, health.StatusReady)
		}
	}
}

// One failing dependency makes the whole service unready. Partial readiness
// would let a broken deployment take traffic.
func TestOneFailingCheckMakesTheServiceUnready(t *testing.T) {
	t.Parallel()

	r := health.NewRegistry()
	r.Register("database", func(context.Context) error { return nil })
	r.Register("temporal", func(context.Context) error { return errors.New("dial tcp: connection refused") })

	got := r.Check(context.Background())

	if got.Status != health.StatusUnready {
		t.Errorf("Status = %q, want %q", got.Status, health.StatusUnready)
	}
}

// The reason a dependency failed is operational detail. It must never reach a
// response body, because a dependency error can carry a connection string.
func TestFailureReasonIsNotExposed(t *testing.T) {
	t.Parallel()

	r := health.NewRegistry()
	r.Register("database", func(context.Context) error {
		return errors.New("postgres://prepeet:hunter2@db.internal:5432 refused")
	})

	got := r.Check(context.Background())

	for _, c := range got.Checks {
		if c.Name != "database" {
			continue
		}
		if c.Detail != "" {
			t.Errorf("Detail = %q, want empty: dependency errors must not be exposed", c.Detail)
		}
	}
}

// Checks are named so an operator can tell which dependency is down without
// reading the error that produced it.
func TestChecksAreReportedInRegistrationOrder(t *testing.T) {
	t.Parallel()

	r := health.NewRegistry()
	for _, name := range []string{"database", "objectstore", "temporal"} {
		r.Register(name, func(context.Context) error { return nil })
	}

	got := r.Check(context.Background())

	want := []string{"database", "objectstore", "temporal"}
	if len(got.Checks) != len(want) {
		t.Fatalf("Checks = %d, want %d", len(got.Checks), len(want))
	}
	for i, name := range want {
		if got.Checks[i].Name != name {
			t.Errorf("Checks[%d].Name = %q, want %q", i, got.Checks[i].Name, name)
		}
	}
}

// A dependency that hangs must not hang the readiness probe, or a stuck
// database takes the whole deployment's health reporting with it.
func TestSlowCheckIsCancelledByContext(t *testing.T) {
	t.Parallel()

	r := health.NewRegistry()
	r.Register("temporal", func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return nil
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	got := r.Check(ctx)

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Check took %s, want it to respect the context deadline", elapsed)
	}
	if got.Status != health.StatusUnready {
		t.Errorf("Status = %q, want %q", got.Status, health.StatusUnready)
	}
}
