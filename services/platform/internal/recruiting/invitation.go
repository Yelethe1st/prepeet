package recruiting

import (
	"errors"
	"time"
)

// Invitation is one issued link as the recruiter's side reads it.
//
// It never carries the token, only the fact that one was issued and how it
// ended. The plaintext lives for the single call that mints it and is then
// unrecoverable; the recruiter's view is about who was invited and whether the
// email reached them, never about the credential itself.
type Invitation struct {
	ID         string
	TenantID   string
	CampaignID string
	Recipient  string
	// EmailID points at the notification row this invitation was carried by,
	// so delivery status can be joined where the two contexts compose. It is
	// not a link the recruiter reads; it is how cmd answers "did it arrive".
	EmailID   string
	IssuedBy  string
	IssuedAt  time.Time
	ExpiresAt time.Time
	Outcome   InvitationOutcome
	// OutcomeAt is nil exactly when the invitation is still live.
	OutcomeAt *time.Time
}

// InvitationOutcome is how an invitation ended, or the empty string while it
// is still live. Expiry is not an outcome: it is time against ExpiresAt, which
// is why Status computes it rather than reading it from here.
type InvitationOutcome string

const (
	// InvitationLive is the only non-terminal state: a null outcome column.
	InvitationLive InvitationOutcome = ""
	// InvitationAccepted and InvitationDeclined are the candidate's endings,
	// recorded by acceptance (SCR-05), never by this ticket's code.
	InvitationAccepted InvitationOutcome = "accepted"
	InvitationDeclined InvitationOutcome = "declined"
	// InvitationRevoked is the recruiter's ending: the link is stopped, and
	// nothing else about the invitation or the candidate is touched.
	InvitationRevoked InvitationOutcome = "revoked"
	// InvitationSuperseded is a resend retiring the link it replaces.
	InvitationSuperseded InvitationOutcome = "superseded"
)

// Invitation status strings a recruiter reads. Live and expired are computed
// from the outcome and the clock; the rest echo the terminal outcome.
const (
	statusLive    = "live"
	statusExpired = "expired"
)

// Status is the state a recruiter sees, with expiry decided against now.
//
// A terminal outcome wins over the clock: a revoked invitation reads revoked
// even after its expiry passes, because how it ended is a fact and expiry is
// only what would have happened had nothing else. A live invitation past its
// expires_at reads expired, so the row does not have to be flipped by a job
// for the truth to be told.
func (i Invitation) Status(now time.Time) string {
	if i.Outcome != InvitationLive {
		return string(i.Outcome)
	}
	if now.After(i.ExpiresAt) {
		return statusExpired
	}
	return statusLive
}

// Live reports whether the invitation could still be accepted right now: no
// terminal outcome and not yet expired. It is the one predicate the consume
// path (SCR-05) and the recruiter's resend share, so it lives here rather than
// being re-derived at each call site.
func (i Invitation) Live(now time.Time) bool {
	return i.Outcome == InvitationLive && !now.After(i.ExpiresAt)
}

var (
	// ErrCampaignNotOpen is returned when an invitation is issued against a
	// campaign that is not open. A draft has not fixed its configuration and a
	// closed campaign issues nothing, so neither may admit a candidate.
	ErrCampaignNotOpen = errors.New("recruiting: campaign is not open")

	// ErrInvitationNotFound is returned when a revoke names no live invitation:
	// the id is unknown, belongs to another tenant, or the invitation has
	// already ended. Revocation of something already terminal is not an error
	// the recruiter can act on differently, so all three collapse to one.
	ErrInvitationNotFound = errors.New("recruiting: no such live invitation")
)

// InvitationExpiry is how long a screening invitation stands.
//
// Days, not minutes, because a candidate answers an interview invitation
// around a job and a life rather than in the one sitting a verification link
// assumes. Seven days is long enough not to expire under a weekend and a busy
// week, short enough that a stale link does not linger for a role already
// filled.
const InvitationExpiry = 7 * 24 * time.Hour
