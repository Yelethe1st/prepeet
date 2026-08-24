//go:build integration

package outbox_test

import (
	"encoding/json"
	"testing"

	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetevents"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

// Fixtures built from the catalogue rather than written out.
//
// Before CTR-03 these tests published invented types like concurrency.probe.v1,
// which is what you write when there is no catalogue to publish against. Now
// there is one, and the outbox refuses anything outside it.
//
// Deriving the fixture from the contract rather than hand-writing one per type
// means a contract that gains a required field does not silently leave a test
// publishing an event that would now be refused in production. The alternative,
// a table of literal payloads, rots the first time somebody edits a schema and
// the test keeps passing because it was never the thing being asserted.

// catalogueEvent builds a publishable event of the given type.
//
// The payload carries a placeholder for every required field. Placeholders are
// enough because the outbox checks which fields are present, not what they
// contain: field types are the schema's business and are checked where the
// contract is generated, not on every publish.
func catalogueEvent(t *testing.T, eventType prepeetevents.Type) outbox.Event {
	t.Helper()

	definition, declared := prepeetevents.Catalogue[eventType]
	if !declared {
		t.Fatalf("%s is not in the catalogue; the test is asking for an event that cannot be published", eventType)
	}

	fields := map[string]string{}
	for _, required := range definition.Required {
		fields[required] = "placeholder"
	}
	payload, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("building a payload for %s: %v", eventType, err)
	}

	return outbox.Event{
		Type:          string(eventType),
		SchemaVersion: definition.SchemaVersion,
		Producer:      definition.Owner,
		Actor:         outbox.Actor{Type: "service", ID: "test"},
		Payload:       payload,
	}
}

// The types these tests use when they need several distinct ones. Real events
// rather than probes, because a test that publishes something the system would
// refuse is testing a path production does not have.
const (
	probeA = prepeetevents.IdentityUserRegisteredV1
	probeB = prepeetevents.TenantMembershipChangedV1
	probeC = prepeetevents.InterviewSessionStartedV1
	probeD = prepeetevents.InterviewSessionCompletedV1
	probeE = prepeetevents.EvaluationRequestedV1
)

// The fixture builder is itself worth a test: every event above has to be
// publishable, and a builder that quietly produced an invalid one would make
// every test using it fail for a reason that has nothing to do with what it
// asserts.
func TestEveryCatalogueEventCanBeBuiltAndPublished(t *testing.T) {
	for eventType := range prepeetevents.Catalogue {
		event := catalogueEvent(t, eventType)

		tx, err := pool.Begin(t.Context())
		if err != nil {
			t.Fatalf("beginning: %v", err)
		}
		if _, err := outbox.New(pool).Publish(t.Context(), tx, event); err != nil {
			t.Errorf("%s cannot be published from its own contract: %v", eventType, err)
		}
		_ = tx.Rollback(t.Context())
	}
}
