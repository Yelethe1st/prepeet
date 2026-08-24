package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
	"github.com/Yelethe1st/prepeet/services/platform/platform/password"
	"github.com/Yelethe1st/prepeet/services/platform/platform/token"
)

// Session and refresh lifetimes.
//
// The session is short because it is presented on every request and revocation
// still costs a lookup. The refresh window is what actually decides how long
// someone stays signed in, and rotation means a stolen refresh token is only
// useful until the legitimate client next refreshes.
const (
	sessionLifetime = 30 * time.Minute
	refreshLifetime = 30 * 24 * time.Hour
)

// ErrCredentialsInvalid is returned for a wrong password and for an address
// that was never registered.
//
// Deliberately one error for both. Two errors here would enumerate accounts one
// login at a time, and the timing is equalised for the same reason: a fast
// rejection says what a carefully worded message will not.
var ErrCredentialsInvalid = errors.New("identity: those credentials did not authenticate")

// ErrSessionInvalid is returned when a token is unknown, expired or revoked.
// The caller cannot act differently on the three, and distinguishing them would
// tell whoever presented it which.
var ErrSessionInvalid = errors.New("identity: that session is not valid")

// Session is an issued token pair and what it authorises.
//
// SessionToken and RefreshToken are plaintext and exist for exactly one
// response. They are never stored, never logged, and unrecoverable afterwards.
type Session struct {
	ID              string
	UserID          string
	FamilyID        string
	SessionToken    string
	RefreshToken    string
	ExpiresAt       time.Time
	RefreshExpires  time.Time
	AuthenticatedAt time.Time
}

// String redacts both tokens, because a struct printed with %v is the ordinary
// way a live credential reaches a log.
func (s Session) String() string {
	return fmt.Sprintf("identity.Session{ID:%s UserID:%s FamilyID:%s SessionToken:[redacted] RefreshToken:[redacted]}",
		s.ID, s.UserID, s.FamilyID)
}

// RegisterInput is a registration request.
type RegisterInput struct {
	Email            string
	Password         string
	AccountType      AccountType
	OrganisationName string
}

// RegisterOutcome is what registration produced.
//
// UserID and VerificationToken are populated only when an account was actually
// created. For an address that already exists the outcome is deliberately
// empty, so a caller cannot turn it into an existence check by inspecting what
// came back.
type RegisterOutcome struct {
	Created           bool
	UserID            string
	VerificationToken string
	// TenantID is populated only for an organisation registration that created
	// an account. Empty for a candidate, who belongs to no tenant, and empty for
	// an address that already existed, for the same reason every other field is.
	TenantID string
}

// Repository is the persistence this service needs.
//
// An interface rather than a concrete type so the service can be reasoned about
// without a database, and so a second implementation, such as a read replica
// for lookups, does not require changing this file.
type Repository interface {
	FindCredentialsByEmail(ctx context.Context, email string) (userID, passwordHash string, err error)
	CreateUserWithCredentials(ctx context.Context, userID, email, passwordHash string) error
	// CreateOrganisationAccount creates the person, the workspace and the
	// membership that says the person administers it, in one transaction.
	//
	// One method rather than three, because the atomicity is the requirement.
	// A half-created registration gives somebody who can verify their address,
	// sign in, and find no workspace, with nothing having visibly failed, and
	// the address is now taken by an account that cannot be completed.
	//
	// It returns the slug actually used, which may differ from the one offered
	// because slugs collide and two organisations may share a name.
	CreateOrganisationAccount(ctx context.Context, account OrganisationAccount) (slug string, err error)
	UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error

	FindUserByID(ctx context.Context, userID string) (User, error)
	FindMembershipsByUser(ctx context.Context, userID string) ([]Membership, error)

	CreateSession(ctx context.Context, row SessionRow) error
	FindSessionByToken(ctx context.Context, tokenHash string) (SessionRow, error)
	FindSessionByRefresh(ctx context.Context, tokenHash string) (SessionRow, error)
	RetireSession(ctx context.Context, sessionID string, at time.Time) error
	RevokeFamily(ctx context.Context, familyID, reason string, at time.Time) error
}

// OrganisationAccount is everything an organisation registration writes.
//
// Assembled by the service and written by the repository in one transaction.
// The identifiers are generated here rather than in SQL so that a caller knows
// them before the write, which is what lets the outcome name the tenant without
// a second query.
type OrganisationAccount struct {
	UserID       string
	Email        string
	PasswordHash string

	TenantID         string
	OrganisationName string
	// Slug is a suggestion. The repository appends to it on collision and
	// reports what it used.
	Slug string
	// Region records residency at creation. ADR-0001 makes this a property of
	// the tenant rather than of the deployment, so it is decidable per tenant
	// from the first row rather than inferred from where the process happened
	// to be running.
	Region string

	MembershipID string
}

// SessionRow is the stored form of a session. Only hashes, never tokens.
type SessionRow struct {
	ID               string
	UserID           string
	FamilyID         string
	SessionTokenHash string
	RefreshTokenHash string
	IssuedAt         time.Time
	ExpiresAt        time.Time
	RefreshExpiresAt time.Time
	AuthenticatedAt  time.Time
	RetiredAt        *time.Time
	RevokedAt        *time.Time
}

// Service is the identity use cases.
type Service struct {
	repo Repository
	now  func() time.Time
	// region is stamped onto every tenant this service creates. ADR-0001 makes
	// residency a property of the tenant, so it is recorded rather than
	// inferred, and a process misconfigured with the wrong region produces
	// tenants that say so rather than tenants that say nothing.
	region string
}

// NewService builds the service. The clock is injected so tests do not depend
// on the wall clock and so expiry can be exercised without waiting.
func NewService(repo Repository, now func() time.Time) *Service {
	return &Service{repo: repo, now: now, region: DefaultRegion}
}

// DefaultRegion is where a tenant is created unless told otherwise.
//
// eu-west-2 per ADR-0001. A constant rather than configuration because a second
// region is a project rather than a setting, and a configurable value here would
// let a misconfigured process silently create tenants outside the residency
// commitment candidates were shown.
const DefaultRegion = "eu-west-2"

// clock is the only way this service reads the time.
//
// It normalises to UTC rather than trusting the injected clock to. Timestamps
// here are stored, compared across processes and serialised to browsers, and a
// process running with a local timezone would otherwise emit offsets that
// differ from every other process in the system. Normalising at the single
// point of use makes that independent of how the service was wired.
func (s *Service) clock() time.Time {
	return s.now().UTC()
}

// Register creates an account, or does nothing and says the same thing.
//
// The response is identical whether or not the address was already registered,
// because confirming it would let anyone enumerate who practises for interviews,
// which is information a candidate never agreed to share. Crucially the existing
// account's password is not touched: re-registering an address must not be a way
// to take it over.
func (s *Service) Register(ctx context.Context, input RegisterInput) (RegisterOutcome, error) {
	email := NormaliseEmail(input.Email)
	if err := ValidateEmail(email); err != nil {
		return RegisterOutcome{}, err
	}
	if err := ValidatePassword(input.Password); err != nil {
		return RegisterOutcome{}, err
	}
	if err := ValidateAccountType(input.AccountType, input.OrganisationName); err != nil {
		return RegisterOutcome{}, err
	}

	existingID, _, err := s.repo.FindCredentialsByEmail(ctx, email)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return RegisterOutcome{}, fmt.Errorf("identity: looking up address: %w", err)
	}
	if existingID != "" {
		// The address is taken. Hash anyway so the two paths cost the same, then
		// return the same shape a new registration returns.
		_ = password.DummyVerify(input.Password)
		return RegisterOutcome{Created: false}, nil
	}

	hash, err := password.Hash(input.Password)
	if err != nil {
		return RegisterOutcome{}, fmt.Errorf("identity: hashing password: %w", err)
	}

	userID := id.New().String()

	// An organisation registration is one transaction covering three tables.
	// A candidate registration touches only identity, so it takes the narrower
	// path rather than a workspace-shaped one with the workspace left empty:
	// every person practising alone would otherwise own an employer tenant they
	// never asked for, and that tenant would be in the billing and retention
	// inventories for no reason.
	tenantID := ""
	if input.AccountType == AccountOrganisation {
		tenantID = id.New().String()
		if _, err := s.repo.CreateOrganisationAccount(ctx, OrganisationAccount{
			UserID:           userID,
			Email:            email,
			PasswordHash:     hash,
			TenantID:         tenantID,
			OrganisationName: input.OrganisationName,
			Slug:             Slugify(input.OrganisationName),
			Region:           s.region,
			MembershipID:     id.New().String(),
		}); err != nil {
			return RegisterOutcome{}, fmt.Errorf("identity: creating organisation account: %w", err)
		}
	} else if err := s.repo.CreateUserWithCredentials(ctx, userID, email, hash); err != nil {
		return RegisterOutcome{}, fmt.Errorf("identity: creating user: %w", err)
	}

	verification, err := token.New(token.PurposeEmailVerify)
	if err != nil {
		return RegisterOutcome{}, fmt.Errorf("identity: issuing verification token: %w", err)
	}

	return RegisterOutcome{
		Created:           true,
		UserID:            userID,
		VerificationToken: verification.Plaintext,
		TenantID:          tenantID,
	}, nil
}

// Authenticate exchanges credentials for a session.
//
// An unknown address performs a dummy verification before failing, so the two
// failing cases cost comparable time. Without it, a fast rejection tells an
// attacker the address is unregistered however carefully the message is worded.
func (s *Service) Authenticate(ctx context.Context, rawEmail, plaintext string) (Session, error) {
	email := NormaliseEmail(rawEmail)

	userID, hash, err := s.repo.FindCredentialsByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			_ = password.DummyVerify(plaintext)
			return Session{}, ErrCredentialsInvalid
		}
		return Session{}, fmt.Errorf("identity: looking up credentials: %w", err)
	}

	result, err := password.Verify(hash, plaintext)
	if err != nil {
		// An unparseable hash is a corrupted record rather than a wrong
		// password. It is reported as a failure to authenticate so nothing is
		// revealed, and wrapped so an operator can find it.
		return Session{}, fmt.Errorf("identity: stored credential is unusable for user %s: %w", userID, err)
	}
	if !result.Match {
		return Session{}, ErrCredentialsInvalid
	}

	if result.NeedsUpgrade {
		// The only moment the plaintext is available to rehash. A failure here
		// must not fail the login: the user authenticated correctly, and the
		// old hash still works.
		if upgraded, hashErr := password.Hash(plaintext); hashErr == nil {
			_ = s.repo.UpdatePasswordHash(ctx, userID, upgraded)
		}
	}

	now := s.clock()
	return s.issue(ctx, userID, id.New().String(), now, now)
}

// Refresh rotates a session, and treats a reused token as theft.
//
// Presenting a token that has already been rotated away means either a stolen
// token or a client bug. Both are handled the same way, because we cannot tell
// them apart and the cost of being wrong is asymmetric: being logged out is a
// cheap failure, and an attacker keeping a foothold is not.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (Session, error) {
	now := s.clock()

	row, err := s.repo.FindSessionByRefresh(ctx, token.HashOf(refreshToken))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Session{}, ErrSessionInvalid
		}
		return Session{}, fmt.Errorf("identity: looking up refresh token: %w", err)
	}

	if row.RetiredAt != nil {
		// Reuse. Everything descended from this login goes, including the
		// session the legitimate client is currently holding, which is the
		// point: we do not know which of the two is the attacker.
		if err := s.repo.RevokeFamily(ctx, row.FamilyID, "refresh token reused", now); err != nil {
			return Session{}, fmt.Errorf("identity: revoking family after reuse: %w", err)
		}
		return Session{}, ErrSessionInvalid
	}
	if row.RevokedAt != nil || !now.Before(row.RefreshExpiresAt) {
		return Session{}, ErrSessionInvalid
	}

	if err := s.repo.RetireSession(ctx, row.ID, now); err != nil {
		return Session{}, fmt.Errorf("identity: retiring session: %w", err)
	}

	// The rotation carries the original authentication time forward. Refreshing
	// is not proving who you are, so it must not satisfy a step-up check.
	return s.issue(ctx, row.UserID, row.FamilyID, now, row.AuthenticatedAt)
}

// Lookup resolves a session token to the session it authorises.
func (s *Service) Lookup(ctx context.Context, sessionToken string) (SessionRow, error) {
	row, err := s.repo.FindSessionByToken(ctx, token.HashOf(sessionToken))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return SessionRow{}, ErrSessionInvalid
		}
		return SessionRow{}, fmt.Errorf("identity: looking up session: %w", err)
	}

	if row.RevokedAt != nil || !s.clock().Before(row.ExpiresAt) {
		return SessionRow{}, ErrSessionInvalid
	}
	return row, nil
}

// Revoke ends a session and its whole family.
//
// Logging out takes the refresh family with it, so a refresh token stolen
// earlier cannot outlive the logout. Idempotent: a client retrying a logout
// should not be told it failed for having succeeded.
func (s *Service) Revoke(ctx context.Context, sessionToken, reason string) error {
	row, err := s.repo.FindSessionByToken(ctx, token.HashOf(sessionToken))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return fmt.Errorf("identity: looking up session: %w", err)
	}
	if row.RevokedAt != nil {
		return nil
	}
	if err := s.repo.RevokeFamily(ctx, row.FamilyID, reason, s.clock()); err != nil {
		return fmt.Errorf("identity: revoking family: %w", err)
	}
	return nil
}

// issue mints a token pair and records it.
// User is what GET /me reports about a person.
//
// Deliberately not the whole row. Status and version are operational fields the
// interface has no use for, and a struct that carried them would eventually be
// serialised somewhere that did not want them.
type User struct {
	ID            string
	Email         string
	EmailVerified bool
	// Memberships are every tenant this person belongs to. A person may belong
	// to several, which is why the interface offers a switcher, and listing one
	// is not a statement that they may currently act under it: status says that.
	Memberships []Membership
}

// Membership is one tenant a person belongs to, as GET /me reports it.
type Membership struct {
	TenantID   string
	TenantName string
	Status     string
	Role       string
}

// Describe returns a user by id.
//
// By id rather than by session token, because the caller has already resolved
// the session and re-deriving who is acting would be a second opportunity to
// get it wrong. It reports ErrNotFound rather than an empty User: an empty User
// has an empty id, and a caller that ignored the error would act as somebody who
// does not exist.
func (s *Service) Describe(ctx context.Context, userID string) (User, error) {
	user, err := s.repo.FindUserByID(ctx, userID)
	if err != nil {
		return User{}, err
	}

	memberships, err := s.repo.FindMembershipsByUser(ctx, userID)
	if err != nil {
		return User{}, fmt.Errorf("identity: reading memberships: %w", err)
	}
	user.Memberships = memberships
	return user, nil
}

func (s *Service) issue(ctx context.Context, userID, familyID string, now, authenticatedAt time.Time) (Session, error) {
	sessionToken, err := token.New(token.PurposeSession)
	if err != nil {
		return Session{}, fmt.Errorf("identity: issuing session token: %w", err)
	}
	refreshToken, err := token.New(token.PurposeRefresh)
	if err != nil {
		return Session{}, fmt.Errorf("identity: issuing refresh token: %w", err)
	}

	row := SessionRow{
		ID:               id.New().String(),
		UserID:           userID,
		FamilyID:         familyID,
		SessionTokenHash: sessionToken.Hash,
		RefreshTokenHash: refreshToken.Hash,
		IssuedAt:         now,
		ExpiresAt:        now.Add(sessionLifetime),
		RefreshExpiresAt: now.Add(refreshLifetime),
		AuthenticatedAt:  authenticatedAt,
	}
	if err := s.repo.CreateSession(ctx, row); err != nil {
		return Session{}, fmt.Errorf("identity: recording session: %w", err)
	}

	return Session{
		ID:              row.ID,
		UserID:          row.UserID,
		FamilyID:        row.FamilyID,
		SessionToken:    sessionToken.Plaintext,
		RefreshToken:    refreshToken.Plaintext,
		ExpiresAt:       row.ExpiresAt,
		RefreshExpires:  row.RefreshExpiresAt,
		AuthenticatedAt: row.AuthenticatedAt,
	}, nil
}
