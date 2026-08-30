// Package recruiting owns campaigns: the unit an employer issues invitations
// under, and the configuration a screening interview is run against.
//
// The whole package turns on one distinction. A campaign is assembled by
// reference, because a recruiter chooses "the backend rubric", and it is opened
// by digest, because what a candidate is actually interviewed against has to be
// a specific thing that cannot move afterwards. Opening is where the first
// becomes the second, and it is the only moment the package resolves anything.
//
// It cannot see content.artifacts. recruiting owns the recruiting schema and
// nothing else, so whether an artifact is published is asked through a port the
// composition root fills in, per ADR-0005. That is not ceremony: it is what
// stops a query here from quietly becoming the second place that decides what
// "published" means.
//
// Implements SCR-01 and the campaign half of ADR-0020.
package recruiting

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Status is where a campaign is in its life.
type Status string

const (
	// StatusDraft accepts configuration changes and issues no invitations.
	StatusDraft Status = "draft"
	// StatusOpen has frozen its configuration and may issue invitations.
	StatusOpen Status = "open"
	// StatusClosed issues nothing further. Its evidence remains readable,
	// because a closed campaign is still a hiring record.
	StatusClosed Status = "closed"
)

// requiredArtifacts are the kinds a campaign must pin before it may open.
//
// Each is here because an interview cannot run without it, not because the
// catalogue happens to have one. A campaign missing its persona has no
// interviewer; missing its plan, no structure; missing its rubric or
// calibration, nothing to evaluate against or any basis for comparing what it
// finds. Discovering any of those at the candidate's first invitation is too
// late, which is why this is checked at open rather than at issue.
var requiredArtifacts = []string{"rubric", "calibration", "persona", "plan"}

// Errors a caller is expected to distinguish. Each maps to a different thing
// the recruiter has to do next, which is the test of whether a distinct error
// earns its existence.
var (
	// ErrNotPublished means an artifact the campaign names is not published.
	// The recruiter's next move is to publish it or choose another.
	ErrNotPublished = errors.New("recruiting: the artifact is not published")
	// ErrNoDetermination means the jurisdiction has no recorded legal
	// determination. Nobody's next move is in the product: ADR-0020 makes this
	// the refusal that keeps DEC-11 honest, and it is answered by counsel.
	ErrNoDetermination = errors.New("recruiting: no legal determination for this jurisdiction")
	// ErrIncompleteConfiguration means a required artifact kind is unpinned.
	ErrIncompleteConfiguration = errors.New("recruiting: the campaign configuration is incomplete")
	// ErrNotDraft means the campaign has already opened. Reopening would
	// re-resolve every pin against whatever is current now, which is the drift
	// the digest pinning exists to prevent.
	ErrNotDraft = errors.New("recruiting: only a draft campaign can be opened")
	// ErrDuplicateArtifact means one kind was pinned twice.
	ErrDuplicateArtifact = errors.New("recruiting: one artifact of each kind")
)

// Artifact is what recruiting needs to know about a piece of content, which is
// deliberately less than content knows.
type Artifact struct {
	Reference string
	Type      string
	// Digest is the identity. content.artifacts calls it what sessions pin, and
	// a campaign pins it for the same reason.
	Digest  string
	Version string
}

// Determination is one jurisdiction's answers under ADR-0020.
type Determination struct {
	ID               string
	Jurisdiction     string
	Version          int
	ResultDisclosure string
	AppealStatus     string
	Approver         string
	ApprovedAt       time.Time
}

// Campaign is one employer's hiring effort for one role.
type Campaign struct {
	ID              string
	TenantID        string
	Name            string
	Status          Status
	RoleReference   string
	Jurisdiction    string
	DeterminationID string
	OpenedAt        *time.Time
	ClosedAt        *time.Time
	CreatedAt       time.Time
	CreatedBy       string
}

// PinRequest is a recruiter's choice, still by reference.
type PinRequest struct {
	Type      string
	Reference string
}

// Pin is that choice resolved, and is what the campaign stores.
type Pin struct {
	Type       string
	Reference  string
	Version    string
	Digest     string
	ArtifactID string
}

// Opening is everything a campaign needs to become open, resolved together.
//
// Returned rather than written, so the decision and its persistence are
// separable: the caller writes the campaign, its pins and its determination in
// one transaction, and this function has no opinion about transactions.
type Opening struct {
	Determination Determination
	Pins          []Pin
}

// Artifacts answers whether a piece of content is published, and what its
// digest is if so. Filled in by the composition root from content's store.
type Artifacts interface {
	// PublishedArtifact returns the artifact only when it is published, and
	// ErrNotPublished otherwise. The two cases are one answer deliberately: a
	// caller that could ask "does it exist" separately from "is it published"
	// would eventually ask only the first.
	PublishedArtifact(ctx context.Context, tenantID, reference string) (Artifact, error)
}

// Determinations answers what the law of a jurisdiction requires of screening.
type Determinations interface {
	// LatestDetermination returns ErrNoDetermination when there is none, which
	// is the normal state of every jurisdiction until counsel records one.
	LatestDetermination(ctx context.Context, jurisdiction string) (Determination, error)
}

// Service decides whether a campaign may open, and against what.
type Service struct {
	artifacts      Artifacts
	determinations Determinations
}

// NewService builds the service over its two ports.
func NewService(artifacts Artifacts, determinations Determinations) *Service {
	return &Service{artifacts: artifacts, determinations: determinations}
}

// ResolveOpening checks everything a campaign needs and resolves what it pins.
//
// The order of the checks is deliberate. The determination comes first because
// it is the one failure nobody in the product can fix, so reporting it before a
// list of unpublished artifacts saves a recruiter from fixing things that will
// not help. Completeness comes before publication because "you have not chosen
// a persona" is a clearer instruction than "the persona you did not choose is
// not published".
func (s *Service) ResolveOpening(ctx context.Context, campaign Campaign, requests []PinRequest) (Opening, error) {
	if campaign.Status != StatusDraft {
		return Opening{}, fmt.Errorf("%w: it is %s", ErrNotDraft, campaign.Status)
	}

	determination, err := s.determinations.LatestDetermination(ctx, campaign.Jurisdiction)
	if err != nil {
		return Opening{}, fmt.Errorf("%w: %s", ErrNoDetermination, campaign.Jurisdiction)
	}

	chosen := make(map[string]PinRequest, len(requests))
	for _, request := range requests {
		if _, already := chosen[request.Type]; already {
			return Opening{}, fmt.Errorf("%w: two of %s", ErrDuplicateArtifact, request.Type)
		}
		chosen[request.Type] = request
	}
	for _, kind := range requiredArtifacts {
		if _, found := chosen[kind]; !found {
			return Opening{}, fmt.Errorf("%w: no %s", ErrIncompleteConfiguration, kind)
		}
	}

	pins := make([]Pin, 0, len(requests))
	for _, kind := range requiredArtifacts {
		request := chosen[kind]
		artifact, err := s.artifacts.PublishedArtifact(ctx, campaign.TenantID, request.Reference)
		if err != nil {
			// The reference is named and the error wrapped, because a recruiter
			// with four unpublished artifacts needs to know which one this is.
			return Opening{}, fmt.Errorf("%w: %s", ErrNotPublished, request.Reference)
		}
		pins = append(pins, Pin{
			Type:      kind,
			Reference: artifact.Reference,
			Version:   artifact.Version,
			Digest:    artifact.Digest,
		})
	}

	return Opening{Determination: determination, Pins: pins}, nil
}
