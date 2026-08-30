package tenantadmin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// The rubric library: TEN-04.
//
// This builds no versioning of its own, and that is the whole design. The
// artifact registry already stores versioned, digest-identified, published
// artifacts with a lifecycle, a separation of duties on publication, an
// immutability trigger and a rollback path, and a rubric is one of its types.
// A second version history here would be a second answer to "what is version
// 1.1.0 of this rubric", and the second answer is the one that drifts. So the
// library is a surface: it decides what a workspace may do with a rubric, and
// the registry decides what a version is.
//
// Reached through ports, per ADR-0005, because the registry is another
// bounded context and so are campaigns. cmd wires all three.

// RubricArtifactType is the registry's name for what this library manages.
// Named here so the library never guesses at another context's vocabulary
// from a string literal at a call site.
const RubricArtifactType = "rubric"

// Rubric library refusals.
var (
	// ErrRubricInvalid means the body is not a rubric the evaluator could
	// use. Nothing is written: the registry deletes nothing past a draft, so
	// an unusable version would be a row somebody has to explain forever.
	ErrRubricInvalid = errors.New("tenantadmin: RUBRIC_INVALID: that is not a usable rubric")
	// ErrRubricInUse refuses removing a rubric a running campaign resolves.
	ErrRubricInUse = errors.New("tenantadmin: RUBRIC_IN_USE: a running campaign is still using this rubric")
	// ErrRubricImmutable refuses changing or discarding anything past a
	// draft. A change is a new version; that is ADR-0011 and not this
	// library's to soften.
	ErrRubricImmutable = errors.New("tenantadmin: RUBRIC_IMMUTABLE: a published rubric is not edited, it is superseded")
	// ErrRubricNotOwn refuses acting on the platform catalogue's templates
	// from a workspace surface. Every workspace can read them; none owns one.
	ErrRubricNotOwn = errors.New("tenantadmin: RUBRIC_NOT_OWN: that rubric belongs to the platform catalogue")
	// ErrRubricNotFound covers absence and another workspace's rubric alike.
	ErrRubricNotFound = errors.New("tenantadmin: RUBRIC_NOT_FOUND: no such rubric")
)

// RubricInUseError names the campaigns still using a rubric.
//
// The names are carried rather than only counted, because a refusal an
// administrator can act on has to say what to go and look at; "no" with
// nothing attached is a dead end.
type RubricInUseError struct {
	Reference string
	Campaigns []string
}

func (e *RubricInUseError) Error() string {
	return fmt.Sprintf("tenantadmin: RUBRIC_IN_USE: %s is still used by %s",
		e.Reference, strings.Join(e.Campaigns, ", "))
}

// Unwrap makes every in-use refusal answer errors.Is(err, ErrRubricInUse).
func (e *RubricInUseError) Unwrap() error { return ErrRubricInUse }

// ArtifactVersion is one version of one artifact, as this library needs it.
//
// A mirror of the registry's own shape at the port rather than an import of
// it. The duplication is the boundary: cmd converts, and neither context has
// to change when the other's internals do.
type ArtifactVersion struct {
	ID            string
	Type          string
	Reference     string
	Version       string
	SchemaVersion string
	Digest        string
	Body          json.RawMessage
	Status        string
	// TenantID is empty for a platform artifact, which every workspace reads
	// and none owns.
	TenantID    string
	CreatedBy   string
	CreatedAt   time.Time
	PublishedBy string
	PublishedAt *time.Time
}

// ArtifactDraft is what drafting hands the registry.
type ArtifactDraft struct {
	Type          string
	Reference     string
	Version       string
	SchemaVersion string
	Body          json.RawMessage
	TenantID      string
	CreatedBy     string
}

// Registry is what the rubric library needs from the artifact registry.
//
// Narrow on purpose: the library never reads by digest and never rolls back,
// because pins belong to sessions and rollback is the registry's own operation
// with its own capability. What is not in this interface is as much of the
// design as what is.
type Registry interface {
	CreateDraft(ctx context.Context, draft ArtifactDraft) (ArtifactVersion, error)
	Get(ctx context.Context, artifactID, tenantID string) (ArtifactVersion, error)
	Transition(ctx context.Context, artifact ArtifactVersion, to string) (ArtifactVersion, error)
	Publish(ctx context.Context, artifact ArtifactVersion, publisherID string) (ArtifactVersion, error)
	Versions(ctx context.Context, reference, tenantID string) ([]ArtifactVersion, error)
	ListByType(ctx context.Context, artifactType, tenantID string) ([]ArtifactVersion, error)
	DeleteDraft(ctx context.Context, artifactID, tenantID string) error
}

// RubricValidator decides whether a body is a rubric the evaluator can use.
//
// Injected rather than implemented here, for the reason the artifact loader
// injects its catalogue parser: the check belongs to the context that reads
// the type, and this one must not import it. Writing a second rubric schema
// here would be the same mistake as writing a second version history.
type RubricValidator interface {
	Validate(body json.RawMessage) error
}

// RubricUsage answers whether a running campaign still needs a rubric.
//
// Campaigns are another context's, and the library asks rather than looks.
// The answer is a list of names because it goes into the refusal.
type RubricUsage interface {
	InUse(ctx context.Context, tenantID, reference string) ([]string, error)
}

// RubricDraft is what an author supplies.
type RubricDraft struct {
	Reference     string
	Version       string
	SchemaVersion string
	Body          json.RawMessage
}

// RubricVersion is one version of one rubric, in the library's words.
type RubricVersion struct {
	ArtifactID string
	Reference  string
	Version    string
	Digest     string
	Status     string
	Body       json.RawMessage
	// Template marks the platform catalogue's rubrics, which every workspace
	// may read and build on and none may change.
	Template    bool
	DraftedBy   string
	DraftedAt   time.Time
	PublishedBy string
	PublishedAt *time.Time
}

// RubricLibrary is the workspace's surface over the artifact registry.
type RubricLibrary struct {
	registry  Registry
	validator RubricValidator
	usage     RubricUsage
}

// NewRubricLibrary wires the library over its three ports.
func NewRubricLibrary(registry Registry, validator RubricValidator, usage RubricUsage) *RubricLibrary {
	return &RubricLibrary{registry: registry, validator: validator, usage: usage}
}

// Draft writes a new rubric version, after checking it is one.
//
// This is also how a published rubric is revised: there is no edit. A change
// is a draft of the same reference at a new version, which is the only shape
// the registry accepts and the only shape this library offers.
func (l *RubricLibrary) Draft(ctx context.Context, tenantID, authorID string, draft RubricDraft) (RubricVersion, error) {
	if err := l.validator.Validate(draft.Body); err != nil {
		return RubricVersion{}, fmt.Errorf("%w: %v", ErrRubricInvalid, err)
	}

	created, err := l.registry.CreateDraft(ctx, ArtifactDraft{
		Type: RubricArtifactType, Reference: draft.Reference, Version: draft.Version,
		SchemaVersion: draft.SchemaVersion, Body: draft.Body,
		TenantID: tenantID, CreatedBy: authorID,
	})
	if err != nil {
		return RubricVersion{}, fmt.Errorf("tenantadmin: drafting the rubric: %w", err)
	}
	return asRubricVersion(created), nil
}

// SubmitForApproval moves a draft into validation, which is the step that
// makes a rubric somebody else's to look at rather than the author's to keep
// editing.
func (l *RubricLibrary) SubmitForApproval(ctx context.Context, tenantID, artifactID string) (RubricVersion, error) {
	return l.advance(ctx, tenantID, artifactID, "validating")
}

// Approve marks a validated rubric ready to publish. Publication is still a
// separate act by a separate person: the registry refuses a publish whose
// actor drafted the version, whatever this library allows.
func (l *RubricLibrary) Approve(ctx context.Context, tenantID, artifactID string) (RubricVersion, error) {
	return l.advance(ctx, tenantID, artifactID, "approved")
}

// Publish makes a rubric the one new campaigns resolve.
//
// Nothing already running changes: campaigns pin a digest, and publication
// moves only the pointer. That is the registry's guarantee and the reason
// this library does not need one of its own.
func (l *RubricLibrary) Publish(ctx context.Context, tenantID, publisherID, artifactID string) (RubricVersion, error) {
	artifact, err := l.own(ctx, tenantID, artifactID)
	if err != nil {
		return RubricVersion{}, err
	}
	published, err := l.registry.Publish(ctx, artifact, publisherID)
	if err != nil {
		return RubricVersion{}, fmt.Errorf("tenantadmin: publishing the rubric: %w", err)
	}
	return asRubricVersion(published), nil
}

// History returns every version of one rubric, with who published each and
// when. Read from the registry, which is the only place that knows.
func (l *RubricLibrary) History(ctx context.Context, tenantID, reference string) ([]RubricVersion, error) {
	versions, err := l.registry.Versions(ctx, reference, tenantID)
	if err != nil {
		return nil, fmt.Errorf("tenantadmin: reading the rubric's history: %w", err)
	}
	history := make([]RubricVersion, 0, len(versions))
	for _, version := range versions {
		history = append(history, asRubricVersion(version))
	}
	return history, nil
}

// List returns the workspace's rubrics and the platform's templates.
//
// Both, because the templates are what a new workspace starts from and hiding
// them would leave the library empty for exactly the people who most need
// something to copy. Which is which is marked rather than implied.
func (l *RubricLibrary) List(ctx context.Context, tenantID string) ([]RubricVersion, error) {
	artifacts, err := l.registry.ListByType(ctx, RubricArtifactType, tenantID)
	if err != nil {
		return nil, fmt.Errorf("tenantadmin: listing rubrics: %w", err)
	}
	listed := make([]RubricVersion, 0, len(artifacts))
	for _, artifact := range artifacts {
		listed = append(listed, asRubricVersion(artifact))
	}
	return listed, nil
}

// DiscardDraft removes a rubric that was never published.
//
// Two refusals stand in front of it. A version past draft is history and is
// never removed, which is the registry's rule restated here so the author is
// told why rather than shown a trigger. And a rubric a running campaign is
// using is not removed even as a draft: a campaign that pinned a draft is a
// campaign whose evidence would stop resolving.
func (l *RubricLibrary) DiscardDraft(ctx context.Context, tenantID, actorID, artifactID string) error {
	artifact, err := l.own(ctx, tenantID, artifactID)
	if err != nil {
		return err
	}
	if artifact.Status != "draft" {
		return ErrRubricImmutable
	}
	if err := l.refuseWhileInUse(ctx, tenantID, artifact.Reference); err != nil {
		return err
	}
	if err := l.registry.DeleteDraft(ctx, artifactID, tenantID); err != nil {
		return fmt.Errorf("tenantadmin: discarding the draft: %w", err)
	}
	return nil
}

// Retire takes a published rubric out of use without removing it.
//
// Deprecated rather than deleted, because "what was this candidate judged by"
// has to stay answerable after the rubric stops being offered. It is refused
// while a campaign is still running against it, which is the third box: a
// campaign losing its rubric mid-round would leave a hiring decision resting
// on something the workspace has said it no longer stands behind.
func (l *RubricLibrary) Retire(ctx context.Context, tenantID, actorID, artifactID string) error {
	artifact, err := l.own(ctx, tenantID, artifactID)
	if err != nil {
		return err
	}
	if err := l.refuseWhileInUse(ctx, tenantID, artifact.Reference); err != nil {
		return err
	}
	if _, err := l.registry.Transition(ctx, artifact, "deprecated"); err != nil {
		return fmt.Errorf("tenantadmin: retiring the rubric: %w", err)
	}
	return nil
}

// advance moves a version one step along the registry's lifecycle.
func (l *RubricLibrary) advance(ctx context.Context, tenantID, artifactID, to string) (RubricVersion, error) {
	artifact, err := l.own(ctx, tenantID, artifactID)
	if err != nil {
		return RubricVersion{}, err
	}
	moved, err := l.registry.Transition(ctx, artifact, to)
	if err != nil {
		return RubricVersion{}, fmt.Errorf("tenantadmin: moving the rubric to %s: %w", to, err)
	}
	return asRubricVersion(moved), nil
}

// own reads a rubric the workspace may act on.
//
// The platform catalogue's rubrics are readable from every workspace, which
// makes "I can see it" and "it is mine to change" two different questions.
// This answers the second, and every mutating operation goes through it.
func (l *RubricLibrary) own(ctx context.Context, tenantID, artifactID string) (ArtifactVersion, error) {
	artifact, err := l.registry.Get(ctx, artifactID, tenantID)
	if errors.Is(err, ErrRubricNotFound) {
		return ArtifactVersion{}, ErrRubricNotFound
	}
	if err != nil {
		return ArtifactVersion{}, fmt.Errorf("tenantadmin: reading the rubric: %w", err)
	}
	if artifact.TenantID == "" {
		return ArtifactVersion{}, ErrRubricNotOwn
	}
	if artifact.TenantID != tenantID {
		return ArtifactVersion{}, ErrRubricNotFound
	}
	return artifact, nil
}

// refuseWhileInUse asks the campaign side and turns a yes into a refusal that
// names what is blocking.
func (l *RubricLibrary) refuseWhileInUse(ctx context.Context, tenantID, reference string) error {
	campaigns, err := l.usage.InUse(ctx, tenantID, reference)
	if err != nil {
		return fmt.Errorf("tenantadmin: checking whether %s is in use: %w", reference, err)
	}
	if len(campaigns) > 0 {
		return &RubricInUseError{Reference: reference, Campaigns: campaigns}
	}
	return nil
}

// asRubricVersion projects a registry artifact into the library's words.
func asRubricVersion(artifact ArtifactVersion) RubricVersion {
	return RubricVersion{
		ArtifactID: artifact.ID, Reference: artifact.Reference, Version: artifact.Version,
		Digest: artifact.Digest, Status: artifact.Status, Body: artifact.Body,
		Template: artifact.TenantID == "",
		// Drafting provenance is the registry's created_by and created_at:
		// there is no separate "author" record, because a version and its
		// author are the same row.
		DraftedBy: artifact.CreatedBy, DraftedAt: artifact.CreatedAt,
		PublishedBy: artifact.PublishedBy, PublishedAt: artifact.PublishedAt,
	}
}
