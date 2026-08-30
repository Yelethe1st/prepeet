package main

import (
	"context"
	"strings"
	"testing"
)

// The refusals that happen before anything is touched.
//
// Everything opsctl does afterwards is the operations context's, and is tested
// there against a real database. What is worth asserting here is that a mistyped
// command, a missing argument or a missing operator is refused with a sentence
// that says what to do, rather than reaching the console and being refused by
// something further away from the person reading the message.

func TestAnEmptyInvocationPrintsTheUsage(t *testing.T) {
	err := run(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "usage:") {
		t.Errorf("run with no arguments returned %v, want the usage", err)
	}
}

func TestAMissingDatabaseIsRefusedBeforeAnythingElse(t *testing.T) {
	t.Setenv("PREPEET_DATABASE_URL", "")
	if err := run(context.Background(), []string{"backlog"}); err == nil ||
		!strings.Contains(err.Error(), "PREPEET_DATABASE_URL") {
		t.Errorf("run without a database returned %v, want it named", err)
	}
}

func TestAnUnknownCommandNamesItself(t *testing.T) {
	t.Setenv("PREPEET_DATABASE_URL", "postgres://user:pass@127.0.0.1:1/prepeet")
	err := run(context.Background(), []string{"delete-everything"})
	if err == nil || !strings.Contains(err.Error(), "delete-everything") {
		t.Errorf("an unknown command returned %v, want it named back", err)
	}
}

// A retry with no reason must not reach the database at all: the reason is the
// only thing that makes the audit row worth having, so the refusal belongs
// where the operator is typing.
func TestAnActionWithoutAReasonIsRefusedAtTheCommandLine(t *testing.T) {
	t.Setenv("PREPEET_DATABASE_URL", "postgres://user:pass@127.0.0.1:1/prepeet")
	t.Setenv("PREPEET_OPERATOR", "00000000-0000-7000-8000-0000000000f1")

	err := run(context.Background(), []string{"retry", "00000000-0000-7000-8000-00000000dead"})
	if err == nil || !strings.Contains(err.Error(), "reason") {
		t.Errorf("a retry with no reason returned %v, want the reason demanded", err)
	}
}

// An action nobody can be named for cannot be audited, and so must not be
// attempted. Refused here as well as in the console, because the console's
// refusal would arrive after a connection and a query.
func TestAnActionWithoutAnOperatorIsRefused(t *testing.T) {
	t.Setenv("PREPEET_DATABASE_URL", "postgres://user:pass@127.0.0.1:1/prepeet")
	t.Setenv("PREPEET_OPERATOR", "")

	err := run(context.Background(), []string{"discard", "00000000-0000-7000-8000-00000000dead", "no longer wanted"})
	if err == nil || !strings.Contains(err.Error(), "PREPEET_OPERATOR") {
		t.Errorf("an anonymous discard returned %v, want the operator demanded", err)
	}
}
