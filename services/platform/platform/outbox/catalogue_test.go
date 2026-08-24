package outbox

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// Publication against the catalogue.
//
// The outbox is the only way a fact leaves this system, so it is the only place
// where "is this event one we declared, and does it carry what we promised"
// can be answered once for everybody. A producer that got it wrong is caught
// here rather than by a consumer, days later, in somebody else's system.
//
// Implements the second acceptance criterion of CTR-03.

// valid is an event that should pass, so that each test below can change one
// thing and attribute the failure to that change.
func valid() Event {
	return Event{
		Type:          "identity.user_registered.v1",
		SchemaVersion: "1.0",
		Producer:      "identity",
		Actor:         Actor{Type: "service", ID: "identity"},
		Payload:       json.RawMessage(`{"user_id":"0199...","account_type":"candidate"}`),
	}
}

func TestAValidEventPasses(t *testing.T) {
	// Without this the tests below would all pass against an event that was
	// refused for some unrelated reason.
	if err := valid().validate(); err != nil {
		t.Fatalf("the fixture is refused, so nothing below proves anything: %v", err)
	}
}

func TestAnUndeclaredEventTypeIsRefused(t *testing.T) {
	event := valid()
	event.Type = "identity.user_teleported.v1"

	err := event.validate()
	if !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("an undeclared type was accepted: %v", err)
	}
	// The message has to name the catalogue, because the mistake is almost
	// always a missing contract rather than a typo, and a bare "invalid event"
	// sends somebody looking in the wrong place.
	if !strings.Contains(err.Error(), "catalogue") {
		t.Errorf("the refusal does not mention the catalogue: %v", err)
	}
}

func TestOnlyTheOwningContextMayEmitAnEvent(t *testing.T) {
	// ADR-0004: only the context owning the authoritative state emits its
	// event. Enforced rather than documented, because the failure it prevents
	// is a context publishing a fact it does not own and therefore cannot be
	// sure of.
	event := valid()
	event.Producer = "recruiting"

	if err := event.validate(); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("a context published another's event: %v", err)
	}
}

func TestAPayloadMissingARequiredFieldIsRefused(t *testing.T) {
	event := valid()
	event.Payload = json.RawMessage(`{"user_id":"0199..."}`)

	err := event.validate()
	if !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("a payload missing account_type was accepted: %v", err)
	}
	if !strings.Contains(err.Error(), "account_type") {
		t.Errorf("the refusal does not name the missing field: %v", err)
	}
}

func TestAPayloadWithAnUndeclaredFieldIsRefused(t *testing.T) {
	// The asymmetry is deliberate. A consumer tolerates a field it does not
	// know, because refusing one turns an additive change into a breaking one.
	// A producer does not, because a field nobody declared is one no consumer
	// was told to expect, and it is usually a typo in a field that was meant to
	// be the declared one.
	event := valid()
	event.Payload = json.RawMessage(`{"user_id":"x","account_type":"candidate","email":"a@b.c"}`)

	err := event.validate()
	if !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("an undeclared payload field was accepted: %v", err)
	}
	if !strings.Contains(err.Error(), "email") {
		t.Errorf("the refusal does not name the undeclared field: %v", err)
	}
}

func TestAnOptionalFieldIsAccepted(t *testing.T) {
	event := valid()
	event.Payload = json.RawMessage(`{"user_id":"x","account_type":"organisation","tenant_id":"y"}`)

	if err := event.validate(); err != nil {
		t.Fatalf("a declared optional field was refused: %v", err)
	}
}

func TestAPayloadThatIsNotAnObjectIsRefused(t *testing.T) {
	// The envelope declares payload as an object. A bare array or string would
	// pass a required-field check vacuously, because there are no fields to
	// find missing.
	for name, payload := range map[string]string{
		"array":  `[]`,
		"string": `"user_registered"`,
		"null":   `null`,
	} {
		event := valid()
		event.Payload = json.RawMessage(payload)

		if err := event.validate(); !errors.Is(err, ErrInvalidEvent) {
			t.Errorf("a %s payload was accepted: %v", name, err)
		}
	}
}

func TestTheSchemaVersionMustMatchTheContract(t *testing.T) {
	// The registry is compiled into this binary, so a mismatch cannot come from
	// a rollout: it means somebody set the field by hand, and a schema_version
	// that lies is one a consumer's compatibility logic will act on.
	event := valid()
	event.SchemaVersion = "9.9"

	if err := event.validate(); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("a schema version the contract does not declare was accepted: %v", err)
	}
}
