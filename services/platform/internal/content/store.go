package content

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/content/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// The registry's durable half. What lives here rather than in SQL: the digest
// computation, the transaction shapes, and the two refusals the schema cannot
// express - the separation of duties, and rollback's insistence that its
// target was once published.

// Stable refusals. Callers branch on these; each is a rule with a name.
var (
	// ErrNotFound covers absence and another tenant's artifact alike.
	ErrNotFound = errors.New("content: ARTIFACT_NOT_FOUND: no such artifact")
	// ErrStaleStatus means the lifecycle moved after the caller looked.
	ErrStaleStatus = errors.New("content: ARTIFACT_STATUS_STALE: the artifact changed after it was read")
	// ErrSelfPublish is ADR-0011's separation of duties: the registry refuses
	// a publish whose actor drafted the version, so one person can never ship
	// their own artifact whatever capabilities they hold.
	ErrSelfPublish = errors.New("content: ARTIFACT_SELF_PUBLISH: the publisher must not be the drafter")
	// ErrNotPublished refuses pointing the catalogue at anything that never
	// passed publication: rollback replays the past, it does not invent one.
	ErrNotPublished = errors.New("content: ARTIFACT_NOT_PUBLISHED: that version was never published")
	// ErrNotDraft refuses removing anything past validation. History is the
	// audit answer, and a version that can be deleted cannot give it.
	ErrNotDraft = errors.New("content: ARTIFACT_NOT_DRAFT: only a draft may be deleted")
	// ErrDigestMismatch means the stored body no longer hashes to the stored
	// digest, which is corruption and never business as usual.
	ErrDigestMismatch = errors.New("content: ARTIFACT_DIGEST_MISMATCH: the body does not match its digest")
)

// Artifact is one version of one reference.
type Artifact struct {
	ID            string
	Type          string
	Reference     string
	Version       string
	SchemaVersion string
	Digest        string
	Body          json.RawMessage
	Status        Status
	// TenantID is empty for platform artifacts, which every tenant reads.
	TenantID    string
	CreatedBy   string
	CreatedAt   time.Time
	PublishedBy string
	PublishedAt *time.Time
}

// Draft is what drafting supplies; everything else the store derives.
type Draft struct {
	Type          string
	Reference     string
	Version       string
	SchemaVersion string
	Body          json.RawMessage
	TenantID      string
	CreatedBy     string
}

// Store persists artifacts.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore builds the store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// DigestOf computes the canonical digest of a body.
//
// Canonical means the JSON is re-encoded with sorted keys before hashing, so
// two bodies that mean the same thing carry the same digest whatever their
// whitespace or key order was when authored. The digest is the identity
// sessions pin; if this function changes, every digest changes, which is why
// it must not.
func DigestOf(body json.RawMessage) (string, error) {
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", fmt.Errorf("content: the body is not valid JSON: %w", err)
	}
	canonical, err := json.Marshal(decoded)
	if err != nil {
		return "", fmt.Errorf("content: canonicalising the body: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// scope sets tenant context for tenant-scoped artifacts; platform artifacts
// need none, and the policy admits their NULL-tenant rows under any scope.
func (s *Store) scope(ctx context.Context, tx pgx.Tx, tenantID string) error {
	if tenantID == "" {
		return nil
	}
	return database.SetTenant(ctx, tx, tenantID)
}

// actingScope is scope plus the actor, for the operations that write audit
// rows: an untenanted audit insert is policy-bound to the acting user, so a
// platform-catalogue publish acts as its publisher the way a practice
// transition acts as its candidate.
func (s *Store) actingScope(ctx context.Context, tx pgx.Tx, tenantID, actorID string) error {
	if err := s.scope(ctx, tx, tenantID); err != nil {
		return err
	}
	if tenantID == "" {
		return database.SetUser(ctx, tx, actorID)
	}
	return nil
}

// CreateDraft writes a new draft version with its computed digest.
func (s *Store) CreateDraft(ctx context.Context, draft Draft) (Artifact, error) {
	digest, err := DigestOf(draft.Body)
	if err != nil {
		return Artifact{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Artifact{}, fmt.Errorf("content: beginning draft: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.scope(ctx, tx, draft.TenantID); err != nil {
		return Artifact{}, err
	}

	artifactID := id.New().String()
	if err := db.New(tx).InsertArtifact(ctx, db.InsertArtifactParams{
		ID:            artifactID,
		ArtifactType:  draft.Type,
		Reference:     draft.Reference,
		Version:       draft.Version,
		SchemaVersion: draft.SchemaVersion,
		Digest:        digest,
		Body:          draft.Body,
		TenantID:      draft.TenantID,
		CreatedBy:     draft.CreatedBy,
	}); err != nil {
		return Artifact{}, fmt.Errorf("content: inserting draft: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Artifact{}, err
	}
	return s.Get(ctx, artifactID, draft.TenantID)
}

// Get reads one version by id.
func (s *Store) Get(ctx context.Context, artifactID, tenantID string) (Artifact, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Artifact{}, fmt.Errorf("content: beginning read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.scope(ctx, tx, tenantID); err != nil {
		return Artifact{}, err
	}
	row, err := db.New(tx).GetArtifact(ctx, artifactID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Artifact{}, ErrNotFound
	}
	if err != nil {
		return Artifact{}, fmt.Errorf("content: reading artifact: %w", err)
	}
	return fromRow(artifactRow(row)), nil
}

// GetByDigest reads whatever a pinned digest names.
//
// This is the read path for anything that already holds a pin, and its
// integrity is checked on every read: a body that no longer hashes to its
// digest is corruption, and serving it as the pinned content would put words
// into a historical session's record.
func (s *Store) GetByDigest(ctx context.Context, digest, tenantID string) (Artifact, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Artifact{}, fmt.Errorf("content: beginning read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.scope(ctx, tx, tenantID); err != nil {
		return Artifact{}, err
	}
	row, err := db.New(tx).GetArtifactByDigest(ctx, digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return Artifact{}, ErrNotFound
	}
	if err != nil {
		return Artifact{}, fmt.Errorf("content: reading by digest: %w", err)
	}
	artifact := fromRow(artifactRow(row))

	recomputed, err := DigestOf(artifact.Body)
	if err != nil {
		return Artifact{}, err
	}
	if recomputed != artifact.Digest {
		return Artifact{}, ErrDigestMismatch
	}
	return artifact, nil
}

// Transition moves a version along the lifecycle, without publication's extra
// duties; publishing goes through Publish.
func (s *Store) Transition(ctx context.Context, artifact Artifact, to Status) (Artifact, error) {
	if to == StatusPublished {
		return Artifact{}, errors.New("content: publication goes through Publish, which enforces its duties")
	}
	if err := CanTransition(artifact.Status, to); err != nil {
		return Artifact{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Artifact{}, fmt.Errorf("content: beginning transition: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.scope(ctx, tx, artifact.TenantID); err != nil {
		return Artifact{}, err
	}

	moved, err := db.New(tx).TransitionArtifact(ctx, db.TransitionArtifactParams{
		ID: artifact.ID, FromStatus: string(artifact.Status), ToStatus: string(to),
	})
	if err != nil {
		return Artifact{}, fmt.Errorf("content: transitioning: %w", err)
	}
	if moved == 0 {
		return Artifact{}, ErrStaleStatus
	}
	if err := tx.Commit(ctx); err != nil {
		return Artifact{}, err
	}
	return s.Get(ctx, artifact.ID, artifact.TenantID)
}

// Publish moves an approved version to published, repoints the reference, and
// writes the audit row, in one transaction.
//
// The separation of duties lives here and nowhere softer: whatever
// capabilities the actor holds, the registry refuses a publish whose actor
// drafted the version. Two people, structurally.
func (s *Store) Publish(ctx context.Context, artifact Artifact, publisherID string) (Artifact, error) {
	if publisherID == artifact.CreatedBy {
		return Artifact{}, ErrSelfPublish
	}
	if err := CanTransition(artifact.Status, StatusPublished); err != nil {
		return Artifact{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Artifact{}, fmt.Errorf("content: beginning publish: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.actingScope(ctx, tx, artifact.TenantID, publisherID); err != nil {
		return Artifact{}, err
	}

	q := db.New(tx)
	moved, err := q.MarkPublished(ctx, db.MarkPublishedParams{
		ID: artifact.ID, PublishedBy: publisherID,
	})
	if err != nil {
		return Artifact{}, fmt.Errorf("content: publishing: %w", err)
	}
	if moved == 0 {
		return Artifact{}, ErrStaleStatus
	}

	if err := q.UpsertPointer(ctx, db.UpsertPointerParams{
		Reference: artifact.Reference, TenantID: artifact.TenantID,
		ArtifactID: artifact.ID, UpdatedBy: publisherID,
	}); err != nil {
		return Artifact{}, fmt.Errorf("content: pointing at the publication: %w", err)
	}

	if err := q.InsertAuditEvent(ctx, db.InsertAuditEventParams{
		ID: id.New().String(), TenantID: artifact.TenantID, ActorID: publisherID,
		Action: "content.artifact_published", SubjectID: artifact.ID, Outcome: "allowed",
	}); err != nil {
		return Artifact{}, fmt.Errorf("content: auditing the publication: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Artifact{}, err
	}
	return s.Get(ctx, artifact.ID, artifact.TenantID)
}

// Resolve returns what a reference currently points at.
//
// For compositions choosing what to pin next; anything already holding a
// digest reads by GetByDigest and is untouched by where this points.
func (s *Store) Resolve(ctx context.Context, reference, tenantID string) (Artifact, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Artifact{}, fmt.Errorf("content: beginning resolve: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.scope(ctx, tx, tenantID); err != nil {
		return Artifact{}, err
	}
	row, err := db.New(tx).ResolvePointer(ctx, db.ResolvePointerParams{
		Reference: reference, TenantID: tenantID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Artifact{}, ErrNotFound
	}
	if err != nil {
		return Artifact{}, fmt.Errorf("content: resolving %q: %w", reference, err)
	}
	return fromRow(artifactRow(row)), nil
}

// Rollback repoints a reference to an earlier published version and marks the
// currently pointed version deprecated, in one transaction.
//
// It replays the past rather than inventing one: the target must have been
// published, and nothing is deleted or edited on the way.
func (s *Store) Rollback(ctx context.Context, reference, toVersion, tenantID, actorID string) (Artifact, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Artifact{}, fmt.Errorf("content: beginning rollback: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.actingScope(ctx, tx, tenantID, actorID); err != nil {
		return Artifact{}, err
	}
	q := db.New(tx)

	target, err := q.GetArtifactByVersion(ctx, db.GetArtifactByVersionParams{
		Reference: reference, Version: toVersion, TenantID: tenantID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Artifact{}, ErrNotFound
	}
	if err != nil {
		return Artifact{}, fmt.Errorf("content: reading the rollback target: %w", err)
	}
	// Published or since deprecated: both passed publication once, which is
	// the property rollback requires. A draft or approved version did not.
	if target.Status != string(StatusPublished) && target.Status != string(StatusDeprecated) {
		return Artifact{}, ErrNotPublished
	}

	// The version being rolled away from is deprecated, visibly: an operator
	// asking "what went wrong this week" finds it, standing and dated.
	current, err := q.ResolvePointer(ctx, db.ResolvePointerParams{
		Reference: reference, TenantID: tenantID,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Artifact{}, fmt.Errorf("content: reading the current pointer: %w", err)
	}
	if err == nil && current.ID != target.ID && current.Status == string(StatusPublished) {
		if _, err := q.TransitionArtifact(ctx, db.TransitionArtifactParams{
			ID: current.ID, FromStatus: string(StatusPublished), ToStatus: string(StatusDeprecated),
		}); err != nil {
			return Artifact{}, fmt.Errorf("content: deprecating the rolled-back version: %w", err)
		}
	}

	if err := q.UpsertPointer(ctx, db.UpsertPointerParams{
		Reference: reference, TenantID: tenantID,
		ArtifactID: target.ID, UpdatedBy: actorID,
	}); err != nil {
		return Artifact{}, fmt.Errorf("content: repointing: %w", err)
	}

	if err := q.InsertAuditEvent(ctx, db.InsertAuditEventParams{
		ID: id.New().String(), TenantID: tenantID, ActorID: actorID,
		Action: "content.artifact_rolled_back", SubjectID: target.ID, Outcome: "allowed",
	}); err != nil {
		return Artifact{}, fmt.Errorf("content: auditing the rollback: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Artifact{}, err
	}
	return s.Get(ctx, target.ID, tenantID)
}

// artifactRow is the shared shape of every generated artifact read.
type artifactRow struct {
	ID            string
	ArtifactType  string
	Reference     string
	Version       string
	SchemaVersion string
	Digest        string
	Body          []byte
	Status        string
	TenantID      string
	CreatedBy     string
	CreatedAt     time.Time
	PublishedBy   string
	PublishedAt   *time.Time
}

func fromRow(row artifactRow) Artifact {
	return Artifact{
		ID: row.ID, Type: row.ArtifactType, Reference: row.Reference,
		Version: row.Version, SchemaVersion: row.SchemaVersion,
		Digest: row.Digest, Body: row.Body, Status: Status(row.Status),
		TenantID: row.TenantID, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt,
		PublishedBy: row.PublishedBy, PublishedAt: row.PublishedAt,
	}
}

// Versions returns every version of one reference, newest first.
//
// The registry is already the answer to "what versions of this exist, who
// published each and when", so a surface over it - TEN-04's rubric library -
// reads this rather than keeping its own history table. Two answers to what a
// version is would be one too many, and the second would be the one that
// drifts.
func (s *Store) Versions(ctx context.Context, reference, tenantID string) ([]Artifact, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("content: beginning version read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.scope(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	rows, err := db.New(tx).ListArtifactVersions(ctx, db.ListArtifactVersionsParams{
		Reference: reference, TenantID: tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("content: listing versions of %q: %w", reference, err)
	}
	versions := make([]Artifact, 0, len(rows))
	for _, row := range rows {
		versions = append(versions, fromRow(artifactRow(row)))
	}
	return versions, nil
}

// ListByType returns every artifact of one type the caller may see.
//
// Which is the tenant's own and the platform catalogue's, because that is
// what the visibility policy admits and this adds no filter of its own: a
// library that hid the platform's templates would be hiding them from the
// only people who can build on them.
func (s *Store) ListByType(ctx context.Context, artifactType, tenantID string) ([]Artifact, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("content: beginning type read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.scope(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	rows, err := db.New(tx).ListArtifactsByType(ctx, artifactType)
	if err != nil {
		return nil, fmt.Errorf("content: listing %q artifacts: %w", artifactType, err)
	}
	artifacts := make([]Artifact, 0, len(rows))
	for _, row := range rows {
		artifacts = append(artifacts, fromRow(artifactRow(row)))
	}
	return artifacts, nil
}

// DeleteDraft removes a draft, and only a draft.
//
// Everything past validation is history and stays: a version somebody could
// remove is a version that cannot answer "what was this session judged by".
// A non-draft is refused as ErrNotDraft rather than as a trigger exception,
// so a surface can tell the author why without reading a database message.
func (s *Store) DeleteDraft(ctx context.Context, artifactID, tenantID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("content: beginning delete: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.scope(ctx, tx, tenantID); err != nil {
		return err
	}

	removed, err := db.New(tx).DeleteDraftArtifact(ctx, artifactID)
	if err != nil {
		return fmt.Errorf("content: deleting draft: %w", err)
	}
	if removed == 0 {
		// Absent, another tenant's, or past validation. The first two are
		// indistinguishable by design; the third is the one worth naming, and
		// a caller separates them by having read the artifact first.
		return ErrNotDraft
	}
	return tx.Commit(ctx)
}
