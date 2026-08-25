package catalog_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/catalog"
)

// The catalogue's rules, before any endpoint: a document either coheres or is
// refused at parse, and a selection either combines validly or is refused
// field by field. Both live here because CAT-03's promise is that the server
// owns validity - the browser filters nothing it could get wrong.

func document(t *testing.T) catalog.Catalogue {
	t.Helper()
	parsed, err := catalog.Parse(json.RawMessage(body))
	if err != nil {
		t.Fatalf("parsing the fixture: %v", err)
	}
	return parsed
}

const body = `{
  "disciplines": [
    {"id": "software-engineering", "name": "Software engineering"},
    {"id": "nursing", "name": "Nursing"}
  ],
  "shapes": [
    {"id": "shape_behavioural", "name": "Behavioural", "description": "Competency questions.", "minutes": [15, 25, 40]},
    {"id": "shape_technical", "name": "Technical deep-dive", "description": "Verbal reasoning.", "minutes": [25, 40]},
    {"id": "shape_panel", "name": "Panel simulation", "description": "Rotating viewpoints.", "minutes": [40, 60]}
  ],
  "personas": [
    {"id": "per_ama", "name": "Ama", "style": "Warm and structured", "voice": "Mid-tone, unhurried", "description": "Opens with context.", "best_for": "First sessions", "shapes": []},
    {"id": "per_lena", "name": "Lena", "style": "Panel chair", "voice": "Formal, measured", "description": "Runs a panel format.", "best_for": "Panels", "shapes": ["shape_panel"]}
  ],
  "roles": [
    {"id": "rl_swe", "discipline": "software-engineering", "title": "Senior Backend Engineer", "organisation": "Product company", "competencies": ["Systems design"], "shapes": ["shape_behavioural", "shape_technical"]},
    {"id": "rl_rn", "discipline": "nursing", "title": "Registered Nurse", "organisation": "Health system", "competencies": ["Clinical reasoning"], "shapes": ["shape_behavioural", "shape_panel"]}
  ]
}`

func TestAParsedCatalogueServesItsCollections(t *testing.T) {
	parsed := document(t)

	if len(parsed.Disciplines) != 2 || len(parsed.Shapes) != 3 || len(parsed.Personas) != 2 || len(parsed.Roles) != 2 {
		t.Fatalf("collections = %d/%d/%d/%d", len(parsed.Disciplines), len(parsed.Shapes), len(parsed.Personas), len(parsed.Roles))
	}
	// A profession is a data change: nothing below the parse knows the names,
	// so the fixture's nursing role is served exactly like the software one.
	if parsed.Roles[1].Discipline != "nursing" {
		t.Fatalf("roles = %+v", parsed.Roles)
	}
}

func TestAnIncoherentCatalogueIsRefusedAtParse(t *testing.T) {
	// The registry's validating state is where this runs for real content; an
	// incoherent document must never become the thing endpoints serve.
	cases := map[string]string{
		"a role in no discipline":     `{"disciplines":[],"shapes":[{"id":"s","name":"S","description":"d","minutes":[15]}],"personas":[],"roles":[{"id":"r","discipline":"ghost","title":"T","organisation":"O","competencies":[],"shapes":["s"]}]}`,
		"a role offering no shape":    `{"disciplines":[{"id":"d","name":"D"}],"shapes":[{"id":"s","name":"S","description":"d","minutes":[15]}],"personas":[],"roles":[{"id":"r","discipline":"d","title":"T","organisation":"O","competencies":[],"shapes":[]}]}`,
		"a role with a ghost shape":   `{"disciplines":[{"id":"d","name":"D"}],"shapes":[{"id":"s","name":"S","description":"d","minutes":[15]}],"personas":[],"roles":[{"id":"r","discipline":"d","title":"T","organisation":"O","competencies":[],"shapes":["ghost"]}]}`,
		"a shape with no durations":   `{"disciplines":[{"id":"d","name":"D"}],"shapes":[{"id":"s","name":"S","description":"d","minutes":[]}],"personas":[],"roles":[]}`,
		"a duplicated identifier":     `{"disciplines":[{"id":"d","name":"D"},{"id":"d","name":"E"}],"shapes":[{"id":"s","name":"S","description":"d","minutes":[15]}],"personas":[],"roles":[]}`,
		"a persona for a ghost shape": `{"disciplines":[{"id":"d","name":"D"}],"shapes":[{"id":"s","name":"S","description":"d","minutes":[15]}],"personas":[{"id":"p","name":"P","style":"s","voice":"v","description":"d","best_for":"b","shapes":["ghost"]}],"roles":[]}`,
	}

	for name, raw := range cases {
		if _, err := catalog.Parse(json.RawMessage(raw)); err == nil {
			t.Errorf("%s parsed without complaint", name)
		}
	}
}

func TestAValidSelectionPasses(t *testing.T) {
	refusals := document(t).Validate(catalog.Selection{
		Discipline: "software-engineering", Role: "rl_swe",
		Shape: "shape_technical", Minutes: 40, Persona: "per_ama",
	})

	if len(refusals) != 0 {
		t.Fatalf("a valid selection was refused: %+v", refusals)
	}
}

func TestEveryInvalidCombinationIsRefusedByField(t *testing.T) {
	parsed := document(t)

	cases := []struct {
		name      string
		selection catalog.Selection
		field     string
	}{
		{"an unknown discipline", catalog.Selection{Discipline: "alchemy", Role: "rl_swe", Shape: "shape_technical", Minutes: 40, Persona: "per_ama"}, "discipline"},
		{"an unknown role", catalog.Selection{Discipline: "software-engineering", Role: "rl_ghost", Shape: "shape_technical", Minutes: 40, Persona: "per_ama"}, "role"},
		{"a role outside its discipline", catalog.Selection{Discipline: "nursing", Role: "rl_swe", Shape: "shape_technical", Minutes: 40, Persona: "per_ama"}, "role"},
		{"a shape the role does not offer", catalog.Selection{Discipline: "nursing", Role: "rl_rn", Shape: "shape_technical", Minutes: 40, Persona: "per_ama"}, "shape"},
		{"minutes the shape does not offer", catalog.Selection{Discipline: "software-engineering", Role: "rl_swe", Shape: "shape_technical", Minutes: 60, Persona: "per_ama"}, "minutes"},
		{"an unknown persona", catalog.Selection{Discipline: "software-engineering", Role: "rl_swe", Shape: "shape_technical", Minutes: 40, Persona: "per_ghost"}, "persona"},
		{"a persona outside its shapes", catalog.Selection{Discipline: "software-engineering", Role: "rl_swe", Shape: "shape_technical", Minutes: 40, Persona: "per_lena"}, "persona"},
	}

	for _, test := range cases {
		refusals := parsed.Validate(test.selection)
		if len(refusals) == 0 {
			t.Errorf("%s was accepted", test.name)
			continue
		}
		found := false
		for _, refusal := range refusals {
			if refusal.Field == test.field {
				found = true
				if refusal.Code == "" || refusal.Message == "" {
					t.Errorf("%s: refusal has no code or message: %+v", test.name, refusal)
				}
			}
		}
		if !found {
			t.Errorf("%s: refused, but not on %q: %+v", test.name, test.field, refusals)
		}
	}
}

func TestAPersonaWithNoShapesServesEveryShape(t *testing.T) {
	// An empty shapes list means unrestricted, so adding a shape does not
	// require touching every persona - the common case stays a data change.
	refusals := document(t).Validate(catalog.Selection{
		Discipline: "nursing", Role: "rl_rn", Shape: "shape_panel", Minutes: 60, Persona: "per_ama",
	})

	if len(refusals) != 0 {
		t.Fatalf("the unrestricted persona was refused: %+v", refusals)
	}
}

func TestTheShippedCatalogueParses(t *testing.T) {
	// The artifact in git is the document production publishes; a fixture
	// passing while the real one rots would be this suite testing itself.
	// Read across the module boundary, which is why test-go runs -count=1.
	raw, err := os.ReadFile("../../../intelligence/artifacts/catalogue/catalog@1.0.0.json")
	if err != nil {
		t.Fatalf("reading the shipped catalogue: %v", err)
	}
	var envelope struct {
		Type          string          `json:"type"`
		Reference     string          `json:"reference"`
		Version       string          `json:"version"`
		SchemaVersion string          `json:"schema_version"`
		Body          json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("the artifact envelope does not decode: %v", err)
	}
	if envelope.Type != "catalogue" || envelope.Reference != "catalog" {
		t.Fatalf("envelope = %+v", envelope)
	}
	parsed, err := catalog.Parse(envelope.Body)
	if err != nil {
		t.Fatalf("the shipped catalogue does not parse: %v", err)
	}
	if len(parsed.Disciplines) < 5 {
		t.Fatalf("the shipped catalogue names %d disciplines; the product is not a software-only list", len(parsed.Disciplines))
	}
	for _, role := range parsed.Roles {
		if strings.TrimSpace(role.Title) == "" {
			t.Fatalf("role %s has no title", role.ID)
		}
	}
}
