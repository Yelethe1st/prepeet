package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/identity/db"
)

// Mailer is the one thing identity asks of the outside world for its token
// flows: put this message into the same transaction as the token it carries.
//
// Declared here per ADR-0005: identity consumes the capability, so identity
// says how narrow it is. The implementation lives in internal/notification
// and cmd wires the two, because a context must not import another.
//
// Every method takes the transaction. A token committed without its email is
// a token nobody can ever use; an email whose token rolled back is a link to
// nothing. The pair lives or dies together or the flow lies to somebody.
type Mailer interface {
	SendEmailVerification(ctx context.Context, tx pgx.Tx, recipient, link string, expiresMinutes int) error
	SendPasswordReset(ctx context.Context, tx pgx.Tx, recipient, link string, expiresMinutes int) error
	SendMagicLink(ctx context.Context, tx pgx.Tx, recipient, link string, expiresMinutes int) error
	SendOTP(ctx context.Context, tx pgx.Tx, recipient, code string, expiresMinutes int) error
}

// ActionTokenRow is the stored form of a single-use token. Only the hash.
type ActionTokenRow struct {
	ID        string
	UserID    string
	Purpose   string
	TokenHash string
	ExpiresAt time.Time
}

// TokenState is everything consumption needs to decide an outcome.
type TokenState struct {
	ID           string
	UserID       string
	Purpose      string
	ExpiresAt    time.Time
	UsedAt       *time.Time
	SupersededAt *time.Time
	Attempts     int
}

// IssueActionToken supersedes the user's live tokens of the same purpose,
// stores the new one, and enqueues its email, in one transaction.
//
// One method because the three cannot be separated: a supersede without its
// successor strands the person with nothing that works, and the email argument
// carries the plaintext, which exists only inside this call.
func (r *PostgresRepository) IssueActionToken(ctx context.Context, row ActionTokenRow,
	enqueue func(tx pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("identity: beginning token issue: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := db.New(tx)
	if err := q.SupersedeActionTokens(ctx, db.SupersedeActionTokensParams{
		UserID: row.UserID, Purpose: row.Purpose,
	}); err != nil {
		return fmt.Errorf("identity: superseding tokens: %w", err)
	}
	if err := q.InsertActionToken(ctx, db.InsertActionTokenParams{
		ID: row.ID, UserID: row.UserID, Purpose: row.Purpose,
		TokenHash: row.TokenHash, ExpiresAt: row.ExpiresAt,
	}); err != nil {
		return fmt.Errorf("identity: storing token: %w", err)
	}
	if err := enqueue(tx); err != nil {
		return fmt.Errorf("identity: enqueueing the token's email: %w", err)
	}
	return tx.Commit(ctx)
}

// FindActionToken reads a token by the hash of what was presented.
func (r *PostgresRepository) FindActionToken(ctx context.Context, tokenHash string) (TokenState, error) {
	row, err := r.q.FindActionTokenByHash(ctx, tokenHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return TokenState{}, ErrNotFound
	}
	if err != nil {
		return TokenState{}, fmt.Errorf("identity: reading token: %w", err)
	}
	return TokenState{
		ID: row.ID, UserID: row.UserID, Purpose: row.Purpose,
		ExpiresAt: row.ExpiresAt, UsedAt: row.UsedAt,
		SupersededAt: row.SupersededAt, Attempts: int(row.Attempts),
	}, nil
}

// FindLiveOTP reads the newest live code for a user.
type LiveOTP struct {
	ID        string
	TokenHash string
	ExpiresAt time.Time
	Attempts  int
}

// FindLiveOTP returns the newest live one-time code issued to a user.
func (r *PostgresRepository) FindLiveOTP(ctx context.Context, userID string) (LiveOTP, error) {
	row, err := r.q.FindLiveOTP(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return LiveOTP{}, ErrNotFound
	}
	if err != nil {
		return LiveOTP{}, fmt.Errorf("identity: reading code: %w", err)
	}
	return LiveOTP{ID: row.ID, TokenHash: row.TokenHash, ExpiresAt: row.ExpiresAt, Attempts: int(row.Attempts)}, nil
}

// ConsumeForEmailVerification marks the token used and the address verified,
// in one transaction, so a replay repeats nothing.
//
// It reports false when the token lost the race to another presentation of
// itself: the guard on the update means exactly one of two concurrent clicks
// wins, and the loser is told the token was already used.
func (r *PostgresRepository) ConsumeForEmailVerification(ctx context.Context, tokenID, userID string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("identity: beginning verification: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := db.New(tx)
	won, err := q.MarkActionTokenUsed(ctx, tokenID)
	if err != nil {
		return false, fmt.Errorf("identity: consuming token: %w", err)
	}
	if won == 0 {
		return false, nil
	}
	if err := q.MarkEmailVerified(ctx, userID); err != nil {
		return false, fmt.Errorf("identity: recording verification: %w", err)
	}
	return true, tx.Commit(ctx)
}

// ConsumeForPasswordReset marks the token used, replaces the hash, and revokes
// every session, in one transaction.
//
// The revocation is not optional: the reset exists because the password may be
// known to somebody else, and that somebody may be holding a session.
func (r *PostgresRepository) ConsumeForPasswordReset(ctx context.Context, tokenID, userID, passwordHash string, at time.Time) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("identity: beginning reset: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := db.New(tx)
	won, err := q.MarkActionTokenUsed(ctx, tokenID)
	if err != nil {
		return false, fmt.Errorf("identity: consuming token: %w", err)
	}
	if won == 0 {
		return false, nil
	}
	if err := q.UpdatePasswordHash(ctx, db.UpdatePasswordHashParams{
		UserID: userID, PasswordHash: passwordHash,
	}); err != nil {
		return false, fmt.Errorf("identity: replacing password: %w", err)
	}
	if err := q.RevokeAllSessions(ctx, db.RevokeAllSessionsParams{
		UserID: userID, RevokedAt: at, Reason: "password_reset",
	}); err != nil {
		return false, fmt.Errorf("identity: revoking sessions: %w", err)
	}
	return true, tx.Commit(ctx)
}

// ConsumeForSignIn marks the token used and the address verified, in one
// transaction, and reports whether this presentation won.
//
// Verified because arriving here proves control of the address, which is the
// same proof the verification email asks for; a person who signed in by magic
// link should not still be nagged to verify.
func (r *PostgresRepository) ConsumeForSignIn(ctx context.Context, tokenID, userID string) (bool, error) {
	return r.ConsumeForEmailVerification(ctx, tokenID, userID)
}

// RecordTokenAttempt counts a wrong guess and returns the new total.
func (r *PostgresRepository) RecordTokenAttempt(ctx context.Context, tokenID string) (int, error) {
	attempts, err := r.q.RecordTokenAttempt(ctx, tokenID)
	if err != nil {
		return 0, fmt.Errorf("identity: recording attempt: %w", err)
	}
	return int(attempts), nil
}
