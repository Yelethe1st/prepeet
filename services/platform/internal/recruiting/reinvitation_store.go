package recruiting

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/recruiting/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// ReInvitation is a named human's decision to let a candidate try again.
type ReInvitation struct {
	ID                 string
	CampaignID         string
	CandidateID        string
	Reason             string
	DecidedBy          string
	InterruptedSession string
	ConsumedSession    string
	CreatedAt          time.Time
}

var (
	// ErrReInvitationReasonRequired means a re-invitation was authorized with no
	// reason. A re-invitation without one is the automatic retry the whole
	// mechanism exists to refuse.
	ErrReInvitationReasonRequired = errors.New("recruiting: a re-invitation requires a reason")
	// ErrReInvitationDeciderRequired means no human was named as deciding it.
	ErrReInvitationDeciderRequired = errors.New("recruiting: a re-invitation requires a named human")
	// ErrNoReInvitation means the candidate holds no unclaimed re-invitation, so
	// they are not authorized to start a further session.
	ErrNoReInvitation = errors.New("recruiting: the candidate holds no re-invitation")
)

// AuthorizeReInvitation records a recruiter's decision, under the tenant scope.
// Reason and decider are refused when empty here as well as by the schema, so a
// caller gets a clean error rather than a constraint violation.
func (s *Store) AuthorizeReInvitation(ctx context.Context, tenantID, campaignID, candidateID, reason, decidedBy, interruptedSession string) (ReInvitation, error) {
	if strings.TrimSpace(reason) == "" {
		return ReInvitation{}, ErrReInvitationReasonRequired
	}
	if strings.TrimSpace(decidedBy) == "" {
		return ReInvitation{}, ErrReInvitationDeciderRequired
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ReInvitation{}, fmt.Errorf("recruiting: beginning re-invitation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, tenantID); err != nil {
		return ReInvitation{}, err
	}
	row, err := db.New(tx).AuthorizeReInvitation(ctx, db.AuthorizeReInvitationParams{
		ID: id.New().String(), CampaignID: campaignID, TenantID: tenantID,
		CandidateID: candidateID, Reason: reason, DecidedBy: decidedBy,
		InterruptedSession: interruptedSession,
	})
	if err != nil {
		return ReInvitation{}, fmt.Errorf("recruiting: recording the re-invitation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ReInvitation{}, fmt.Errorf("recruiting: committing the re-invitation: %w", err)
	}
	return reInvitationFrom(row.ID, row.CampaignID, row.CandidateID, row.Reason,
		row.DecidedBy, row.InterruptedSession, row.ConsumedSession, row.CreatedAt), nil
}

// ClaimReInvitation binds the candidate's oldest unclaimed re-invitation to a
// new session, as the candidate themselves, so one authorization admits exactly
// one further attempt. ErrNoReInvitation when they hold none.
func (s *Store) ClaimReInvitation(ctx context.Context, campaignID, candidateID, sessionID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("recruiting: beginning claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetUser(ctx, tx, candidateID); err != nil {
		return err
	}
	_, err = db.New(tx).ClaimReInvitation(ctx, db.ClaimReInvitationParams{
		CampaignID: campaignID, CandidateID: candidateID, SessionID: sessionID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoReInvitation
	}
	if err != nil {
		return fmt.Errorf("recruiting: claiming the re-invitation: %w", err)
	}
	return tx.Commit(ctx)
}

// ReInvitationsForCandidate lists a candidate's re-invitations for the
// recruiter's audit, under the tenant scope.
func (s *Store) ReInvitationsForCandidate(ctx context.Context, tenantID, campaignID, candidateID string) ([]ReInvitation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("recruiting: beginning re-invitation read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	rows, err := db.New(tx).ReInvitationsForCandidate(ctx, db.ReInvitationsForCandidateParams{
		CampaignID: campaignID, CandidateID: candidateID,
	})
	if err != nil {
		return nil, fmt.Errorf("recruiting: listing re-invitations: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("recruiting: committing the re-invitation read: %w", err)
	}
	out := make([]ReInvitation, 0, len(rows))
	for _, row := range rows {
		out = append(out, reInvitationFrom(row.ID, row.CampaignID, row.CandidateID, row.Reason,
			row.DecidedBy, row.InterruptedSession, row.ConsumedSession, row.CreatedAt))
	}
	return out, nil
}

func reInvitationFrom(id, campaignID, candidateID, reason, decidedBy string, interrupted, consumed *string, createdAt time.Time) ReInvitation {
	r := ReInvitation{
		ID: id, CampaignID: campaignID, CandidateID: candidateID,
		Reason: reason, DecidedBy: decidedBy, CreatedAt: createdAt,
	}
	if interrupted != nil {
		r.InterruptedSession = *interrupted
	}
	if consumed != nil {
		r.ConsumedSession = *consumed
	}
	return r
}
