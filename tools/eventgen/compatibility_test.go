// The rules that stop a consumer being broken.
//
// ADR-0004 names what counts as breaking for an event: removing a payload
// field, changing a field's meaning, and removing an event type without a
// successor. This file is where those become mechanical, because a rule a
// reviewer has to remember is one that holds until the week somebody is busy.
//
// The catalogues here are built in memory rather than read from disk. A test
// that needed a second checkout to exercise "a field was removed" would be slow
// enough that it stopped being run, and it would only ever cover the removals
// somebody had already made.
//
// Implements the third acceptance criterion of CTR-03.
package main

import (
	"strings"
	"testing"
)

// catalogueOf builds a catalogue from a compact description, so each test below
// reads as the change it is about rather than as schema construction.
func catalogueOf(events map[string]*Event) *Catalogue {
	return &Catalogue{Envelope: &Schema{}, Events: events}
}

func event(schemaVersion string, required []string, properties map[string]*Schema) *Event {
	return &Event{
		SchemaVersion: schemaVersion,
		Payload: &Schema{
			Required:   required,
			Properties: properties,
		},
	}
}

func field(kind string, enum ...string) *Schema {
	return &Schema{Type: kind, Enum: enum}
}

// breaksWith runs the comparison and returns each break as it is rendered.
//
// Rendered rather than the Reason field alone, because what an engineer reads
// in CI is the rendering, and a message that omits which event changed is one
// they cannot act on. Asserting against the field would let that regress.
func breaksWith(t *testing.T, previous, current *Catalogue) []string {
	t.Helper()

	rendered := make([]string, 0)
	for _, b := range Compare(previous, current) {
		rendered = append(rendered, b.String())
	}
	return rendered
}

func TestAnUnchangedCatalogueBreaksNothing(t *testing.T) {
	// Without this every assertion below could pass against a comparison that
	// reports everything as broken.
	c := catalogueOf(map[string]*Event{
		"identity.user_registered.v1": event("1.0", []string{"user_id"},
			map[string]*Schema{"user_id": field("string")}),
	})

	if breaks := Compare(c, c); len(breaks) != 0 {
		t.Fatalf("an unchanged catalogue reported %d breaks: %v", len(breaks), breaks)
	}
}

func TestRemovingAPayloadFieldBreaks(t *testing.T) {
	previous := catalogueOf(map[string]*Event{
		"identity.user_registered.v1": event("1.0", []string{"user_id"},
			map[string]*Schema{"user_id": field("string"), "account_type": field("string")}),
	})
	current := catalogueOf(map[string]*Event{
		"identity.user_registered.v1": event("1.1", []string{"user_id"},
			map[string]*Schema{"user_id": field("string")}),
	})

	reasons := breaksWith(t, previous, current)
	if len(reasons) != 1 || !strings.Contains(reasons[0], "account_type") {
		t.Fatalf("removing a field was not reported: %v", reasons)
	}
}

func TestAddingAnOptionalFieldIsAdditive(t *testing.T) {
	// The whole point. A consumer tolerates a field it does not know, so adding
	// one must not be refused, or nothing could ever be added.
	previous := catalogueOf(map[string]*Event{
		"identity.user_registered.v1": event("1.0", []string{"user_id"},
			map[string]*Schema{"user_id": field("string")}),
	})
	current := catalogueOf(map[string]*Event{
		"identity.user_registered.v1": event("1.1", []string{"user_id"},
			map[string]*Schema{"user_id": field("string"), "referrer": field("string")}),
	})

	if reasons := breaksWith(t, previous, current); len(reasons) != 0 {
		t.Fatalf("an additive change was reported as breaking: %v", reasons)
	}
}

func TestAddingAFieldWithoutBumpingTheSchemaVersionIsRefused(t *testing.T) {
	// A consumer decides what it can handle from schema_version. An additive
	// change that does not bump it is a change the consumer has no way to
	// notice, which is how an addition becomes a surprise.
	previous := catalogueOf(map[string]*Event{
		"identity.user_registered.v1": event("1.0", []string{"user_id"},
			map[string]*Schema{"user_id": field("string")}),
	})
	current := catalogueOf(map[string]*Event{
		"identity.user_registered.v1": event("1.0", []string{"user_id"},
			map[string]*Schema{"user_id": field("string"), "referrer": field("string")}),
	})

	reasons := breaksWith(t, previous, current)
	if len(reasons) != 1 || !strings.Contains(reasons[0], "schema version") {
		t.Fatalf("an unversioned addition was not reported: %v", reasons)
	}
}

func TestChangingAFieldsTypeBreaks(t *testing.T) {
	previous := catalogueOf(map[string]*Event{
		"interview.session_completed.v1": event("1.0", nil,
			map[string]*Schema{"turn_count": field("integer")}),
	})
	current := catalogueOf(map[string]*Event{
		"interview.session_completed.v1": event("1.1", nil,
			map[string]*Schema{"turn_count": field("string")}),
	})

	reasons := breaksWith(t, previous, current)
	if len(reasons) != 1 || !strings.Contains(reasons[0], "turn_count") {
		t.Fatalf("a type change was not reported: %v", reasons)
	}
}

func TestRemovingAnEnumValueBreaks(t *testing.T) {
	// The closest mechanical proxy for "changing a field's meaning". A consumer
	// with a branch per value loses one, and the branch it loses is the one
	// nobody tested.
	previous := catalogueOf(map[string]*Event{
		"review.decision_recorded.v1": event("1.0", nil,
			map[string]*Schema{"decision": field("string", "advance", "reject", "hold")}),
	})
	current := catalogueOf(map[string]*Event{
		"review.decision_recorded.v1": event("1.1", nil,
			map[string]*Schema{"decision": field("string", "advance", "reject")}),
	})

	reasons := breaksWith(t, previous, current)
	if len(reasons) != 1 || !strings.Contains(reasons[0], "hold") {
		t.Fatalf("a removed enum value was not reported: %v", reasons)
	}
}

func TestAddingAnEnumValueIsAdditive(t *testing.T) {
	previous := catalogueOf(map[string]*Event{
		"review.decision_recorded.v1": event("1.0", nil,
			map[string]*Schema{"decision": field("string", "advance", "reject")}),
	})
	current := catalogueOf(map[string]*Event{
		"review.decision_recorded.v1": event("1.1", nil,
			map[string]*Schema{"decision": field("string", "advance", "reject", "hold")}),
	})

	if reasons := breaksWith(t, previous, current); len(reasons) != 0 {
		t.Fatalf("an added enum value was reported as breaking: %v", reasons)
	}
}

func TestRemovingAnEventTypeBreaks(t *testing.T) {
	previous := catalogueOf(map[string]*Event{
		"appeal.requested.v1": event("1.0", nil, nil),
	})
	current := catalogueOf(map[string]*Event{})

	reasons := breaksWith(t, previous, current)
	if len(reasons) != 1 || !strings.Contains(reasons[0], "appeal.requested.v1") {
		t.Fatalf("a removed event type was not reported: %v", reasons)
	}
}

func TestRemovingAnEventTypeThatHasASuccessorIsAllowed(t *testing.T) {
	// ADR-0004 permits retiring a version once its successor exists, which is
	// the whole reason the version lives in the event type.
	previous := catalogueOf(map[string]*Event{
		"appeal.requested.v1": event("1.0", nil, nil),
	})
	current := catalogueOf(map[string]*Event{
		"appeal.requested.v2": event("1.0", nil, nil),
	})

	if reasons := breaksWith(t, previous, current); len(reasons) != 0 {
		t.Fatalf("retiring a version with a successor was reported as breaking: %v", reasons)
	}
}

func TestAnEarlierVersionIsNotASuccessor(t *testing.T) {
	// v2 removed while only v1 remains is a rollback of the contract, not a
	// migration, and a consumer that moved to v2 has nothing to receive.
	previous := catalogueOf(map[string]*Event{
		"appeal.requested.v1": event("1.0", nil, nil),
		"appeal.requested.v2": event("1.0", nil, nil),
	})
	current := catalogueOf(map[string]*Event{
		"appeal.requested.v1": event("1.0", nil, nil),
	})

	if reasons := breaksWith(t, previous, current); len(reasons) != 1 {
		t.Fatalf("removing the newer version was not reported: %v", reasons)
	}
}

func TestANewEventTypeIsAdditive(t *testing.T) {
	previous := catalogueOf(map[string]*Event{})
	current := catalogueOf(map[string]*Event{
		"appeal.requested.v1": event("1.0", nil, nil),
	})

	if reasons := breaksWith(t, previous, current); len(reasons) != 0 {
		t.Fatalf("a new event type was reported as breaking: %v", reasons)
	}
}

func TestEveryBreakNamesItsEventAndItsFix(t *testing.T) {
	// A gate that says "breaking change detected" and stops teaches people to
	// bypass it. Each break has to say which event, what changed, and what to
	// do instead.
	previous := catalogueOf(map[string]*Event{
		"identity.user_registered.v1": event("1.0", nil,
			map[string]*Schema{"user_id": field("string"), "account_type": field("string")}),
	})
	current := catalogueOf(map[string]*Event{
		"identity.user_registered.v1": event("1.1", nil,
			map[string]*Schema{"user_id": field("string")}),
	})

	breaks := Compare(previous, current)
	if len(breaks) != 1 {
		t.Fatalf("expected one break, got %d", len(breaks))
	}
	if breaks[0].EventType != "identity.user_registered.v1" {
		t.Errorf("the break does not name its event: %+v", breaks[0])
	}
	if breaks[0].Remedy == "" {
		t.Errorf("the break offers no remedy: %+v", breaks[0])
	}
}
