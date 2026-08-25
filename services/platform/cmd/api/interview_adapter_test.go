package main

import (
	"encoding/json"
	"testing"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/internal/catalog"
)

// The enforcement seam: the catalogue's refusals become the API's field
// errors with nothing dropped and nothing renamed. The catalogue's own rules
// are its package's suite; the store's behaviour is the interview suite's.

func TestSelectionErrorsCarryEveryRefusal(t *testing.T) {
	catalogue, err := catalog.Parse(json.RawMessage(`{
		"disciplines": [{"id": "d", "name": "D"}],
		"shapes": [{"id": "s", "name": "S", "description": "x", "minutes": [15]}],
		"personas": [{"id": "p", "name": "P", "style": "s", "voice": "v", "description": "x", "best_for": "b", "shapes": []}],
		"roles": [{"id": "r", "discipline": "d", "title": "T", "organisation": "O", "competencies": [], "shapes": ["s"]}]
	}`))
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}

	refused := selectionErrors(catalogue, api.InterviewSelection{
		Discipline: "d", Role: "r", Shape: "s", Minutes: 99, Persona: "ghost",
	})
	if refused == nil {
		t.Fatal("an invalid selection passed")
	}
	if len(refused.Fields) != 2 {
		t.Fatalf("refusals = %+v, want minutes and persona", refused.Fields)
	}
	byField := map[string]string{}
	for _, field := range refused.Fields {
		byField[field.Field] = field.Code
	}
	if byField["minutes"] != "DURATION_NOT_OFFERED" || byField["persona"] != "PERSONA_UNKNOWN" {
		t.Fatalf("codes = %v", byField)
	}

	if valid := selectionErrors(catalogue, api.InterviewSelection{
		Discipline: "d", Role: "r", Shape: "s", Minutes: 15, Persona: "p",
	}); valid != nil {
		t.Fatalf("a valid selection was refused: %+v", valid)
	}
}
