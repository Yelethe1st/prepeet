//go:build integration

package content_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/content"
)

// The loader against the real registry: git authors, the loader publishes,
// and the registry's guarantees hold against it like against anybody else.

func loader(t *testing.T, validators map[string]content.Validator) *content.Loader {
	t.Helper()
	// The loader's principals are real accounts - the audit trail's foreign
	// key insists, correctly, that an actor exists - so the harness's seeded
	// author and reviewer serve as the drafting and publishing services.
	return content.NewLoader(content.NewStore(pool), validators, authorID, reviewerID)
}

// envelopeFile writes one artifact file into dir.
func envelopeFile(t *testing.T, dir, name string, envelope content.Envelope) {
	t.Helper()
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
		t.Fatalf("writing: %v", err)
	}
}

func TestTheShippedArtifactsPublishAndResolve(t *testing.T) {
	// The real directory, the real files: what `contentctl publish` runs in
	// every environment. Read across the module boundary, hence -count=1.
	ctx := context.Background()
	load := loader(t, nil)

	outcomes, err := load.LoadDirectory(ctx, os.DirFS("../../../intelligence/artifacts"))
	if err != nil {
		t.Fatalf("loading the shipped artifacts: %v", err)
	}
	if len(outcomes) == 0 {
		t.Fatal("the shipped directory published nothing")
	}

	resolved, err := content.NewStore(pool).Resolve(ctx, "catalog", "")
	if err != nil {
		t.Fatalf("the catalogue did not resolve after loading: %v", err)
	}
	if resolved.Type != "catalogue" || resolved.Version == "" {
		t.Fatalf("resolved = %+v", resolved)
	}

	// Idempotence: the second run publishes nothing and changes nothing.
	again, err := load.LoadDirectory(ctx, os.DirFS("../../../intelligence/artifacts"))
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	for _, outcome := range again {
		if outcome.Action != "unchanged" {
			t.Fatalf("the second run did %s to %s@%s", outcome.Action, outcome.Reference, outcome.Version)
		}
	}
}

func TestAnEditedFileUnderAPublishedVersionIsRefused(t *testing.T) {
	// The in-place mutation the registry exists to prevent, attempted through
	// the loader's own front door.
	ctx := context.Background()
	dir := t.TempDir()
	load := loader(t, nil)

	envelopeFile(t, dir, "thing@1.json", content.Envelope{
		Type: "plan", Reference: "loader_thing", Version: "1.0.0",
		SchemaVersion: "1.0", Body: json.RawMessage(`{"stages":["a"]}`),
	})
	if _, err := load.LoadDirectory(ctx, os.DirFS(dir)); err != nil {
		t.Fatalf("first publish: %v", err)
	}

	envelopeFile(t, dir, "thing@1.json", content.Envelope{
		Type: "plan", Reference: "loader_thing", Version: "1.0.0",
		SchemaVersion: "1.0", Body: json.RawMessage(`{"stages":["a","sneaky"]}`),
	})
	if _, err := load.LoadDirectory(ctx, os.DirFS(dir)); !errors.Is(err, content.ErrArtifactMutated) {
		t.Fatalf("an edited published version = %v, want ErrArtifactMutated", err)
	}

	// A new version is the legitimate path, and it moves the pointer.
	envelopeFile(t, dir, "thing@2.json", content.Envelope{
		Type: "plan", Reference: "loader_thing", Version: "1.1.0",
		SchemaVersion: "1.0", Body: json.RawMessage(`{"stages":["a","b"]}`),
	})
	envelopeFile(t, dir, "thing@1.json", content.Envelope{
		Type: "plan", Reference: "loader_thing", Version: "1.0.0",
		SchemaVersion: "1.0", Body: json.RawMessage(`{"stages":["a"]}`),
	})
	if _, err := load.LoadDirectory(ctx, os.DirFS(dir)); err != nil {
		t.Fatalf("publishing the new version: %v", err)
	}
	resolved, err := content.NewStore(pool).Resolve(ctx, "loader_thing", "")
	if err != nil || resolved.Version != "1.1.0" {
		t.Fatalf("resolve after new version = %+v, %v", resolved, err)
	}
}

func TestAValidatorRefusesABodyBeforeItEntersTheRegistry(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	load := loader(t, map[string]content.Validator{
		"strict": func(json.RawMessage) error { return errors.New("no") },
	})

	envelopeFile(t, dir, "bad@1.json", content.Envelope{
		Type: "strict", Reference: "loader_bad", Version: "1.0.0",
		SchemaVersion: "1.0", Body: json.RawMessage(`{}`),
	})
	if _, err := load.LoadDirectory(ctx, os.DirFS(dir)); err == nil {
		t.Fatal("the refused body was loaded anyway")
	}
	if _, err := content.NewStore(pool).Resolve(ctx, "loader_bad", ""); !errors.Is(err, content.ErrNotFound) {
		t.Fatal("the refused body reached the registry")
	}
}
