package recruiting

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/recruiting/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// IssueInvitationInput is everything an issue needs that is not the token.
//
// The campaign is the one already resolved for the recruiter, so its status is
// the status a recruiter with access just read; issuing trusts it the same way
// Open trusts the campaign handed to it. The token hash is supplied rather than
// minted here because the plaintext belongs to the caller's email and must not
// exist in this package at all.
type IssueInvitationInput struct {
	Campaign  Campaign
	Recipient string
	IssuedBy  string
	ExpiresAt time.Time
	TokenHash string
}

// IssueInvitation supersedes the recipient's live links, enqueues the email and
// stores the new invitation, all in one transaction.
//
// The three are inseparable for the outbox's reason, applied twice over. A
// supersede without a successor strands a recipient whose old link now fails
// and whose new one was never written. An email committed without its
// invitation is a link to a row that does not exist; an invitation committed
// without its email is a link nobody was ever sent. And the enqueue runs before
// the insert only so the invitation can record the email's id: the email is the
// invitation's proof of delivery, so the row points at it rather than the other
// way round.
//
// enqueue is the caller's, and returns the stored email's id. It is a closure
// rather than a port on this store because the message it builds carries the
// plaintext link, which is the one thing this package must never see.
func (s *Store) IssueInvitation(ctx context.Context, input IssueInvitationInput,
	enqueue func(tx pgx.Tx) (emailID string, err error)) (Invitation, error) {
	if input.Campaign.Status != StatusOpen {
		return Invitation{}, fmt.Errorf("%w: %s", ErrCampaignNotOpen, input.Campaign.Status)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Invitation{}, fmt.Errorf("recruiting: beginning invitation issue: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, input.Campaign.TenantID); err != nil {
		return Invitation{}, err
	}

	queries := db.New(tx)
	if err := queries.SupersedeLiveInvitations(ctx, db.SupersedeLiveInvitationsParams{
		CampaignID: input.Campaign.ID, Recipient: input.Recipient,
	}); err != nil {
		return Invitation{}, fmt.Errorf("recruiting: retiring the recipient's live invitations: %w", err)
	}

	emailID, err := enqueue(tx)
	if err != nil {
		return Invitation{}, fmt.Errorf("recruiting: enqueueing the invitation email: %w", err)
	}

	row, err := queries.IssueInvitation(ctx, db.IssueInvitationParams{
		ID: id.New().String(), TenantID: input.Campaign.TenantID,
		CampaignID: input.Campaign.ID, Recipient: input.Recipient,
		TokenHash: input.TokenHash, EmailID: emailID,
		IssuedBy: input.IssuedBy, ExpiresAt: input.ExpiresAt,
	})
	if err != nil {
		return Invitation{}, fmt.Errorf("recruiting: storing the invitation: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Invitation{}, fmt.Errorf("recruiting: committing the invitation: %w", err)
	}
	return invitationFromIssue(row), nil
}

// RevokeInvitation stops one live invitation on one campaign and returns what
// it was.
//
// The campaign is part of the guard, not decoration: a recruiter admitted to
// one campaign cannot revoke another campaign's invitation in the same tenant
// by its id alone, because the id matches nothing without the campaign the
// caller was already checked against. The guard on a live outcome lives in the
// query too, so a link the candidate has already accepted or declined cannot be
// revoked out from under the record of what they did, and revoking one already
// terminal returns ErrInvitationNotFound rather than a second ending. Nothing
// is deleted: the row stays, the campaign stays, and anything the candidate did
// under the invitation stays, which is what revocation has to be able to say
// plainly.
func (s *Store) RevokeInvitation(ctx context.Context, tenantID, campaignID, invitationID string) (Invitation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Invitation{}, fmt.Errorf("recruiting: beginning revoke: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, tenantID); err != nil {
		return Invitation{}, err
	}

	row, err := db.New(tx).RevokeInvitation(ctx, db.RevokeInvitationParams{
		ID: invitationID, CampaignID: campaignID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Invitation{}, ErrInvitationNotFound
	}
	if err != nil {
		return Invitation{}, fmt.Errorf("recruiting: revoking the invitation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Invitation{}, fmt.Errorf("recruiting: committing the revoke: %w", err)
	}
	return invitationFromRevoke(row), nil
}

// InvitationsForCampaign lists one campaign's invitations, newest first.
//
// Tenant scoping is the row-level security policy's, so a caller who forgets to
// scope reads nothing rather than another workspace's roster. The per-campaign
// access check is the handler's, exactly as it is for reading the campaign
// itself: this returns the invitations of a campaign the caller was already
// allowed to see.
func (s *Store) InvitationsForCampaign(ctx context.Context, tenantID, campaignID string) ([]Invitation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("recruiting: beginning invitation list: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	rows, err := db.New(tx).InvitationsForCampaign(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("recruiting: listing invitations: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("recruiting: committing the invitation list: %w", err)
	}

	out := make([]Invitation, 0, len(rows))
	for _, row := range rows {
		out = append(out, invitationFromList(row))
	}
	return out, nil
}

// InvitationByID reads one invitation on one campaign, or ErrInvitationNotFound.
//
// The resend path needs the recipient and the outcome before it decides whether
// a fresh link may go out, and this is that read. Scoped by campaign like every
// other invitation read, so a recruiter cannot reach another campaign's
// invitation by id.
func (s *Store) InvitationByID(ctx context.Context, tenantID, campaignID, invitationID string) (Invitation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Invitation{}, fmt.Errorf("recruiting: beginning invitation read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, tenantID); err != nil {
		return Invitation{}, err
	}

	row, err := db.New(tx).InvitationByID(ctx, db.InvitationByIDParams{
		ID: invitationID, CampaignID: campaignID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Invitation{}, ErrInvitationNotFound
	}
	if err != nil {
		return Invitation{}, fmt.Errorf("recruiting: reading the invitation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Invitation{}, fmt.Errorf("recruiting: committing the invitation read: %w", err)
	}
	return invitationFromByID(row), nil
}

// The Row shapes carry the same columns under different generated names, so the
// mapping is written once per shape rather than once per field.

func invitationFromByID(row db.InvitationByIDRow) Invitation {
	return Invitation{
		ID: row.ID, TenantID: row.TenantID, CampaignID: row.CampaignID,
		Recipient: row.Recipient, EmailID: row.EmailID, IssuedBy: row.IssuedBy,
		IssuedAt: row.IssuedAt, ExpiresAt: row.ExpiresAt,
		Outcome: outcomeFrom(row.Outcome.Valid, row.Outcome.String), OutcomeAt: row.OutcomeAt,
	}
}

func invitationFromIssue(row db.IssueInvitationRow) Invitation {
	return Invitation{
		ID: row.ID, TenantID: row.TenantID, CampaignID: row.CampaignID,
		Recipient: row.Recipient, EmailID: row.EmailID, IssuedBy: row.IssuedBy,
		IssuedAt: row.IssuedAt, ExpiresAt: row.ExpiresAt,
		Outcome: outcomeFrom(row.Outcome.Valid, row.Outcome.String), OutcomeAt: row.OutcomeAt,
	}
}

func invitationFromRevoke(row db.RevokeInvitationRow) Invitation {
	return Invitation{
		ID: row.ID, TenantID: row.TenantID, CampaignID: row.CampaignID,
		Recipient: row.Recipient, EmailID: row.EmailID, IssuedBy: row.IssuedBy,
		IssuedAt: row.IssuedAt, ExpiresAt: row.ExpiresAt,
		Outcome: outcomeFrom(row.Outcome.Valid, row.Outcome.String), OutcomeAt: row.OutcomeAt,
	}
}

func invitationFromList(row db.InvitationsForCampaignRow) Invitation {
	return Invitation{
		ID: row.ID, TenantID: row.TenantID, CampaignID: row.CampaignID,
		Recipient: row.Recipient, EmailID: row.EmailID, IssuedBy: row.IssuedBy,
		IssuedAt: row.IssuedAt, ExpiresAt: row.ExpiresAt,
		Outcome: outcomeFrom(row.Outcome.Valid, row.Outcome.String), OutcomeAt: row.OutcomeAt,
	}
}

// outcomeFrom turns the nullable outcome column into the domain's outcome,
// where a null is the live state rather than a missing value to reinterpret.
func outcomeFrom(valid bool, value string) InvitationOutcome {
	if !valid {
		return InvitationLive
	}
	return InvitationOutcome(value)
}
