// The gate that keeps the event catalogue and the contract from drifting apart.
//
// ADR-0004 makes the hand-authored schema the source and the documentation the
// reasoning behind it. That division only works if the two are checked against
// each other: a documented event with no schema is an event nobody can produce,
// and a schema with no documentation is one nobody knows the purpose of.
//
// Implements the first acceptance criterion of CTR-03.
package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// repoRoot is two directories up from this test, which lives in tools/eventgen.
const repoRoot = "../.."

// catalogRow matches one row of the "Initial events" table in the catalogue
// document: the event type in backticks, then the producer, then consumers.
var catalogRow = regexp.MustCompile("^\\| `([a-z_.0-9]+)` \\| ([^|]+) \\| ([^|]+) \\|$")

// documentedEvents reads the event types the catalogue document promises.
func documentedEvents(t *testing.T) map[string]string {
	t.Helper()

	path := filepath.Join(repoRoot, "docs/contracts/event-catalog.md")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	events := map[string]string{}
	for _, line := range strings.Split(string(source), "\n") {
		if match := catalogRow.FindStringSubmatch(strings.TrimSpace(line)); match != nil {
			events[match[1]] = strings.TrimSpace(match[2])
		}
	}

	// A parser that silently matches nothing would make every assertion below
	// pass against an empty set, which is the failure mode of every test that
	// reads a document.
	if len(events) == 0 {
		t.Fatalf("parsed no events from %s; the table format has changed", path)
	}
	return events
}

func TestEveryDocumentedEventHasASchema(t *testing.T) {
	documented := documentedEvents(t)

	catalogue, err := Load(filepath.Join(repoRoot, "packages/contracts/events"))
	if err != nil {
		t.Fatalf("loading the event contracts: %v", err)
	}

	for eventType := range documented {
		if _, ok := catalogue.Events[eventType]; !ok {
			t.Errorf("%s is in event-catalog.md with no schema in packages/contracts/events.\n"+
				"    An event nobody can produce is documentation rather than a contract.", eventType)
		}
	}
}

func TestEverySchemaIsDocumented(t *testing.T) {
	documented := documentedEvents(t)

	catalogue, err := Load(filepath.Join(repoRoot, "packages/contracts/events"))
	if err != nil {
		t.Fatalf("loading the event contracts: %v", err)
	}

	for eventType := range catalogue.Events {
		if _, ok := documented[eventType]; !ok {
			t.Errorf("%s has a schema but is absent from event-catalog.md.\n"+
				"    The document is where a consumer learns the event exists and who owns it.", eventType)
		}
	}
}

func TestEveryEventNamesAnOwnerAndAgreesWithTheDocument(t *testing.T) {
	documented := documentedEvents(t)

	catalogue, err := Load(filepath.Join(repoRoot, "packages/contracts/events"))
	if err != nil {
		t.Fatalf("loading the event contracts: %v", err)
	}

	for eventType, event := range catalogue.Events {
		if event.Owner == "" {
			t.Errorf("%s declares no owner.\n"+
				"    An event with no owner is one nobody is answerable for when a consumer breaks.", eventType)
		}

		// The producer is the owner, per ADR-0004: only the context owning the
		// authoritative state emits its event. Checking it against the document
		// catches the case where one of the two was edited alone.
		if producer, ok := documented[eventType]; ok && !strings.EqualFold(producer, event.Owner) {
			t.Errorf("%s is produced by %q in event-catalog.md and owned by %q in its schema.",
				eventType, producer, event.Owner)
		}
	}
}

func TestEveryEventTypeCarriesItsContractVersion(t *testing.T) {
	catalogue, err := Load(filepath.Join(repoRoot, "packages/contracts/events"))
	if err != nil {
		t.Fatalf("loading the event contracts: %v", err)
	}

	// ADR-0004: the version in the event type is the contract version and the
	// only one consumers subscribe against. An event type without one cannot be
	// superseded without breaking every consumer at once.
	suffix := regexp.MustCompile(`\.v[0-9]+$`)
	names := make([]string, 0, len(catalogue.Events))
	for eventType := range catalogue.Events {
		names = append(names, eventType)
	}
	sort.Strings(names)

	for _, eventType := range names {
		if !suffix.MatchString(eventType) {
			t.Errorf("%s has no .vN suffix, so there is no way to emit a successor beside it.", eventType)
		}
	}
}
