//go:build integration

package candidate_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/candidate"
)

// PRO-04 against real PostgreSQL: the candidate inspects and corrects what
// extraction proposed. The properties under test are the ticket's three
// boxes: every fact carries its span, confidence and correction state; a
// correction never destroys the original extraction; and the effective read
// - the one composition consumes - serves the correction and omits the
// rejected, from the moment the candidate acts.

// factsFixture uploads a CV and stores a fixed extraction for it.
func factsFixture(t *testing.T, userID string) (service *candidate.Documents, documentID string) {
	t.Helper()
	ctx := context.Background()
	service = documents(t)
	stored := uploadCV(t, service, userID, []byte("%PDF-1.7 facts fixture "+userID))

	activities := candidate.NewExtractionActivities(candidate.NewStore(pool), &fakeExtractor{})
	if err := activities.StoreFacts(ctx,
		candidate.ExtractionInput{DocumentID: stored.ID, CandidateID: userID}, cvFacts()); err != nil {
		t.Fatalf("storing the extraction: %v", err)
	}
	return service, stored.ID
}

func TestEveryFactCarriesItsProvenanceAndReviewState(t *testing.T) {
	ctx := context.Background()
	service, documentID := factsFixture(t, amaraID)

	facts, err := service.ListFacts(ctx, amaraID, documentID)
	if err != nil {
		t.Fatalf("ListFacts: %v", err)
	}
	if len(facts) != len(cvFacts()) {
		t.Fatalf("%d facts, want %d", len(facts), len(cvFacts()))
	}
	for _, fact := range facts {
		if fact.SpanEnd <= fact.SpanStart {
			t.Fatalf("fact %s has no span: %d-%d", fact.ID, fact.SpanStart, fact.SpanEnd)
		}
		if fact.Confidence < 0 || fact.Confidence > 1 {
			t.Fatalf("fact %s confidence = %v", fact.ID, fact.Confidence)
		}
		if fact.Status != "proposed" || fact.ReviewedAt != nil {
			t.Fatalf("a fresh extraction must arrive proposed and unreviewed: %+v", fact)
		}
		if fact.ExtractorVersion == "" {
			t.Fatalf("fact %s carries no extractor version", fact.ID)
		}
	}
}

func TestCorrectingRecordsTheCorrectionAndKeepsTheOriginal(t *testing.T) {
	// The second box. The extracted value is never rewritten: the correction
	// lives beside it, visibly, and un-correcting brings the original back
	// because the original never went anywhere.
	ctx := context.Background()
	service, documentID := factsFixture(t, amaraID)

	facts, err := service.ListFacts(ctx, amaraID, documentID)
	if err != nil {
		t.Fatalf("ListFacts: %v", err)
	}
	var skill candidate.Fact
	for _, fact := range facts {
		if fact.Kind == "skill" {
			skill = fact
		}
	}

	corrected, err := service.ReviewFact(ctx, amaraID, skill.ID, "corrected",
		json.RawMessage(`{"name":"Golang"}`))
	if err != nil {
		t.Fatalf("correcting: %v", err)
	}
	if corrected.Status != "corrected" || corrected.ReviewedAt == nil {
		t.Fatalf("correction state = %+v", corrected)
	}
	if string(corrected.CorrectedValue) != `{"name": "Golang"}` && string(corrected.CorrectedValue) != `{"name":"Golang"}` {
		t.Fatalf("corrected value = %s", corrected.CorrectedValue)
	}
	if string(corrected.Value) != string(skill.Value) {
		t.Fatalf("the original extraction changed: %s -> %s", skill.Value, corrected.Value)
	}

	// Confirming afterwards returns to the extraction, correction cleared.
	confirmed, err := service.ReviewFact(ctx, amaraID, skill.ID, "confirmed", nil)
	if err != nil {
		t.Fatalf("confirming after correcting: %v", err)
	}
	if confirmed.CorrectedValue != nil {
		t.Fatalf("a confirmed fact still carries a correction: %s", confirmed.CorrectedValue)
	}
	if string(confirmed.Value) != string(skill.Value) {
		t.Fatal("the original did not survive the round trip")
	}
}

func TestTheEffectiveReadServesCorrectionsAndOmitsRejections(t *testing.T) {
	// The third box: what composition reads. The corrected skill arrives as
	// the correction, the rejected role does not arrive at all, and the
	// untouched facts arrive as extracted.
	ctx := context.Background()
	service, documentID := factsFixture(t, priyaID)

	facts, err := service.ListFacts(ctx, priyaID, documentID)
	if err != nil {
		t.Fatalf("ListFacts: %v", err)
	}
	var skillID, roleID string
	for _, fact := range facts {
		switch fact.Kind {
		case "skill":
			skillID = fact.ID
		case "role":
			roleID = fact.ID
		}
	}
	if _, err := service.ReviewFact(ctx, priyaID, skillID, "corrected",
		json.RawMessage(`{"name":"Golang"}`)); err != nil {
		t.Fatalf("correcting: %v", err)
	}
	if _, err := service.ReviewFact(ctx, priyaID, roleID, "rejected", nil); err != nil {
		t.Fatalf("rejecting: %v", err)
	}

	effective, err := service.EffectiveFacts(ctx, priyaID)
	if err != nil {
		t.Fatalf("EffectiveFacts: %v", err)
	}
	byID := map[string]candidate.EffectiveFact{}
	for _, fact := range effective {
		byID[fact.ID] = fact
	}
	if _, present := byID[roleID]; present {
		t.Fatal("a rejected fact reached the effective read")
	}
	var value struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(byID[skillID].Value, &value); err != nil || value.Name != "Golang" {
		t.Fatalf("the corrected fact did not win the effective read: %s (%v)", byID[skillID].Value, err)
	}
}

func TestTheReviewRefusesWhatItCannotHonestly(t *testing.T) {
	ctx := context.Background()
	service, documentID := factsFixture(t, amaraID)
	facts, _ := service.ListFacts(ctx, amaraID, documentID)
	factID := facts[0].ID

	// A correction without a corrected value is a status that lies.
	if _, err := service.ReviewFact(ctx, amaraID, factID, "corrected", nil); !errors.Is(err, candidate.ErrFactReview) {
		t.Fatalf("corrected-without-value = %v, want ErrFactReview", err)
	}
	// A status outside the lifecycle.
	if _, err := service.ReviewFact(ctx, amaraID, factID, "blessed", nil); !errors.Is(err, candidate.ErrFactReview) {
		t.Fatalf("unknown status = %v, want ErrFactReview", err)
	}
	// A corrected value that is not a JSON object.
	if _, err := service.ReviewFact(ctx, amaraID, factID, "corrected",
		json.RawMessage(`"just a string"`)); !errors.Is(err, candidate.ErrFactReview) {
		t.Fatalf("non-object correction = %v, want ErrFactReview", err)
	}

	// Another person's fact does not exist, even by id.
	if _, err := service.ReviewFact(ctx, priyaID, factID, "confirmed", nil); !errors.Is(err, candidate.ErrFactNotFound) {
		t.Fatalf("cross-owner review = %v, want ErrFactNotFound", err)
	}
	// Nor does another person's fact list.
	if _, err := service.ListFacts(ctx, priyaID, documentID); !errors.Is(err, candidate.ErrDocumentNotFound) {
		t.Fatalf("cross-owner list = %v, want ErrDocumentNotFound", err)
	}
}
