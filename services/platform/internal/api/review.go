package api

import (
	"context"
	"errors"
	"net/http"

	prepeetapi "github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
)

// The evidence-first review: REV-02.
//
// Everything a reviewer weighs, framed as evidence rather than a verdict.
// The port's answer deliberately has no recommendation and no aggregate
// number, because the contract promises their absence and a field that
// hinted at a decision would be the platform quietly making one. Reading
// this is a recorded event: the contract declares the operation a
// sensitive read, so REV-04's interlock commits the audit row before the
// response is written and refuses the read when it cannot.

// The refusals this surface translates.
var (
	// ErrReviewSessionMissing covers a session that does not exist on the
	// campaign and one on another campaign alike, so an id cannot be
	// probed across campaigns.
	ErrReviewSessionMissing = errors.New("api: no such screening session on this campaign")
	// ErrReviewNotReady means the session exists and its evaluation has
	// not been published yet: a different situation with a different next
	// step (wait), so a different code.
	ErrReviewNotReady = errors.New("api: the evaluation is not published yet")
)

// PinnedConfigurationView names what the session actually ran, by digest.
type PinnedConfigurationView struct {
	BundleDigest       string
	Rubric             RubricPinView
	AggregationVersion string
	ExtractionVersion  string
	ModelVersion       string
	PolicyVersion      string
}

// ReviewCoverageView names what the conversation reached, by competency.
type ReviewCoverageView struct {
	Reached    []string
	NotReached []string
	Covered    int
	Total      int
}

// RequirementFindingView is one job requirement's report, with its text so
// the screen never has to fetch the requirement to render the finding.
type RequirementFindingView struct {
	RequirementID string
	Text          string
	Status        string
	Competencies  []string
	EvidenceIDs   []string
	FollowUp      string
}

// RequirementsReportView is EVL-06's report at the port: findings and the
// mapping rules' version, and deliberately nothing that counts them.
type RequirementsReportView struct {
	MapVersion   string
	Requirements []RequirementFindingView
}

// ScreeningReviewView is the review document at the port.
type ScreeningReviewView struct {
	SessionID      string
	Pinned         PinnedConfigurationView
	Competencies   []CompetencyResultView
	Evidence       []EvidenceSpanView
	Coverage       ReviewCoverageView
	Contradictions []ContradictionView
	Requirements   RequirementsReportView
}

// ScreeningReviews is what this surface needs of the composition in cmd,
// where interview, evaluation and recruiting meet.
type ScreeningReviews interface {
	ScreeningReview(ctx context.Context, tenantID, campaignID, sessionID string) (ScreeningReviewView, error)
}

// reviewHandlers serves REV-02's surface.
type reviewHandlers struct {
	authentication *authentication
	campaigns      Recruiting
	reviews        ScreeningReviews
}

func (h *reviewHandlers) caller(ctx context.Context) (Principal, *failure) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		refusal := h.authentication.rejectedSession(ctx)
		return Principal{}, &refusal
	}
	principal, err := h.authentication.identity.Authorize(ctx, presented, requiredCapabilityFrom(ctx))
	if err != nil {
		refusal := h.authentication.failed(ctx, err)
		return Principal{}, &refusal
	}
	return principal, nil
}

// GetScreeningReview answers the review for a recruiter on the campaign.
func (h *reviewHandlers) GetScreeningReview(ctx context.Context, request prepeetapi.GetScreeningReviewRequestObject) (prepeetapi.GetScreeningReviewResponseObject, error) {
	principal, refused := h.caller(ctx)
	if refused != nil {
		return *refused, nil
	}
	campaign, err := h.campaigns.CampaignForRecruiter(ctx, principal.ActiveTenantID, request.CampaignID.String(), principal.UserID)
	if err != nil {
		return h.reviewFailure(ctx, err), nil
	}

	review, err := h.reviews.ScreeningReview(ctx, principal.ActiveTenantID, campaign.ID, request.SessionID.String())
	if err != nil {
		return h.reviewFailure(ctx, err), nil
	}

	body := prepeetapi.ScreeningReview{
		SessionID:      campaignUUID(review.SessionID),
		Contradictions: make([]prepeetapi.ContradictionView, 0, len(review.Contradictions)),
		Competencies:   make([]prepeetapi.CompetencyResultView, 0, len(review.Competencies)),
		Evidence:       make([]prepeetapi.EvidenceSpanView, 0, len(review.Evidence)),
	}
	body.Pinned.BundleDigest = review.Pinned.BundleDigest
	body.Pinned.Rubric.Reference = review.Pinned.Rubric.Reference
	body.Pinned.Rubric.Version = review.Pinned.Rubric.Version
	body.Pinned.Rubric.Digest = review.Pinned.Rubric.Digest
	body.Pinned.AggregationVersion = review.Pinned.AggregationVersion
	body.Pinned.ExtractionVersion = review.Pinned.ExtractionVersion
	body.Pinned.ModelVersion = review.Pinned.ModelVersion
	body.Pinned.PolicyVersion = review.Pinned.PolicyVersion

	for _, competency := range review.Competencies {
		body.Competencies = append(body.Competencies, competencyResultBody(competency))
	}
	for _, span := range review.Evidence {
		body.Evidence = append(body.Evidence, prepeetapi.EvidenceSpanView{
			ID: span.ID, CompetencyID: span.CompetencyID,
			Kind:            prepeetapi.EvidenceSpanViewKind(span.Kind),
			Quote:           span.Quote,
			SegmentSequence: span.SegmentSequence,
			StartMs:         span.StartMs, EndMs: span.EndMs,
		})
	}
	body.Coverage.Reached = orEmpty(review.Coverage.Reached)
	body.Coverage.NotReached = orEmpty(review.Coverage.NotReached)
	body.Coverage.Covered = review.Coverage.Covered
	body.Coverage.Total = review.Coverage.Total
	for _, pair := range review.Contradictions {
		body.Contradictions = append(body.Contradictions, contradictionBody(pair))
	}
	body.Requirements.MapVersion = review.Requirements.MapVersion
	body.Requirements.Requirements = make([]prepeetapi.RequirementFinding, 0, len(review.Requirements.Requirements))
	for _, finding := range review.Requirements.Requirements {
		encoded := prepeetapi.RequirementFinding{
			RequirementID: campaignUUID(finding.RequirementID),
			Text:          finding.Text,
			Status:        prepeetapi.RequirementFindingStatus(finding.Status),
			Competencies:  orEmpty(finding.Competencies),
			EvidenceIds:   orEmpty(finding.EvidenceIDs),
		}
		if finding.FollowUp != "" {
			followUp := finding.FollowUp
			encoded.FollowUp = &followUp
		}
		body.Requirements.Requirements = append(body.Requirements.Requirements, encoded)
	}

	return prepeetapi.GetScreeningReview200JSONResponse{
		Body:    body,
		Headers: prepeetapi.GetScreeningReview200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// reviewFailure translates the refusals the review surface can meet.
func (h *reviewHandlers) reviewFailure(ctx context.Context, err error) failure {
	base := h.authentication.failed(ctx, err)
	switch {
	case errors.Is(err, ErrCampaignNoAccess):
		base.status = http.StatusNotFound
		base.code = string(prepeetapi.NOTFOUND)
		base.message = "There is no campaign at that identifier."
	case errors.Is(err, ErrReviewSessionMissing):
		base.status = http.StatusNotFound
		base.code = string(prepeetapi.NOTFOUND)
		base.message = "There is no screening session at that identifier on this campaign."
	case errors.Is(err, ErrReviewNotReady):
		base.status = http.StatusConflict
		base.code = "REVIEW_NOT_READY"
		base.message = "This screening has not published its evaluation yet. It appears here the moment it does."
	}
	return base
}

// competencyResultBody encodes one competency's aggregation for the wire.
// Confidence and the sufficiency counts travel in the same object as the
// band: uncertainty beside every score, never in a footnote.
func competencyResultBody(competency CompetencyResultView) prepeetapi.CompetencyResultView {
	encoded := prepeetapi.CompetencyResultView{
		CompetencyID:  competency.CompetencyID,
		Status:        prepeetapi.CompetencyResultViewStatus(competency.Status),
		Confidence:    prepeetapi.CompetencyResultViewConfidence(competency.Confidence),
		EvidenceCount: competency.EvidenceCount,
		Supporting:    competency.Supporting,
		Contradictory: competency.Contradictory,
		Unverified:    competency.Unverified,
		Gaps:          competency.Gaps,
		EvidenceIds:   orEmpty(competency.EvidenceIDs),
		ReasonCodes:   orEmpty(competency.ReasonCodes),
	}
	if competency.Band != "" {
		band := competency.Band
		encoded.Band = &band
	}
	return encoded
}

// contradictionBody encodes one neutral pair.
func contradictionBody(pair ContradictionView) prepeetapi.ContradictionView {
	return prepeetapi.ContradictionView{
		Topic: orEmpty(pair.Topic),
		SideA: prepeetapi.ContradictionSideView{
			SegmentSequence: pair.SideA.SegmentSequence, Quote: pair.SideA.Quote,
			StartMs: pair.SideA.StartMs, EndMs: pair.SideA.EndMs,
		},
		SideB: prepeetapi.ContradictionSideView{
			SegmentSequence: pair.SideB.SegmentSequence, Quote: pair.SideB.Quote,
			StartMs: pair.SideB.StartMs, EndMs: pair.SideB.EndMs,
		},
	}
}

// orEmpty keeps a nil slice off the wire as [] rather than null: the
// contract's arrays are required, and null is a shape surprise.
func orEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func (f failure) VisitGetScreeningReviewResponse(w http.ResponseWriter) error {
	return f.write(w)
}
