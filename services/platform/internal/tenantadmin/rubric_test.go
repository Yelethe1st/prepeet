package tenantadmin_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/internal/tenantadmin"
)

// TEN-04. The library is a surface over the artifact registry, so what is
// tested here is what the library adds: validation before anything is
// written, the version history as the registry already holds it, and the two
// refusals that are the library's own - a rubric a running campaign is using
// cannot be removed, and a platform template is not a workspace's to touch.
//
// What is NOT tested here is that a published version cannot be edited. That
// is the registry's, enforced by a trigger in migration 0013 and proven in
// content's own suite, and asserting it against a fake registry would prove
// only that the fake was written to agree.

// registry is an in-memory artifact registry: enough of the real one for the
// library's decisions to be visible, and deliberately not a reimplementation
// of its guarantees.
type registry struct {
	versions []tenantadmin.ArtifactVersion
	deleted  []string
	moved    []string
}

func (r *registry) CreateDraft(_ context.Context, draft tenantadmin.ArtifactDraft) (tenantadmin.ArtifactVersion, error) {
	version := tenantadmin.ArtifactVersion{
		ID:        draft.Reference + "@" + draft.Version,
		Type:      draft.Type,
		Reference: draft.Reference,
		Version:   draft.Version,
		Digest:    "sha256:" + draft.Version,
		Body:      draft.Body,
		Status:    "draft",
		TenantID:  draft.TenantID,
		CreatedBy: draft.CreatedBy,
		CreatedAt: time.Now(),
	}
	r.versions = append(r.versions, version)
	return version, nil
}

func (r *registry) Get(_ context.Context, artifactID, tenantID string) (tenantadmin.ArtifactVersion, error) {
	for _, version := range r.versions {
		if version.ID == artifactID && (version.TenantID == tenantID || version.TenantID == "") {
			return version, nil
		}
	}
	return tenantadmin.ArtifactVersion{}, tenantadmin.ErrRubricNotFound
}

func (r *registry) Transition(_ context.Context, artifact tenantadmin.ArtifactVersion, to string) (tenantadmin.ArtifactVersion, error) {
	for i, version := range r.versions {
		if version.ID == artifact.ID {
			r.versions[i].Status = to
			r.moved = append(r.moved, artifact.ID+"->"+to)
			return r.versions[i], nil
		}
	}
	return tenantadmin.ArtifactVersion{}, tenantadmin.ErrRubricNotFound
}

func (r *registry) Publish(_ context.Context, artifact tenantadmin.ArtifactVersion, publisherID string) (tenantadmin.ArtifactVersion, error) {
	published := time.Now()
	for i, version := range r.versions {
		if version.ID == artifact.ID {
			r.versions[i].Status = "published"
			r.versions[i].PublishedBy = publisherID
			r.versions[i].PublishedAt = &published
			return r.versions[i], nil
		}
	}
	return tenantadmin.ArtifactVersion{}, tenantadmin.ErrRubricNotFound
}

func (r *registry) Versions(_ context.Context, reference, tenantID string) ([]tenantadmin.ArtifactVersion, error) {
	var found []tenantadmin.ArtifactVersion
	for _, version := range r.versions {
		if version.Reference == reference && version.TenantID == tenantID {
			found = append(found, version)
		}
	}
	return found, nil
}

func (r *registry) ListByType(_ context.Context, artifactType, tenantID string) ([]tenantadmin.ArtifactVersion, error) {
	var found []tenantadmin.ArtifactVersion
	for _, version := range r.versions {
		if version.Type == artifactType && (version.TenantID == tenantID || version.TenantID == "") {
			found = append(found, version)
		}
	}
	return found, nil
}

func (r *registry) DeleteDraft(_ context.Context, artifactID, _ string) error {
	for i, version := range r.versions {
		if version.ID == artifactID {
			if version.Status != "draft" {
				return errors.New("registry: only a draft may be deleted")
			}
			r.versions = append(r.versions[:i], r.versions[i+1:]...)
			r.deleted = append(r.deleted, artifactID)
			return nil
		}
	}
	return tenantadmin.ErrRubricNotFound
}

// usage is the port that answers whether a running campaign still needs a
// rubric. Campaigns belong to another context; this is what the library asks
// it, and cmd wires the answer.
type usage struct{ campaigns []string }

func (u *usage) InUse(context.Context, string, string) ([]string, error) { return u.campaigns, nil }

// validator stands in for the rubric schema's own parser, which belongs to
// the context that reads rubric bodies and is injected for exactly the reason
// the artifact loader injects its catalogue parser.
type validator struct{ err error }

func (v *validator) Validate(json.RawMessage) error { return v.err }

func library(t *testing.T) (*tenantadmin.RubricLibrary, *registry, *usage, *validator) {
	t.Helper()
	store := &registry{}
	inUse := &usage{}
	check := &validator{}
	return tenantadmin.NewRubricLibrary(store, check, inUse), store, inUse, check
}

const tenant = "00000000-0000-7000-8000-0000000000a1"

func body() json.RawMessage { return json.RawMessage(`{"sufficiency":{"min_supporting":2}}`) }

// Validation runs before anything is written, because the registry never
// deletes anything past a draft and a rubric nobody can use is still a row
// somebody has to explain.
func TestAnInvalidRubricIsNeverDrafted(t *testing.T) {
	t.Parallel()
	rubrics, store, _, check := library(t)
	check.err = errors.New("a rubric with no bands judges nothing")

	_, err := rubrics.Draft(t.Context(), tenant, "author", tenantadmin.RubricDraft{
		Reference: "rubric/clinical-panel", Version: "1.0.0", SchemaVersion: "1.0", Body: body(),
	})
	if !errors.Is(err, tenantadmin.ErrRubricInvalid) {
		t.Fatalf("Draft = %v, want ErrRubricInvalid", err)
	}
	if len(store.versions) != 0 {
		t.Errorf("the registry holds %d versions, want none", len(store.versions))
	}
}

func TestAValidRubricIsDraftedAsARubricArtifact(t *testing.T) {
	t.Parallel()
	rubrics, store, _, _ := library(t)

	drafted, err := rubrics.Draft(t.Context(), tenant, "author", tenantadmin.RubricDraft{
		Reference: "rubric/clinical-panel", Version: "1.0.0", SchemaVersion: "1.0", Body: body(),
	})
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if drafted.Status != "draft" {
		t.Errorf("Status = %q, want draft", drafted.Status)
	}
	if store.versions[0].Type != tenantadmin.RubricArtifactType {
		t.Errorf("the library drafted a %q, want a rubric", store.versions[0].Type)
	}
	if store.versions[0].TenantID != tenant {
		t.Error("the library drafted a rubric outside the workspace that asked for it")
	}
}

// Editing produces a new version, which here means the library offers no
// other way: a revision is a fresh draft of the same reference, and the
// published version it was revised from is untouched.
func TestRevisingAPublishedRubricProducesANewDraft(t *testing.T) {
	t.Parallel()
	rubrics, store, _, _ := library(t)
	ctx := t.Context()

	first, err := rubrics.Draft(ctx, tenant, "author", tenantadmin.RubricDraft{
		Reference: "rubric/clinical-panel", Version: "1.0.0", SchemaVersion: "1.0", Body: body(),
	})
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if _, err := rubrics.Publish(ctx, tenant, "approver", first.ArtifactID); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	revised, err := rubrics.Draft(ctx, tenant, "author", tenantadmin.RubricDraft{
		Reference: "rubric/clinical-panel", Version: "1.1.0", SchemaVersion: "1.0",
		Body: json.RawMessage(`{"sufficiency":{"min_supporting":3}}`),
	})
	if err != nil {
		t.Fatalf("revising: %v", err)
	}
	if revised.Version == first.Version {
		t.Error("a revision reused the published version number")
	}
	if store.versions[0].Status != "published" || string(store.versions[0].Body) != string(body()) {
		t.Error("revising changed the published version rather than adding one")
	}
}

// The version history, from the registry rather than from a second table.
func TestHistoryShowsWhoPublishedWhatAndWhen(t *testing.T) {
	t.Parallel()
	rubrics, _, _, _ := library(t)
	ctx := t.Context()

	first, _ := rubrics.Draft(ctx, tenant, "author", tenantadmin.RubricDraft{
		Reference: "rubric/clinical-panel", Version: "1.0.0", SchemaVersion: "1.0", Body: body(),
	})
	if _, err := rubrics.Publish(ctx, tenant, "approver", first.ArtifactID); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if _, err := rubrics.Draft(ctx, tenant, "author", tenantadmin.RubricDraft{
		Reference: "rubric/clinical-panel", Version: "1.1.0", SchemaVersion: "1.0", Body: body(),
	}); err != nil {
		t.Fatalf("second Draft: %v", err)
	}

	history, err := rubrics.History(ctx, tenant, "rubric/clinical-panel")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("History returned %d versions, want 2", len(history))
	}
	var published tenantadmin.RubricVersion
	for _, version := range history {
		if version.Version == "1.0.0" {
			published = version
		}
	}
	if published.PublishedBy != "approver" {
		t.Errorf("PublishedBy = %q, want the person who published it", published.PublishedBy)
	}
	if published.PublishedAt == nil {
		t.Error("a published version carries no publication time")
	}
	if published.DraftedBy != "author" {
		t.Errorf("DraftedBy = %q, want the person who drafted it", published.DraftedBy)
	}
}

// The third box. The refusal names what is still using it, because "no" with
// nothing to act on is a dead end rather than an answer.
func TestARubricARunningCampaignUsesCannotBeDiscarded(t *testing.T) {
	t.Parallel()
	rubrics, _, inUse, _ := library(t)
	ctx := t.Context()

	drafted, _ := rubrics.Draft(ctx, tenant, "author", tenantadmin.RubricDraft{
		Reference: "rubric/clinical-panel", Version: "1.0.0", SchemaVersion: "1.0", Body: body(),
	})
	inUse.campaigns = []string{"ICU night cover", "ICU day cover"}

	err := rubrics.DiscardDraft(ctx, tenant, "author", drafted.ArtifactID)
	if !errors.Is(err, tenantadmin.ErrRubricInUse) {
		t.Fatalf("DiscardDraft = %v, want ErrRubricInUse", err)
	}
	var blocked *tenantadmin.RubricInUseError
	if !errors.As(err, &blocked) {
		t.Fatalf("DiscardDraft = %T, want *RubricInUseError", err)
	}
	if len(blocked.Campaigns) != 2 {
		t.Errorf("Campaigns = %v, want the two still using it", blocked.Campaigns)
	}
}

func TestARubricNoCampaignUsesCanBeDiscardedWhileItIsStillADraft(t *testing.T) {
	t.Parallel()
	rubrics, store, _, _ := library(t)
	ctx := t.Context()

	drafted, _ := rubrics.Draft(ctx, tenant, "author", tenantadmin.RubricDraft{
		Reference: "rubric/clinical-panel", Version: "1.0.0", SchemaVersion: "1.0", Body: body(),
	})
	if err := rubrics.DiscardDraft(ctx, tenant, "author", drafted.ArtifactID); err != nil {
		t.Fatalf("DiscardDraft: %v", err)
	}
	if len(store.deleted) != 1 {
		t.Errorf("the registry deleted %d versions, want 1", len(store.deleted))
	}
}

// Retiring a rubric a campaign is running against would leave that campaign
// resolving to something deprecated halfway through its own hiring round.
func TestARubricInUseCannotBeRetired(t *testing.T) {
	t.Parallel()
	rubrics, _, inUse, _ := library(t)
	ctx := t.Context()

	drafted, _ := rubrics.Draft(ctx, tenant, "author", tenantadmin.RubricDraft{
		Reference: "rubric/clinical-panel", Version: "1.0.0", SchemaVersion: "1.0", Body: body(),
	})
	if _, err := rubrics.Publish(ctx, tenant, "approver", drafted.ArtifactID); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	inUse.campaigns = []string{"ICU night cover"}

	if err := rubrics.Retire(ctx, tenant, "author", drafted.ArtifactID); !errors.Is(err, tenantadmin.ErrRubricInUse) {
		t.Fatalf("Retire = %v, want ErrRubricInUse", err)
	}
}

func TestRetiringAnUnusedRubricDeprecatesItRatherThanRemovingIt(t *testing.T) {
	t.Parallel()
	rubrics, store, _, _ := library(t)
	ctx := t.Context()

	drafted, _ := rubrics.Draft(ctx, tenant, "author", tenantadmin.RubricDraft{
		Reference: "rubric/clinical-panel", Version: "1.0.0", SchemaVersion: "1.0", Body: body(),
	})
	if _, err := rubrics.Publish(ctx, tenant, "approver", drafted.ArtifactID); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := rubrics.Retire(ctx, tenant, "author", drafted.ArtifactID); err != nil {
		t.Fatalf("Retire: %v", err)
	}
	if len(store.deleted) != 0 {
		t.Error("retiring removed a published version instead of deprecating it")
	}
	if store.versions[0].Status != "deprecated" {
		t.Errorf("Status = %q, want deprecated", store.versions[0].Status)
	}
}

// A published version is not a draft, so discarding one is refused here
// before the registry's trigger has to refuse it.
func TestAPublishedRubricCannotBeDiscarded(t *testing.T) {
	t.Parallel()
	rubrics, _, _, _ := library(t)
	ctx := t.Context()

	drafted, _ := rubrics.Draft(ctx, tenant, "author", tenantadmin.RubricDraft{
		Reference: "rubric/clinical-panel", Version: "1.0.0", SchemaVersion: "1.0", Body: body(),
	})
	if _, err := rubrics.Publish(ctx, tenant, "approver", drafted.ArtifactID); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := rubrics.DiscardDraft(ctx, tenant, "author", drafted.ArtifactID); !errors.Is(err, tenantadmin.ErrRubricImmutable) {
		t.Fatalf("DiscardDraft = %v, want ErrRubricImmutable", err)
	}
}

// The platform's templates are readable by every workspace and are nobody's
// to remove, which is a different refusal from "in use" and has to say so.
func TestAWorkspaceCannotRemoveAPlatformTemplate(t *testing.T) {
	t.Parallel()
	rubrics, store, _, _ := library(t)
	store.versions = append(store.versions, tenantadmin.ArtifactVersion{
		ID: "rubric/practice-default@1.1.0", Type: tenantadmin.RubricArtifactType,
		Reference: "rubric/practice-default", Version: "1.1.0", Status: "published",
		TenantID: "", CreatedBy: "platform",
	})

	err := rubrics.Retire(t.Context(), tenant, "author", "rubric/practice-default@1.1.0")
	if !errors.Is(err, tenantadmin.ErrRubricNotOwn) {
		t.Fatalf("Retire = %v, want ErrRubricNotOwn", err)
	}
}

// The library view: the workspace's own rubrics and the platform's templates,
// with the templates marked as such rather than mixed in.
func TestTheLibraryDistinguishesTemplatesFromTheWorkspacesOwn(t *testing.T) {
	t.Parallel()
	rubrics, store, _, _ := library(t)
	ctx := t.Context()

	store.versions = append(store.versions, tenantadmin.ArtifactVersion{
		ID: "rubric/practice-default@1.1.0", Type: tenantadmin.RubricArtifactType,
		Reference: "rubric/practice-default", Version: "1.1.0", Status: "published",
	})
	if _, err := rubrics.Draft(ctx, tenant, "author", tenantadmin.RubricDraft{
		Reference: "rubric/clinical-panel", Version: "1.0.0", SchemaVersion: "1.0", Body: body(),
	}); err != nil {
		t.Fatalf("Draft: %v", err)
	}

	listed, err := rubrics.List(ctx, tenant)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("List returned %d rubrics, want 2", len(listed))
	}
	found := map[string]bool{}
	for _, entry := range listed {
		found[entry.Reference] = entry.Template
	}
	if found["rubric/clinical-panel"] {
		t.Error("the workspace's own rubric was listed as a platform template")
	}
	if !found["rubric/practice-default"] {
		t.Error("the platform's rubric was not listed as a template")
	}
}

// The lifecycle the ticket names, in order: draft, validate, approve,
// publish. Each step is its own act because each is a different person's, and
// the registry's separation of duties rests on the last two being separable.
func TestARubricWalksDraftValidatingApprovedPublished(t *testing.T) {
	t.Parallel()
	rubrics, store, _, _ := library(t)
	ctx := t.Context()

	drafted, err := rubrics.Draft(ctx, tenant, "author", tenantadmin.RubricDraft{
		Reference: "rubric/clinical-panel", Version: "1.0.0", SchemaVersion: "1.0", Body: body(),
	})
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	submitted, err := rubrics.SubmitForApproval(ctx, tenant, drafted.ArtifactID)
	if err != nil {
		t.Fatalf("SubmitForApproval: %v", err)
	}
	if submitted.Status != "validating" {
		t.Errorf("Status = %q, want validating", submitted.Status)
	}
	approved, err := rubrics.Approve(ctx, tenant, drafted.ArtifactID)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if approved.Status != "approved" {
		t.Errorf("Status = %q, want approved", approved.Status)
	}
	published, err := rubrics.Publish(ctx, tenant, "approver", drafted.ArtifactID)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if published.Status != "published" || published.PublishedBy != "approver" {
		t.Errorf("published = %+v, want published by the approver", published)
	}
	if len(store.moved) != 2 {
		t.Errorf("the registry was moved %d times before publication, want 2", len(store.moved))
	}
}

// The refusal is read by a person, so it has to say what to go and look at.
func TestTheInUseRefusalNamesTheCampaignsAndTheRubric(t *testing.T) {
	t.Parallel()
	blocked := &tenantadmin.RubricInUseError{
		Reference: "rubric/clinical-panel",
		Campaigns: []string{"ICU night cover", "ICU day cover"},
	}
	message := blocked.Error()
	for _, want := range []string{"rubric/clinical-panel", "ICU night cover", "ICU day cover"} {
		if !strings.Contains(message, want) {
			t.Errorf("the refusal does not mention %q: %s", want, message)
		}
	}
}

// A workspace may not publish the platform catalogue's rubric either, which
// is the same ownership question asked of a different verb.
func TestAWorkspaceCannotPublishAPlatformTemplate(t *testing.T) {
	t.Parallel()
	rubrics, store, _, _ := library(t)
	store.versions = append(store.versions, tenantadmin.ArtifactVersion{
		ID: "rubric/practice-default@1.1.0", Type: tenantadmin.RubricArtifactType,
		Reference: "rubric/practice-default", Version: "1.1.0", Status: "approved",
	})

	_, err := rubrics.Publish(t.Context(), tenant, "approver", "rubric/practice-default@1.1.0")
	if !errors.Is(err, tenantadmin.ErrRubricNotOwn) {
		t.Fatalf("Publish = %v, want ErrRubricNotOwn", err)
	}
}

func TestActingOnARubricThatDoesNotExistIsRefused(t *testing.T) {
	t.Parallel()
	rubrics, _, _, _ := library(t)

	if _, err := rubrics.SubmitForApproval(t.Context(), tenant, "rubric/nothing@1.0.0"); !errors.Is(
		err, tenantadmin.ErrRubricNotFound) {
		t.Fatalf("SubmitForApproval = %v, want ErrRubricNotFound", err)
	}
}
