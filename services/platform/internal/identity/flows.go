package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
	"github.com/Yelethe1st/prepeet/services/platform/platform/password"
	"github.com/Yelethe1st/prepeet/services/platform/platform/ratelimit"
	"github.com/Yelethe1st/prepeet/services/platform/platform/token"
)

// The four token flows: email verification, password recovery, magic link
// and one-time code. Implements IAM-02.
//
// The request half of every flow answers identically for a known and an
// unknown address, because "we sent you an email" to an address we did not is
// the price of not confirming which addresses hold accounts. The consume half
// does the opposite: expired, already used and superseded each get their own
// error, because the person holding a dead link deserves to know which dead
// it is, and none of those states is secret from someone who has the link.

// Outcomes a presented token can have, distinct because the prototype's
// screens are distinct and a person can act differently on each.
var (
	// ErrTokenInvalid covers a token we never issued. Deliberately vague:
	// everything else about it is unknown, including whether it was an attack.
	ErrTokenInvalid = errors.New("identity: that link is not valid")
	// ErrTokenExpired means it was real and time ran out. The fix is asking
	// for a new one, and the message can safely say so.
	ErrTokenExpired = errors.New("identity: that link has expired")
	// ErrTokenUsed means it already did its work once. Second clicks on one
	// email are normal; the screen says nothing further is needed.
	ErrTokenUsed = errors.New("identity: that link has already been used")
	// ErrTokenSuperseded means a newer email replaced this one, which the
	// newer email's existence explains.
	ErrTokenSuperseded = errors.New("identity: a newer link has replaced that one")
	// ErrCodeIncorrect means the one-time code did not match.
	ErrCodeIncorrect = errors.New("identity: that code is not right")
	// ErrTooManyAttempts means the code was guessed at until it died.
	ErrTooManyAttempts = errors.New("identity: too many wrong codes; ask for a new one")
)

// CooldownError refuses a resend, carrying how long until the next is allowed.
//
// A type rather than a sentinel because the duration is the message: the
// prototype shows a visible countdown, and a bare "slow down" would leave the
// interface inventing one.
type CooldownError struct {
	RetryAfter time.Duration
}

func (e *CooldownError) Error() string {
	return fmt.Sprintf("identity: a recent email is on its way; try again in %s", e.RetryAfter)
}

// How long each proof lives. Stated in the email each sends, so the expired
// screen is never the first mention of a deadline. A sign-in proof lives
// shorter than a verification one because it grants more.
const (
	verifyTTL = 30 * time.Minute
	resetTTL  = 30 * time.Minute
	magicTTL  = 15 * time.Minute
	otpTTL    = 10 * time.Minute
)

// otpMaxAttempts is how many wrong guesses kill a code. Six digits survive
// online guessing only because of this cap; the length is usability.
const otpMaxAttempts = 5

// TokenFlows is what the flows need beyond the repository.
type TokenFlows struct {
	// Mailer carries each token's email inside the issuing transaction.
	Mailer Mailer
	// Resend is the per-address cooldown on the request half. The decision's
	// RetryAfter becomes the visible countdown.
	Resend ratelimit.Counter
	// BaseURL is where links point, such as https://app.prepeet.com.
	BaseURL string
}

// WithTokenFlows equips the service for IAM-02's flows.
//
// Separate from NewService so the processes that never send email, such as
// the worker's use of Lookup, do not have to construct a mailer to satisfy a
// constructor.
func (s *Service) WithTokenFlows(flows TokenFlows) *Service {
	s.flows = &flows
	return s
}

// requireFlows guards the flow entry points, loudly rather than with a nil
// dereference three calls deep.
func (s *Service) requireFlows() (*TokenFlows, error) {
	if s.flows == nil {
		return nil, errors.New("identity: token flows are not configured; call WithTokenFlows at composition")
	}
	return s.flows, nil
}

// RequestEmailVerification sends a fresh verification email.
//
// Nil for an unknown address and nil for an already-verified one, because the
// response must not say which addresses hold accounts, and a verified person
// re-requesting has nothing to do; mailing them "verify your email" would
// only teach them to ignore real ones.
func (s *Service) RequestEmailVerification(ctx context.Context, rawEmail string) error {
	return s.sendToken(ctx, rawEmail, token.PurposeEmailVerify, verifyTTL,
		func(flows *TokenFlows, tx pgx.Tx, recipient, plaintext string) error {
			return flows.Mailer.SendEmailVerification(ctx, tx, recipient,
				flows.BaseURL+"/verify-email?token="+plaintext, int(verifyTTL.Minutes()))
		})
}

// ConfirmEmailVerification consumes a verification link.
//
// Replay-safe by construction: the mark and the effect share a transaction,
// and a replay finds the mark and earns ErrTokenUsed with nothing repeated.
func (s *Service) ConfirmEmailVerification(ctx context.Context, plaintext string) error {
	state, err := s.consumable(ctx, plaintext, token.PurposeEmailVerify)
	if err != nil {
		return err
	}

	won, err := s.repo.ConsumeForEmailVerification(ctx, state.ID, state.UserID)
	if err != nil {
		return err
	}
	if !won {
		// Two clicks raced and this one lost; the winner did the work.
		return ErrTokenUsed
	}
	return nil
}

// RequestPasswordReset sends a recovery email.
func (s *Service) RequestPasswordReset(ctx context.Context, rawEmail string) error {
	return s.sendToken(ctx, rawEmail, token.PurposePasswordReset, resetTTL,
		func(flows *TokenFlows, tx pgx.Tx, recipient, plaintext string) error {
			return flows.Mailer.SendPasswordReset(ctx, tx, recipient,
				flows.BaseURL+"/reset-password?token="+plaintext, int(resetTTL.Minutes()))
		})
}

// ConfirmPasswordReset consumes a recovery link and sets the new password.
//
// Every session is revoked in the same transaction. The reset exists because
// the old password may be known to somebody else, and that somebody may be
// holding a session right now.
func (s *Service) ConfirmPasswordReset(ctx context.Context, plaintext, newPassword string) error {
	if err := ValidatePassword(newPassword); err != nil {
		return err
	}

	state, err := s.consumable(ctx, plaintext, token.PurposePasswordReset)
	if err != nil {
		return err
	}

	hash, err := password.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("identity: hashing password: %w", err)
	}

	won, err := s.repo.ConsumeForPasswordReset(ctx, state.ID, state.UserID, hash, s.clock())
	if err != nil {
		return err
	}
	if !won {
		return ErrTokenUsed
	}
	return nil
}

// RequestMagicLink sends a sign-in link.
func (s *Service) RequestMagicLink(ctx context.Context, rawEmail string) error {
	return s.sendToken(ctx, rawEmail, token.PurposeMagicLink, magicTTL,
		func(flows *TokenFlows, tx pgx.Tx, recipient, plaintext string) error {
			return flows.Mailer.SendMagicLink(ctx, tx, recipient,
				flows.BaseURL+"/magic-link?token="+plaintext, int(magicTTL.Minutes()))
		})
}

// ConsumeMagicLink signs the holder in.
//
// It also marks the address verified, because arriving here proves control of
// it, which is the same proof the verification email asks for.
func (s *Service) ConsumeMagicLink(ctx context.Context, plaintext string) (Session, error) {
	state, err := s.consumable(ctx, plaintext, token.PurposeMagicLink)
	if err != nil {
		return Session{}, err
	}

	won, err := s.repo.ConsumeForSignIn(ctx, state.ID, state.UserID)
	if err != nil {
		return Session{}, err
	}
	if !won {
		return Session{}, ErrTokenUsed
	}

	// The session is issued after the consume commits. A crash between the
	// two costs the person a second email rather than ever leaving a live
	// token that already produced a session.
	now := s.clock()
	return s.issue(ctx, state.UserID, id.New().String(), now, now)
}

// ProvisionCandidateSession resolves the invitation's candidate and signs them
// in, whether or not they already had an account.
//
// It is the identity half of SCR-05's acceptance. The invitation token, which
// the caller has already validated, is what proves control of the address, so
// this issues a session on the strength of it exactly as ConsumeMagicLink does
// on the strength of a magic-link token. The same session either way is the
// requirement ADR-0003 states: a candidate who was new and one who already had
// an account are indistinguishable from here, so acceptance cannot be used to
// learn whether an address is registered.
//
// The userID is returned alongside the session because the acceptance flow
// records the accepted invitation against the candidate, and the caller needs
// to know who that resolved to.
func (s *Service) ProvisionCandidateSession(ctx context.Context, rawEmail string) (string, Session, error) {
	email := NormaliseEmail(rawEmail)
	if err := ValidateEmail(email); err != nil {
		return "", Session{}, err
	}

	userID, err := s.repo.ProvisionCandidate(ctx, email)
	if err != nil {
		return "", Session{}, err
	}

	now := s.clock()
	session, err := s.issue(ctx, userID, id.New().String(), now, now)
	if err != nil {
		return "", Session{}, err
	}
	return userID, session, nil
}

// RequestOTP sends a one-time code.
func (s *Service) RequestOTP(ctx context.Context, rawEmail string) error {
	return s.sendCode(ctx, rawEmail)
}

// ConfirmOTP exchanges an emailed code for a session.
//
// The code is short enough to guess, so everything here is about the cap:
// wrong guesses are counted on the token, the fifth kills it, and the count
// survives restarts because it lives on the row rather than in memory.
func (s *Service) ConfirmOTP(ctx context.Context, rawEmail, code string) (Session, error) {
	if _, err := s.requireFlows(); err != nil {
		return Session{}, err
	}
	email := NormaliseEmail(rawEmail)

	userID, _, err := s.repo.FindCredentialsByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// The same failure as a wrong code. A distinct answer here would
			// make the code entry screen an address oracle.
			return Session{}, ErrCodeIncorrect
		}
		return Session{}, fmt.Errorf("identity: looking up address: %w", err)
	}

	live, err := s.repo.FindLiveOTP(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Session{}, ErrCodeIncorrect
		}
		return Session{}, err
	}
	if s.clock().After(live.ExpiresAt) {
		return Session{}, ErrTokenExpired
	}

	if !token.Equals(live.TokenHash, token.HashOf(code)) {
		attempts, err := s.repo.RecordTokenAttempt(ctx, live.ID)
		if err != nil {
			return Session{}, err
		}
		if attempts >= otpMaxAttempts {
			// The token dies by being marked used, so the right code arriving
			// on the next try is told "already used" rather than let through
			// after five wrong neighbours.
			if _, err := s.repo.ConsumeForSignIn(ctx, live.ID, userID); err != nil {
				return Session{}, err
			}
			return Session{}, ErrTooManyAttempts
		}
		return Session{}, ErrCodeIncorrect
	}

	won, err := s.repo.ConsumeForSignIn(ctx, live.ID, userID)
	if err != nil {
		return Session{}, err
	}
	if !won {
		return Session{}, ErrTokenUsed
	}

	now := s.clock()
	return s.issue(ctx, userID, id.New().String(), now, now)
}

// sendToken is the request half every link flow shares.
func (s *Service) sendToken(ctx context.Context, rawEmail string, purpose token.Purpose,
	ttl time.Duration, enqueue func(flows *TokenFlows, tx pgx.Tx, recipient, plaintext string) error) error {
	flows, err := s.requireFlows()
	if err != nil {
		return err
	}

	email := NormaliseEmail(rawEmail)
	if err := ValidateEmail(email); err != nil {
		return err
	}

	// The cooldown is charged before the address is looked up, so a request
	// for an unknown address costs the same and cools down the same. A
	// cooldown that only known addresses experienced would itself be the
	// oracle the identical responses exist to prevent.
	decision, err := flows.Resend.Allow(ctx, string(purpose)+":"+email)
	if err != nil {
		return fmt.Errorf("identity: checking the resend cooldown: %w", err)
	}
	if !decision.Allowed {
		return &CooldownError{RetryAfter: decision.RetryAfter}
	}

	userID, _, err := s.repo.FindCredentialsByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil // indistinguishable from success, deliberately
		}
		return fmt.Errorf("identity: looking up address: %w", err)
	}

	minted, err := token.New(purpose)
	if err != nil {
		return err
	}

	return s.repo.IssueActionToken(ctx, ActionTokenRow{
		ID:        id.New().String(),
		UserID:    userID,
		Purpose:   string(purpose),
		TokenHash: minted.Hash,
		ExpiresAt: s.clock().Add(ttl),
	}, func(tx pgx.Tx) error {
		return enqueue(flows, tx, email, minted.Plaintext)
	})
}

// sendCode is sendToken for the one flow whose secret is typed, not clicked.
func (s *Service) sendCode(ctx context.Context, rawEmail string) error {
	flows, err := s.requireFlows()
	if err != nil {
		return err
	}

	email := NormaliseEmail(rawEmail)
	if err := ValidateEmail(email); err != nil {
		return err
	}

	decision, err := flows.Resend.Allow(ctx, string(token.PurposeOTP)+":"+email)
	if err != nil {
		return fmt.Errorf("identity: checking the resend cooldown: %w", err)
	}
	if !decision.Allowed {
		return &CooldownError{RetryAfter: decision.RetryAfter}
	}

	userID, _, err := s.repo.FindCredentialsByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return fmt.Errorf("identity: looking up address: %w", err)
	}

	minted, err := token.NewOTP()
	if err != nil {
		return err
	}

	return s.repo.IssueActionToken(ctx, ActionTokenRow{
		ID:        id.New().String(),
		UserID:    userID,
		Purpose:   string(token.PurposeOTP),
		TokenHash: minted.Hash,
		ExpiresAt: s.clock().Add(otpTTL),
	}, func(tx pgx.Tx) error {
		return flows.Mailer.SendOTP(ctx, tx, email, minted.Plaintext, int(otpTTL.Minutes()))
	})
}

// consumable reads a presented token and decides which outcome it has earned.
//
// The order of checks is the order of usefulness to the person: used beats
// superseded beats expired, because "you already did this" ends the journey
// happily, while "expired" sends them back for another email.
func (s *Service) consumable(ctx context.Context, plaintext string, purpose token.Purpose) (TokenState, error) {
	// The prefix check costs nothing and stops a session token pasted into a
	// verification URL from ever reaching the table.
	if presented, ok := token.PurposeOf(plaintext); !ok || presented != purpose {
		return TokenState{}, ErrTokenInvalid
	}

	state, err := s.repo.FindActionToken(ctx, token.HashOf(plaintext))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return TokenState{}, ErrTokenInvalid
		}
		return TokenState{}, err
	}
	if state.Purpose != string(purpose) {
		return TokenState{}, ErrTokenInvalid
	}

	switch {
	case state.UsedAt != nil:
		return TokenState{}, ErrTokenUsed
	case state.SupersededAt != nil:
		return TokenState{}, ErrTokenSuperseded
	case s.clock().After(state.ExpiresAt):
		return TokenState{}, ErrTokenExpired
	}
	return state, nil
}
