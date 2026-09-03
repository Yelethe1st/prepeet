package recruiting

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/recruiting/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// Requirement is one requirement as it stands: extraction's until a recruiter
// corrects it, span-linked to the job context it came from.
type Requirement struct {
	ID                string
	CampaignID        string
	Text              string
	SpanStart         int
	SpanEnd           int
	Status            string
	ExtractionVersion string
}

// Requirement statuses a caller distinguishes.
const (
	RequirementProposed  = "proposed"
	RequirementCorrected = "corrected"
	RequirementRejected  = "rejected"
)

// ErrRequirementNotFound means a correction named a requirement that is not on
// the campaign, collapsed with somebody else's so an id cannot be probed.
var ErrRequirementNotFound = errors.New("recruiting: no such requirement on this campaign")

// ErrRequirementsFrozen means the campaign has opened, so its requirements can
// no longer change. The freeze trigger is the database backstop; this is the
// clean refusal the store returns before reaching for it.
var ErrRequirementsFrozen = errors.New("recruiting: the campaign's requirements are frozen")

// mustBeDraft returns ErrRequirementsFrozen unless the campaign is still a
// draft, so a mutation refuses with a clean error rather than the trigger's
// exception. The trigger remains the backstop for any path that skips this.
func mustBeDraft(ctx context.Context, q *db.Queries, campaignID string) error {
	row, err := q.CampaignByID(ctx, campaignID)
	if err != nil {
		return fmt.Errorf("recruiting: reading the campaign status: %w", err)
	}
	if row.Status != string(StatusDraft) {
		return ErrRequirementsFrozen
	}
	return nil
}

// SubmitJobContext stores a campaign's job description and replaces its
// requirements with a fresh extraction, all in one transaction.
//
// Wholesale replacement is the honest model for a resubmission: the spans of
// the old requirements were measured in the old text, so keeping them beside a
// new source would leave provenance pointing at bytes that no longer exist.
// The freeze trigger refuses this once the campaign has opened, so a running
// campaign's requirements cannot be re-extracted out from under it.
func (s *Store) SubmitJobContext(ctx context.Context, tenantID, campaignID, sourceText string, extractor RequirementExtractor) ([]Requirement, error) {
	version, extracted := extractor.Extract(sourceText)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("recruiting: beginning job context submit: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	queries := db.New(tx)
	if err := mustBeDraft(ctx, queries, campaignID); err != nil {
		return nil, err
	}

	if err := queries.UpsertJobContext(ctx, db.UpsertJobContextParams{
		CampaignID: campaignID, TenantID: tenantID, SourceText: sourceText, ExtractionVersion: version,
	}); err != nil {
		return nil, fmt.Errorf("recruiting: storing the job context: %w", err)
	}
	if err := queries.DeleteRequirementsForCampaign(ctx, campaignID); err != nil {
		return nil, fmt.Errorf("recruiting: clearing prior requirements: %w", err)
	}

	requirements := make([]Requirement, 0, len(extracted))
	for _, candidate := range extracted {
		row, err := queries.InsertRequirement(ctx, db.InsertRequirementParams{
			ID: id.New().String(), CampaignID: campaignID, TenantID: tenantID,
			Text: candidate.Text, SpanStart: int32(candidate.SpanStart), SpanEnd: int32(candidate.SpanEnd),
			ExtractionVersion: version,
		})
		if err != nil {
			return nil, fmt.Errorf("recruiting: storing a requirement: %w", err)
		}
		requirements = append(requirements, requirementFromInsert(row))
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("recruiting: committing the job context: %w", err)
	}
	return requirements, nil
}

// RequirementsForCampaign lists a campaign's requirements for review. Tenant
// scoping is the policy's; the per-campaign access check is the handler's.
func (s *Store) RequirementsForCampaign(ctx context.Context, tenantID, campaignID string) ([]Requirement, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("recruiting: beginning requirements read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	rows, err := db.New(tx).RequirementsForCampaign(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("recruiting: listing requirements: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("recruiting: committing the requirements read: %w", err)
	}
	out := make([]Requirement, 0, len(rows))
	for _, row := range rows {
		out = append(out, requirementFromList(row))
	}
	return out, nil
}

// CorrectRequirement changes a requirement's text or rejects it, on a draft
// campaign. The span is untouched: where it came from does not change when its
// wording is fixed. A requirement not on the campaign is ErrRequirementNotFound;
// a campaign already open is refused by the freeze trigger.
func (s *Store) CorrectRequirement(ctx context.Context, tenantID, campaignID, requirementID, text, status string) (Requirement, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Requirement{}, fmt.Errorf("recruiting: beginning correction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, tenantID); err != nil {
		return Requirement{}, err
	}
	queries := db.New(tx)
	if err := mustBeDraft(ctx, queries, campaignID); err != nil {
		return Requirement{}, err
	}
	row, err := queries.CorrectRequirement(ctx, db.CorrectRequirementParams{
		ID: requirementID, CampaignID: campaignID, Text: text, Status: status,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Requirement{}, ErrRequirementNotFound
	}
	if err != nil {
		return Requirement{}, fmt.Errorf("recruiting: correcting the requirement: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Requirement{}, fmt.Errorf("recruiting: committing the correction: %w", err)
	}
	return requirementFromCorrect(row), nil
}

func requirementFromInsert(row db.InsertRequirementRow) Requirement {
	return Requirement{
		ID: row.ID, CampaignID: row.CampaignID, Text: row.Text,
		SpanStart: int(row.SpanStart), SpanEnd: int(row.SpanEnd),
		Status: row.Status, ExtractionVersion: row.ExtractionVersion,
	}
}

func requirementFromList(row db.RequirementsForCampaignRow) Requirement {
	return Requirement{
		ID: row.ID, CampaignID: row.CampaignID, Text: row.Text,
		SpanStart: int(row.SpanStart), SpanEnd: int(row.SpanEnd),
		Status: row.Status, ExtractionVersion: row.ExtractionVersion,
	}
}

func requirementFromCorrect(row db.CorrectRequirementRow) Requirement {
	return Requirement{
		ID: row.ID, CampaignID: row.CampaignID, Text: row.Text,
		SpanStart: int(row.SpanStart), SpanEnd: int(row.SpanEnd),
		Status: row.Status, ExtractionVersion: row.ExtractionVersion,
	}
}
