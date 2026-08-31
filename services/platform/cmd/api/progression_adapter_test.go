package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/internal/catalog"
	"github.com/Yelethe1st/prepeet/services/platform/internal/progression"
)

// The targeting seam: progression asks the catalogue what a role could be
// asked about, and gets back identifiers in the form its observations
// already use. Progression's own rules are its package's suite; this
// checks only the translation, which is the part that can silently stop
// lining up.

// stubCatalogueSource serves one catalogue document, so the adapter can be
// exercised without a registry or a database.
type stubCatalogueSource struct {
	body json.RawMessage
}

func (s stubCatalogueSource) ResolveBody(_ context.Context, _, _ string) (json.RawMessage, string, error) {
	return s.body, "sha256:stub", nil
}

func targetingCatalogue() *catalog.Service {
	return catalog.NewService(stubCatalogueSource{body: json.RawMessage(`{
		"disciplines": [{"id": "software-engineering", "name": "Software engineering"}],
		"shapes": [{"id": "mixed", "name": "Mixed", "description": "x", "minutes": [30]}],
		"personas": [{"id": "p", "name": "P", "style": "s", "voice": "v", "description": "x", "best_for": "b", "shapes": []}],
		"roles": [{"id": "backend-engineer", "discipline": "software-engineering",
		           "title": "Backend engineer", "organisation": "O",
		           "competencies": ["Systems design", "Debugging", "Testing"],
		           "shapes": ["mixed"]}]
	}`)})
}

func TestTheTargetingPortAnswersCatalogueCompetencyIdentifiers(t *testing.T) {
	competencies, err := roleCompetencies(targetingCatalogue()).
		Competencies(context.Background(), "backend-engineer")
	if err != nil {
		t.Fatalf("competencies: %v", err)
	}
	// The same derivation evaluation uses. A recommendation naming
	// "Systems design" could never be shown to have been covered by an
	// observation of "systems-design".
	want := []string{"systems-design", "debugging", "testing"}
	if len(competencies) != len(want) {
		t.Fatalf("competencies = %v, want %v", competencies, want)
	}
	for index, expected := range want {
		if competencies[index] != expected {
			t.Errorf("competency %d = %q, want %q", index, competencies[index], expected)
		}
	}
}

func TestAnUnknownRoleAnswersNothingAndTargetingRefusesIt(t *testing.T) {
	// The two halves of one behaviour: the adapter reports the honest
	// absence, and progression turns it into a refusal rather than
	// composing a session out of nothing.
	_, err := progression.NewTargeting(roleCompetencies(targetingCatalogue())).
		Recommend(context.Background(), progression.TargetingRequest{
			RoleID:          "role-that-was-retired",
			RubricReference: "rubric/practice-default",
			Bands:           []string{"emerging", "solid"},
			Slots:           3,
		}, nil, time.Now())
	if err == nil {
		t.Fatal("a retired role composed a session anyway")
	}
}

func TestARecommendationCoversTheGapsTheCatalogueKnowsAbout(t *testing.T) {
	// PRG-05 end to end across the port: a weak reading of a real
	// catalogued competency reaches the recommendation as a covered slot.
	recommendation, err := progression.NewTargeting(roleCompetencies(targetingCatalogue())).
		Recommend(context.Background(), progression.TargetingRequest{
			RoleID:          "backend-engineer",
			RubricReference: "rubric/practice-default",
			Bands:           []string{"emerging", "solid", "strong"},
			Slots:           2,
		}, []progression.Observation{{
			ID: "o1", CompetencyID: "debugging", Status: "assessed", Band: "emerging",
			RubricReference: "rubric/practice-default", RubricVersion: "1.0.0",
			ObservedAt: time.Now().AddDate(0, 0, -3),
		}}, time.Now())
	if err != nil {
		t.Fatalf("recommend: %v", err)
	}
	covered := false
	for _, competency := range recommendation.Covers {
		if competency == "debugging" {
			covered = true
		}
	}
	if !covered {
		t.Fatalf("the gap is not covered: %v", recommendation.Covers)
	}
	if len(recommendation.Targeted) >= len(recommendation.Covers) {
		t.Fatalf("targeting took the whole session: %v of %v",
			recommendation.Targeted, recommendation.Covers)
	}
}
