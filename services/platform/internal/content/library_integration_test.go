//go:build integration

package content_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/content"
)

// The three reads TEN-04's rubric library is built on, against real
// PostgreSQL.
//
// They live here rather than in the library because the registry owns
// content.artifacts and is the only thing that may name it. What the library
// adds is tested against a fake registry in its own package; what a version
// IS is tested here, once.

// tenantDraft writes a draft owned by one workspace.
func tenantDraft(t *testing.T, tenantID, reference, version string) content.Artifact {
	t.Helper()
	artifact, err := content.NewStore(pool).CreateDraft(context.Background(), content.Draft{
		Type:          "rubric",
		Reference:     reference,
		Version:       version,
		SchemaVersion: "1.0",
		Body:          json.RawMessage(`{"sufficiency":{"min_supporting":2}}`),
		TenantID:      tenantID,
		CreatedBy:     authorID,
	})
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	return artifact
}

// Version history, with who published what and when: TEN-04's second box,
// answered by the registry rather than by a table beside it.
func TestVersionsReturnEveryVersionWithItsPublicationProvenance(t *testing.T) {
	ctx := context.Background()
	store := content.NewStore(pool)
	name := "rubric/" + t.Name()

	first := tenantDraft(t, tenantAlpha, name, "1.0.0")
	validating, err := store.Transition(ctx, first, content.StatusValidating)
	if err != nil {
		t.Fatalf("to validating: %v", err)
	}
	approved, err := store.Transition(ctx, validating, content.StatusApproved)
	if err != nil {
		t.Fatalf("to approved: %v", err)
	}
	published, err := store.Publish(ctx, approved, reviewerID)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	tenantDraft(t, tenantAlpha, name, "1.1.0")

	versions, err := store.Versions(ctx, name, tenantAlpha)
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("Versions returned %d, want 2", len(versions))
	}
	var seen content.Artifact
	for _, version := range versions {
		if version.Version == "1.0.0" {
			seen = version
		}
	}
	if seen.PublishedBy != reviewerID {
		t.Errorf("PublishedBy = %q, want the publisher", seen.PublishedBy)
	}
	if seen.PublishedAt == nil {
		t.Error("a published version carries no publication time")
	}
	if seen.CreatedBy != authorID {
		t.Errorf("CreatedBy = %q, want the drafter", seen.CreatedBy)
	}
	if seen.Digest != published.Digest {
		t.Error("the history's digest differs from the published one")
	}
}

// The attack that means something: a version that genuinely exists under
// another workspace, asked for by its own reference from inside this one.
func TestVersionsNeverCrossATenantBoundary(t *testing.T) {
	ctx := context.Background()
	store := content.NewStore(pool)
	name := "rubric/" + t.Name()

	tenantDraft(t, tenantBeta, name, "1.0.0")
	if theirs, err := store.Versions(ctx, name, tenantBeta); err != nil || len(theirs) != 1 {
		t.Fatalf("the row under attack must exist: %d versions, err %v", len(theirs), err)
	}

	stolen, err := store.Versions(ctx, name, tenantAlpha)
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	if len(stolen) != 0 {
		t.Errorf("one workspace read %d of another's rubric versions", len(stolen))
	}
}

// The library view: a workspace's own artifacts and the platform catalogue's,
// and nobody else's.
func TestListByTypeReturnsTheWorkspacesOwnAndThePlatformsOnly(t *testing.T) {
	ctx := context.Background()
	store := content.NewStore(pool)

	mine := tenantDraft(t, tenantAlpha, "rubric/"+t.Name()+"-mine", "1.0.0")
	theirs := tenantDraft(t, tenantBeta, "rubric/"+t.Name()+"-theirs", "1.0.0")
	platform, err := store.CreateDraft(ctx, content.Draft{
		Type: "rubric", Reference: "rubric/" + t.Name() + "-platform", Version: "1.0.0",
		SchemaVersion: "1.0", Body: json.RawMessage(`{"sufficiency":{"min_supporting":2}}`),
		CreatedBy: authorID,
	})
	if err != nil {
		t.Fatalf("platform draft: %v", err)
	}

	listed, err := store.ListByType(ctx, "rubric", tenantAlpha)
	if err != nil {
		t.Fatalf("ListByType: %v", err)
	}
	seen := map[string]bool{}
	for _, artifact := range listed {
		seen[artifact.ID] = true
	}
	if !seen[mine.ID] {
		t.Error("the workspace's own rubric is missing from its library")
	}
	if !seen[platform.ID] {
		t.Error("the platform catalogue's rubric is missing from the library")
	}
	if seen[theirs.ID] {
		t.Error("another workspace's rubric appeared in this one's library")
	}
}

// Drafts go; everything past validation stays. This is the registry's rule
// and TEN-04's first box rests on it: a published rubric is not removed, it
// is superseded.
func TestOnlyADraftMayBeDeleted(t *testing.T) {
	ctx := context.Background()
	store := content.NewStore(pool)
	name := "rubric/" + t.Name()

	draft := tenantDraft(t, tenantAlpha, name, "1.0.0")
	if err := store.DeleteDraft(ctx, draft.ID, tenantAlpha); err != nil {
		t.Fatalf("DeleteDraft: %v", err)
	}
	if _, err := store.Get(ctx, draft.ID, tenantAlpha); !errors.Is(err, content.ErrNotFound) {
		t.Errorf("the draft survived deletion: %v", err)
	}

	second := tenantDraft(t, tenantAlpha, name, "2.0.0")
	validating, err := store.Transition(ctx, second, content.StatusValidating)
	if err != nil {
		t.Fatalf("to validating: %v", err)
	}
	approved, err := store.Transition(ctx, validating, content.StatusApproved)
	if err != nil {
		t.Fatalf("to approved: %v", err)
	}
	published, err := store.Publish(ctx, approved, reviewerID)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := store.DeleteDraft(ctx, published.ID, tenantAlpha); !errors.Is(err, content.ErrNotDraft) {
		t.Fatalf("DeleteDraft on a published version = %v, want ErrNotDraft", err)
	}
	if _, err := store.Get(ctx, published.ID, tenantAlpha); err != nil {
		t.Errorf("the published version is gone after a refused delete: %v", err)
	}
}

// And one workspace cannot delete another's draft, which the row count
// reports as "not a draft" because absence and another tenant's rows are
// deliberately indistinguishable here.
func TestOneWorkspaceCannotDeleteAnothersDraft(t *testing.T) {
	ctx := context.Background()
	store := content.NewStore(pool)

	victim := tenantDraft(t, tenantBeta, "rubric/"+t.Name(), "1.0.0")

	if err := store.DeleteDraft(ctx, victim.ID, tenantAlpha); !errors.Is(err, content.ErrNotDraft) {
		t.Fatalf("cross-tenant DeleteDraft = %v, want a refusal", err)
	}
	if _, err := store.Get(ctx, victim.ID, tenantBeta); err != nil {
		t.Errorf("the victim's draft is gone: %v", err)
	}
}
