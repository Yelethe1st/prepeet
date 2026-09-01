package recruiting

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/recruiting/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// ErrNoAccess means this recruiter is not on this campaign.
//
// Deliberately the same answer as a campaign that does not exist. A recruiter
// who can tell "not yours" from "no such thing" can enumerate a tenant's
// campaigns by asking, and the roster of who is hiring for what is exactly the
// kind of thing a competitor would ask for.
var ErrNoAccess = errors.New("recruiting: no such campaign")

// Store is the campaign context's persistence.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore builds the store over a pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// scope sets the tenant for the transaction, which is what every policy on
// these tables reads. Without it the row-level security matches nothing and
// every query returns empty, which fails closed.
func scope(ctx context.Context, tx pgx.Tx, tenantID string) error {
	return database.SetTenant(ctx, tx, tenantID)
}

// CreateDraft starts a campaign. Draft is the only state it can start in.
func (s *Store) CreateDraft(ctx context.Context, campaign Campaign) (Campaign, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Campaign{}, fmt.Errorf("recruiting: beginning create: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, campaign.TenantID); err != nil {
		return Campaign{}, err
	}

	row, err := db.New(tx).CreateCampaign(ctx, db.CreateCampaignParams{
		ID: id.New().String(), TenantID: campaign.TenantID, Name: campaign.Name,
		RoleReference: campaign.RoleReference, Jurisdiction: campaign.Jurisdiction,
		CreatedBy: campaign.CreatedBy,
	})
	if err != nil {
		return Campaign{}, fmt.Errorf("recruiting: creating the campaign: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Campaign{}, fmt.Errorf("recruiting: committing the campaign: %w", err)
	}
	return campaignFrom(row), nil
}

// Open freezes a campaign's configuration and admits it to issuing invitations.
//
// Everything happens in one transaction: the pins, the determination and the
// status move together or not at all. A campaign that was open with half its
// pins written would be one whose configuration is not actually fixed, which is
// the single thing this whole context exists to guarantee.
func (s *Store) Open(ctx context.Context, campaign Campaign, opening Opening) (Campaign, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Campaign{}, fmt.Errorf("recruiting: beginning open: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, campaign.TenantID); err != nil {
		return Campaign{}, err
	}
	q := db.New(tx)

	for _, pin := range opening.Pins {
		if err := q.PinArtifact(ctx, db.PinArtifactParams{
			CampaignID: campaign.ID, TenantID: campaign.TenantID,
			ArtifactType: pin.Type, ArtifactID: pinArtifactID(pin),
			Digest: pin.Digest, Reference: pin.Reference, Version: pin.Version,
		}); err != nil {
			return Campaign{}, fmt.Errorf("recruiting: pinning %s: %w", pin.Type, err)
		}
	}

	row, err := q.OpenCampaign(ctx, db.OpenCampaignParams{
		ID: campaign.ID, DeterminationID: &opening.Determination.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// The guard is in the query's WHERE, so no rows means it was not a
		// draft. Two concurrent opens cannot both succeed: the second finds
		// nothing to update rather than overwriting the first one's pins.
		return Campaign{}, fmt.Errorf("%w: it was not a draft when the write landed", ErrNotDraft)
	}
	if err != nil {
		return Campaign{}, fmt.Errorf("recruiting: opening the campaign: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Campaign{}, fmt.Errorf("recruiting: committing the open: %w", err)
	}
	return campaignFrom(row), nil
}

// pinArtifactID answers the artifact identifier a pin carries.
//
// The domain Pin does not carry one today because the opening decision has no
// use for it: the digest is the identity. The column exists so an operator can
// join back to content.artifacts, and it takes the campaign's own generated id
// when the resolver did not supply one, rather than being nullable and letting
// half the rows have no route back.
func pinArtifactID(pin Pin) string {
	if pin.ArtifactID != "" {
		return pin.ArtifactID
	}
	return id.New().String()
}

// CampaignForRecruiter reads a campaign only if this recruiter is on it.
//
// The access check and the read are one query rather than two steps, because
// two steps is where the check gets skipped: a later caller reaching for the
// plain read is not doing anything that looks wrong.
func (s *Store) CampaignForRecruiter(ctx context.Context, tenantID, campaignID, userID string) (Campaign, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Campaign{}, fmt.Errorf("recruiting: beginning read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, tenantID); err != nil {
		return Campaign{}, err
	}

	rows, err := db.New(tx).CampaignsForRecruiter(ctx, userID)
	if err != nil {
		return Campaign{}, fmt.Errorf("recruiting: reading campaigns: %w", err)
	}
	for _, row := range rows {
		if row.ID == campaignID {
			return campaignFrom(row), nil
		}
	}
	return Campaign{}, ErrNoAccess
}

// GrantAccess puts a recruiter on a campaign.
func (s *Store) GrantAccess(ctx context.Context, tenantID, campaignID, userID, grantedBy string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("recruiting: beginning grant: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, tenantID); err != nil {
		return err
	}
	if err := db.New(tx).GrantCampaignAccess(ctx, db.GrantCampaignAccessParams{
		CampaignID: campaignID, TenantID: tenantID, UserID: userID, GrantedBy: grantedBy,
	}); err != nil {
		return fmt.Errorf("recruiting: granting access: %w", err)
	}
	return tx.Commit(ctx)
}

// RecordAcceptance stores a candidate accepting a disclosure version, together
// with every consent decision made against it.
//
// One transaction, because an acceptance without its decisions would read as
// consent to everything, and decisions without their acceptance would be
// answers to a document nobody is recorded as having seen.
func (s *Store) RecordAcceptance(ctx context.Context, acceptance Acceptance, decisions []ConsentDecision) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("recruiting: beginning acceptance: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, acceptance.TenantID); err != nil {
		return err
	}
	q := db.New(tx)

	if err := q.RecordAcceptance(ctx, db.RecordAcceptanceParams{
		ID: id.New().String(), TenantID: acceptance.TenantID,
		CampaignID: acceptance.CampaignID, CandidateID: acceptance.CandidateID,
		DisclosureDigest:  acceptance.DisclosureDigest,
		DisclosureVersion: acceptance.DisclosureVersion,
	}); err != nil {
		return fmt.Errorf("recruiting: recording the acceptance: %w", err)
	}

	for _, decision := range decisions {
		if err := q.RecordConsentDecision(ctx, db.RecordConsentDecisionParams{
			ID: id.New().String(), TenantID: acceptance.TenantID,
			CampaignID: acceptance.CampaignID, CandidateID: acceptance.CandidateID,
			Purpose: decision.Purpose, Required: decision.Required, Granted: decision.Granted,
			DisclosureDigest: acceptance.DisclosureDigest,
		}); err != nil {
			return fmt.Errorf("recruiting: recording consent for %s: %w", decision.Purpose, err)
		}
	}
	return tx.Commit(ctx)
}

// StandingConsent answers what this candidate currently agrees to.
//
// The latest decision per purpose, so a withdrawal recorded later becomes the
// standing answer without any row being edited.
func (s *Store) StandingConsent(ctx context.Context, tenantID, campaignID, candidateID string) ([]ConsentDecision, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("recruiting: beginning consent read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	rows, err := db.New(tx).StandingConsent(ctx, db.StandingConsentParams{
		CampaignID: campaignID, CandidateID: candidateID,
	})
	if err != nil {
		return nil, fmt.Errorf("recruiting: reading consent: %w", err)
	}
	decisions := make([]ConsentDecision, 0, len(rows))
	for _, row := range rows {
		decisions = append(decisions, ConsentDecision{
			Purpose: row.Purpose, Required: row.Required, Granted: row.Granted,
		})
	}
	return decisions, nil
}

// AcceptancesFor answers every disclosure version this candidate accepted.
func (s *Store) AcceptancesFor(ctx context.Context, tenantID, campaignID, candidateID string) ([]Acceptance, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("recruiting: beginning acceptance read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	rows, err := db.New(tx).AcceptancesFor(ctx, db.AcceptancesForParams{
		CampaignID: campaignID, CandidateID: candidateID,
	})
	if err != nil {
		return nil, fmt.Errorf("recruiting: reading acceptances: %w", err)
	}
	acceptances := make([]Acceptance, 0, len(rows))
	for _, row := range rows {
		acceptances = append(acceptances, Acceptance{
			TenantID: tenantID, CampaignID: row.CampaignID, CandidateID: row.CandidateID,
			DisclosureDigest: row.DisclosureDigest, DisclosureVersion: row.DisclosureVersion,
			AcceptedAt: row.AcceptedAt,
		})
	}
	return acceptances, nil
}

// campaignFrom maps a generated row onto the domain type.
func campaignFrom(row db.RecruitingCampaign) Campaign {
	campaign := Campaign{
		ID: row.ID, TenantID: row.TenantID, Name: row.Name,
		Status: Status(row.Status), RoleReference: row.RoleReference,
		Jurisdiction: row.Jurisdiction, OpenedAt: row.OpenedAt, ClosedAt: row.ClosedAt,
		CreatedAt: row.CreatedAt, CreatedBy: row.CreatedBy,
	}
	if row.DeterminationID != nil {
		campaign.DeterminationID = *row.DeterminationID
	}
	return campaign
}

// CampaignsUsing answers which open campaigns pinned an artifact reference.
//
// The caller is the rubric library, deciding whether an author may discard a
// draft. It asks rather than looks: campaigns are this context's and the
// library is another's, so the answer crosses as a list of names that goes
// straight into the refusal a person reads.
//
// Names rather than identifiers for the same reason. "backend-hiring-q3 is
// still using it" is something an author can act on; a UUID is something they
// have to go and look up.
func (s *Store) CampaignsUsing(ctx context.Context, tenantID, reference string) ([]string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("recruiting: beginning the usage read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	names, err := db.New(tx).CampaignsUsingArtifact(ctx, reference)
	if err != nil {
		return nil, fmt.Errorf("recruiting: reading campaigns using %s: %w", reference, err)
	}
	return names, nil
}
