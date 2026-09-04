package main

import (
	"context"
	"errors"
	"fmt"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/internal/evaluation"
	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/internal/recruiting"
	"github.com/Yelethe1st/prepeet/services/platform/platform/objectstore"
)

// The review, composed: REV-02.
//
// Interview says which session the campaign ran and what it pinned;
// evaluation holds the published result, the evidence and the
// contradictions; recruiting holds the frozen requirements; and the sealed
// input document - fetched from the object store, the same bytes Python
// evaluated - names the competencies, so the requirement mapping links
// against exactly what the interview measured rather than a copy that
// could drift. cmd is the only place all four may meet.
type reviewAdapter struct {
	sessions     *interview.Store
	results      *evaluation.Store
	requirements *recruiting.Store
	documents    *objectstore.S3Store
}

func newReviewAdapter(sessions *interview.Store, results *evaluation.Store,
	requirements *recruiting.Store, documents *objectstore.S3Store) reviewAdapter {
	return reviewAdapter{
		sessions: sessions, results: results,
		requirements: requirements, documents: documents,
	}
}

var _ api.ScreeningReviews = reviewAdapter{}

func (a reviewAdapter) ScreeningReview(ctx context.Context, tenantID, campaignID, sessionID string) (api.ScreeningReviewView, error) {
	// The session must be the campaign's own: resolved through the
	// campaign's list so an id from another campaign answers as absence.
	sessions, err := a.sessions.CampaignSessions(ctx, tenantID, campaignID)
	if err != nil {
		return api.ScreeningReviewView{}, err
	}
	var session *interview.CampaignSession
	for i := range sessions {
		if sessions[i].ID == sessionID {
			session = &sessions[i]
			break
		}
	}
	if session == nil {
		return api.ScreeningReviewView{}, api.ErrReviewSessionMissing
	}

	ref := evaluation.SessionRef{
		SessionID: sessionID, Mode: "screening",
		CandidateID: session.CandidateID, TenantID: tenantID,
	}
	result, err := a.results.ResultOf(ctx, ref)
	if errors.Is(err, evaluation.ErrNoResult) {
		return api.ScreeningReviewView{}, api.ErrReviewNotReady
	}
	if err != nil {
		return api.ScreeningReviewView{}, err
	}
	spans, err := a.results.List(ctx, ref)
	if err != nil {
		return api.ScreeningReviewView{}, err
	}
	contradictions, err := a.results.Contradictions(ctx, ref)
	if err != nil {
		return api.ScreeningReviewView{}, err
	}

	// The competencies the requirement mapping links against come from the
	// sealed input: the same bytes Python evaluated, never a copy that
	// could drift. A review that cannot read them refuses loudly rather
	// than mapping against nothing and calling every requirement
	// not_assessable.
	key, err := objectstore.SealedInputKey("screening", tenantID, session.CandidateID, sessionID)
	if err != nil {
		return api.ScreeningReviewView{}, err
	}
	body, err := a.documents.Fetch(ctx, key)
	if err != nil {
		return api.ScreeningReviewView{}, fmt.Errorf("fetching the sealed input: %w", err)
	}
	sealed, err := evaluation.DecodeSealedInput(body)
	if err != nil {
		return api.ScreeningReviewView{}, err
	}

	frozen, err := a.requirements.RequirementsForCampaign(ctx, tenantID, campaignID)
	if err != nil {
		return api.ScreeningReviewView{}, err
	}
	requirements := make([]evaluation.JobRequirement, 0, len(frozen))
	textByID := make(map[string]string, len(frozen))
	for _, requirement := range frozen {
		// A rejected requirement was reviewed out of the job context; it
		// is not part of what the interview answers for.
		if requirement.Status == recruiting.RequirementRejected {
			continue
		}
		requirements = append(requirements, evaluation.JobRequirement{
			ID: requirement.ID, Text: requirement.Text,
		})
		textByID[requirement.ID] = requirement.Text
	}
	report := evaluation.MapRequirements(requirements, sealed.Competencies,
		result.Aggregation.Competencies, spans)

	view := api.ScreeningReviewView{
		SessionID: sessionID,
		Pinned: api.PinnedConfigurationView{
			BundleDigest: session.BundleDigest,
			Rubric: api.RubricPinView{
				Reference: result.RubricReference,
				Version:   result.RubricVersion,
				Digest:    result.RubricDigest,
			},
			AggregationVersion: result.AggregationVersion,
			ExtractionVersion:  result.ExtractionVersion,
			ModelVersion:       result.ModelVersion,
			PolicyVersion:      result.PolicyVersion,
		},
		Coverage: api.ReviewCoverageView{
			Reached:    result.Aggregation.Coverage.Reached,
			NotReached: result.Aggregation.Coverage.NotReached,
			Covered:    result.Aggregation.CoveredCompetencies,
			Total:      result.Aggregation.TotalCompetencies,
		},
	}
	for _, competency := range result.Aggregation.Competencies {
		view.Competencies = append(view.Competencies, api.CompetencyResultView{
			CompetencyID:  competency.CompetencyID,
			Status:        competency.Status,
			Confidence:    competency.Confidence,
			Band:          competency.Band,
			EvidenceIDs:   competency.EvidenceIDs,
			EvidenceCount: competency.EvidenceCount,
			Supporting:    competency.Supporting,
			Contradictory: competency.Contradictory,
			Unverified:    competency.Unverified,
			Gaps:          competency.Gaps,
			ReasonCodes:   competency.ReasonCodes,
		})
	}
	for _, span := range spans {
		view.Evidence = append(view.Evidence, api.EvidenceSpanView{
			ID: span.ID, CompetencyID: span.CompetencyID, Kind: span.Kind,
			Quote: span.Quote, SegmentSequence: span.SegmentSequence,
			StartMs: span.StartMs, EndMs: span.EndMs,
		})
	}
	for _, pair := range contradictions {
		view.Contradictions = append(view.Contradictions, api.ContradictionView{
			Topic: pair.Topic,
			SideA: api.ContradictionSideView{
				SegmentSequence: pair.SideA.SegmentSequence, Quote: pair.SideA.Quote,
				StartMs: pair.SideA.StartMs, EndMs: pair.SideA.EndMs,
			},
			SideB: api.ContradictionSideView{
				SegmentSequence: pair.SideB.SegmentSequence, Quote: pair.SideB.Quote,
				StartMs: pair.SideB.StartMs, EndMs: pair.SideB.EndMs,
			},
		})
	}
	view.Requirements = api.RequirementsReportView{
		MapVersion:   report.MapVersion,
		Requirements: make([]api.RequirementFindingView, 0, len(report.Requirements)),
	}
	for _, finding := range report.Requirements {
		view.Requirements.Requirements = append(view.Requirements.Requirements,
			api.RequirementFindingView{
				RequirementID: finding.RequirementID,
				Text:          textByID[finding.RequirementID],
				Status:        finding.Status,
				Competencies:  finding.Competencies,
				EvidenceIDs:   finding.EvidenceIDs,
				FollowUp:      finding.FollowUp,
			})
	}
	return view, nil
}
