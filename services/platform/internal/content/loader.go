package content

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// The git-authored loader: ADR-0011's publishing tool, not a runtime
// dependency. It reads artifact files - reviewed in git, which is the
// authoring path for platform content - and walks each through the
// registry's own lifecycle to published. At runtime nothing reads these
// files; the registry row the loader produced is the only source anything
// resolves.
//
// The loader is idempotent the honest way: a version already published with
// the same digest is a no-op, a version already published with a DIFFERENT
// digest is a refusal, because an edited file wearing an old version number
// is exactly the in-place mutation the registry exists to prevent.

// Envelope is one artifact file: the registry coordinates and the body.
type Envelope struct {
	Type          string          `json:"type"`
	Reference     string          `json:"reference"`
	Version       string          `json:"version"`
	SchemaVersion string          `json:"schema_version"`
	Body          json.RawMessage `json:"body"`
}

// Validator checks one artifact type's body before it can leave draft. The
// map is supplied by cmd, because the checks belong to the contexts that
// read each type and this context must not import them.
type Validator func(body json.RawMessage) error

// ErrArtifactMutated means a file changed under a version that is already
// published. The fix is a new version, never an edit.
var ErrArtifactMutated = errors.New("content: a published version's file changed; publish a new version instead")

// LoadOutcome says what the loader did with one file.
type LoadOutcome struct {
	Reference string
	Version   string
	// Action is published, unchanged.
	Action string
}

// Loader publishes git-authored artifacts into the registry.
type Loader struct {
	store      *Store
	validators map[string]Validator
	// The two principals: git review already approved the content, but the
	// registry's separation of duties still holds structurally, so the
	// drafting identity and the publishing identity are distinct service
	// principals rather than one account wearing two hats invisibly.
	authorID    string
	publisherID string
}

// NewLoader wires a loader.
func NewLoader(store *Store, validators map[string]Validator, authorID, publisherID string) *Loader {
	return &Loader{store: store, validators: validators, authorID: authorID, publisherID: publisherID}
}

// LoadDirectory publishes every artifact file under dir, in path order.
//
// Files are *.json; anything else is ignored. The walk is sorted so two runs
// see the same order and a failure names a deterministic place.
func (l *Loader) LoadDirectory(ctx context.Context, dir fs.FS) ([]LoadOutcome, error) {
	var files []string
	err := fs.WalkDir(dir, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".json") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("content: walking artifacts: %w", err)
	}
	sort.Strings(files)

	outcomes := make([]LoadOutcome, 0, len(files))
	for _, path := range files {
		raw, err := fs.ReadFile(dir, path)
		if err != nil {
			return outcomes, fmt.Errorf("content: reading %s: %w", path, err)
		}
		outcome, err := l.load(ctx, raw)
		if err != nil {
			return outcomes, fmt.Errorf("content: %s: %w", filepath.ToSlash(path), err)
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, nil
}

// load publishes one envelope, idempotently.
func (l *Loader) load(ctx context.Context, raw []byte) (LoadOutcome, error) {
	var envelope Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return LoadOutcome{}, fmt.Errorf("the envelope does not decode: %w", err)
	}
	if envelope.Type == "" || envelope.Reference == "" || envelope.Version == "" || envelope.SchemaVersion == "" {
		return LoadOutcome{}, errors.New("the envelope is missing type, reference, version or schema_version")
	}

	// The validating state's work, before anything is drafted: a file that
	// does not satisfy its type's reader never enters the registry at all.
	if validate, known := l.validators[envelope.Type]; known {
		if err := validate(envelope.Body); err != nil {
			return LoadOutcome{}, fmt.Errorf("validation refused the body: %w", err)
		}
	}

	digest, err := DigestOf(envelope.Body)
	if err != nil {
		return LoadOutcome{}, err
	}

	current, err := l.store.Resolve(ctx, envelope.Reference, "")
	switch {
	case err == nil && current.Version == envelope.Version:
		if current.Digest != digest {
			return LoadOutcome{}, ErrArtifactMutated
		}
		return LoadOutcome{Reference: envelope.Reference, Version: envelope.Version, Action: "unchanged"}, nil
	case err != nil && !errors.Is(err, ErrNotFound):
		return LoadOutcome{}, err
	}

	draft, err := l.store.CreateDraft(ctx, Draft{
		Type: envelope.Type, Reference: envelope.Reference, Version: envelope.Version,
		SchemaVersion: envelope.SchemaVersion, Body: envelope.Body, CreatedBy: l.authorID,
	})
	if err != nil {
		return LoadOutcome{}, err
	}
	step := draft
	for _, to := range []Status{StatusValidating, StatusApproved} {
		if step, err = l.store.Transition(ctx, step, to); err != nil {
			return LoadOutcome{}, err
		}
	}
	if _, err := l.store.Publish(ctx, step, l.publisherID); err != nil {
		return LoadOutcome{}, err
	}
	return LoadOutcome{Reference: envelope.Reference, Version: envelope.Version, Action: "published"}, nil
}
