//go:build integration

// Identity service tests against real PostgreSQL.
//
// These are the tests ADR-0003 exists for. Building authentication rather than
// buying it makes password handling and session revocation a standing
// obligation of this codebase, and an obligation nobody tests is a claim.
package identity_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Yelethe1st/prepeet/services/platform/internal/identity"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/password"
)

var (
	adminURL string
	pool     *pgxpool.Pool
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("prepeet"),
		tcpostgres.WithUsername("prepeet_migrator"),
		tcpostgres.WithPassword("migrator-password"),
		// Not ForListeningPort. PostgreSQL accepts TCP connections before it
		// will answer them, so that strategy returns while the server is still
		// replying "the database system is starting up" and the first
		// connection fails. It made this suite flaky rather than broken, which
		// is worse: a failure that looks like the code under test.
		//
		// The occurrence matters as much as the log line. The official image
		// starts a temporary server to run its initialisation scripts and logs
		// readiness for that one too, so waiting for the first occurrence waits
		// for a server that is about to be shut down.
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(2*time.Minute)),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "starting PostgreSQL: %v\n", err)
		os.Exit(1)
	}

	adminURL, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "connection string: %v\n", err)
		os.Exit(1)
	}
	if err := database.Migrate(ctx, adminURL, database.MigrateOptions{AppPassword: "app-password"}); err != nil {
		fmt.Fprintf(os.Stderr, "migrating: %v\n", err)
		os.Exit(1)
	}

	cfg, err := pgx.ParseConfig(adminURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parsing connection string: %v\n", err)
		os.Exit(1)
	}
	pool, err = pgxpool.New(ctx, fmt.Sprintf(
		"postgres://prepeet_app:app-password@%s:%d/%s?sslmode=disable",
		cfg.Host, cfg.Port, cfg.Database))
	if err != nil {
		fmt.Fprintf(os.Stderr, "app pool: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	pool.Close()
	if err := container.Terminate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "terminating PostgreSQL: %v\n", err)
	}
	os.Exit(code)
}

func newService(t *testing.T) *identity.Service {
	t.Helper()
	return identity.NewService(identity.NewRepository(pool), time.Now)
}

// emailFor gives each test its own address, since the suite shares a database.
func emailFor(t *testing.T) string {
	t.Helper()
	return strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-")) + "@example.com"
}

const goodPassword = "correct horse battery staple"

// ────────────────────────────────────────────────────────────── registration

func TestRegisterCreatesAUserAndCredentials(t *testing.T) {
	ctx := context.Background()
	service := newService(t)

	outcome, err := service.Register(ctx, identity.RegisterInput{
		Email: emailFor(t), Password: goodPassword, AccountType: identity.AccountCandidate,
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if !outcome.Created {
		t.Error("Created is false for a new address")
	}
	if outcome.VerificationToken == "" {
		t.Error("no verification token was issued, so the address can never be confirmed")
	}
}

// Registering an address that already exists answers exactly as a new one does.
// Confirming an address here would let anyone enumerate who practises for
// interviews, which is information a candidate never agreed to share.
func TestRegisterDoesNotRevealThatAnAddressExists(t *testing.T) {
	ctx := context.Background()
	service := newService(t)
	email := emailFor(t)

	first, err := service.Register(ctx, identity.RegisterInput{
		Email: email, Password: goodPassword, AccountType: identity.AccountCandidate,
	})
	if err != nil {
		t.Fatalf("first Register returned error: %v", err)
	}

	second, err := service.Register(ctx, identity.RegisterInput{
		Email: email, Password: "a completely different password", AccountType: identity.AccountCandidate,
	})
	if err != nil {
		t.Fatalf("second Register returned error: %v", err)
	}

	if second.Created {
		t.Error("Created is true for an address that already existed")
	}
	if first.UserID == "" {
		t.Fatal("the first registration produced no user")
	}
	if second.UserID != "" {
		t.Error("the second registration returned a user identifier, which confirms the address exists")
	}
}

// The second registration must not overwrite the password, or anyone could
// take over an account by registering the address again.
func TestRegisteringAnExistingAddressDoesNotChangeThePassword(t *testing.T) {
	ctx := context.Background()
	service := newService(t)
	email := emailFor(t)

	if _, err := service.Register(ctx, identity.RegisterInput{
		Email: email, Password: goodPassword, AccountType: identity.AccountCandidate,
	}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if _, err := service.Register(ctx, identity.RegisterInput{
		Email: email, Password: "attacker chosen password", AccountType: identity.AccountCandidate,
	}); err != nil {
		t.Fatalf("second Register returned error: %v", err)
	}

	if _, err := service.Authenticate(ctx, email, "attacker chosen password"); err == nil {
		t.Fatal("the attacker's password now authenticates, so re-registration took over the account")
	}
	if _, err := service.Authenticate(ctx, email, goodPassword); err != nil {
		t.Errorf("the original password no longer authenticates: %v", err)
	}
}

func TestRegisterNormalisesTheAddress(t *testing.T) {
	ctx := context.Background()
	service := newService(t)
	email := emailFor(t)

	if _, err := service.Register(ctx, identity.RegisterInput{
		Email: strings.ToUpper(email), Password: goodPassword, AccountType: identity.AccountCandidate,
	}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	if _, err := service.Authenticate(ctx, email, goodPassword); err != nil {
		t.Errorf("an address registered in upper case does not authenticate in lower case: %v", err)
	}
}

// A password reaching the database in plaintext is the failure this whole
// package exists to prevent, so it is asserted directly rather than assumed.
func TestPasswordIsNeverStoredInPlaintext(t *testing.T) {
	ctx := context.Background()
	service := newService(t)

	if _, err := service.Register(ctx, identity.RegisterInput{
		Email: emailFor(t), Password: goodPassword, AccountType: identity.AccountCandidate,
	}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	var found int
	if err := conn.QueryRow(ctx,
		`SELECT count(*) FROM identity.credentials WHERE password_hash LIKE '%' || $1 || '%'`,
		goodPassword).Scan(&found); err != nil {
		t.Fatalf("querying credentials: %v", err)
	}
	if found != 0 {
		t.Error("the password appears in the credentials table")
	}
}

// ──────────────────────────────────────────────────────────── authentication

func TestAuthenticateIssuesASession(t *testing.T) {
	ctx := context.Background()
	service := newService(t)
	email := emailFor(t)

	if _, err := service.Register(ctx, identity.RegisterInput{
		Email: email, Password: goodPassword, AccountType: identity.AccountCandidate,
	}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	session, err := service.Authenticate(ctx, email, goodPassword)
	if err != nil {
		t.Fatalf("Authenticate returned error: %v", err)
	}
	if session.SessionToken == "" || session.RefreshToken == "" {
		t.Fatal("Authenticate returned no tokens")
	}
	if !session.ExpiresAt.After(time.Now()) {
		t.Error("the session is already expired")
	}
	if session.AuthenticatedAt.IsZero() {
		t.Error("AuthenticatedAt is zero, and step-up decisions depend on it")
	}
}

// A wrong password and an unknown address are the same event to anyone
// watching. Different errors here would enumerate accounts one login at a time.
func TestWrongPasswordAndUnknownAddressAreIndistinguishable(t *testing.T) {
	ctx := context.Background()
	service := newService(t)
	email := emailFor(t)

	if _, err := service.Register(ctx, identity.RegisterInput{
		Email: email, Password: goodPassword, AccountType: identity.AccountCandidate,
	}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	_, wrongPassword := service.Authenticate(ctx, email, "not the password")
	_, unknownAddress := service.Authenticate(ctx, "nobody-here-"+emailFor(t), goodPassword)

	if wrongPassword == nil || unknownAddress == nil {
		t.Fatal("one of the two failing cases succeeded")
	}
	if wrongPassword.Error() != unknownAddress.Error() {
		t.Errorf("errors differ:\n  wrong password: %v\n  unknown address: %v", wrongPassword, unknownAddress)
	}
}

// The response body says the same thing for both cases, and so must the clock.
// A fast rejection tells an attacker the address is unregistered however
// carefully the message is worded.
func TestUnknownAddressCostsComparableTimeToAWrongPassword(t *testing.T) {
	ctx := context.Background()
	service := newService(t)
	email := emailFor(t)

	if _, err := service.Register(ctx, identity.RegisterInput{
		Email: email, Password: goodPassword, AccountType: identity.AccountCandidate,
	}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	// Warm both paths so the first call's setup is not measured.
	_, _ = service.Authenticate(ctx, email, "wrong")
	_, _ = service.Authenticate(ctx, "absent-"+email, goodPassword)

	start := time.Now()
	_, _ = service.Authenticate(ctx, email, "wrong")
	known := time.Since(start)

	start = time.Now()
	_, _ = service.Authenticate(ctx, "absent-"+email, goodPassword)
	unknown := time.Since(start)

	// A generous ratio: the point is that the unknown path does real work, not
	// that the two match to the microsecond.
	if unknown < known/4 {
		t.Errorf("unknown address took %s against %s for a wrong password, which is fast enough to enumerate",
			unknown, known)
	}
}

// Raising the argon2 parameters must not lock anyone out, and must not leave
// old hashes weak forever.
func TestLoginUpgradesAHashMadeUnderWeakerParameters(t *testing.T) {
	ctx := context.Background()
	service := newService(t)
	email := emailFor(t)

	weak, err := password.HashWith(goodPassword, password.Params{
		Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32,
	})
	if err != nil {
		t.Fatalf("HashWith: %v", err)
	}
	userID := seedUserWithHash(t, email, weak)

	if _, err := service.Authenticate(ctx, email, goodPassword); err != nil {
		t.Fatalf("a hash under old parameters did not authenticate: %v", err)
	}

	after := storedHash(t, userID)
	if after == weak {
		t.Error("the stored hash was not upgraded after a successful login")
	}

	result, err := password.Verify(after, goodPassword)
	if err != nil {
		t.Fatalf("verifying the upgraded hash: %v", err)
	}
	if !result.Match {
		t.Error("the upgraded hash does not verify the original password")
	}
	if result.NeedsUpgrade {
		t.Error("the upgraded hash still reports NeedsUpgrade")
	}
}

// ───────────────────────────────────────────────────────────────── sessions

func TestSessionLookupFindsAnActiveSession(t *testing.T) {
	ctx := context.Background()
	service := newService(t)
	session := register(t, service, emailFor(t))

	found, err := service.Lookup(ctx, session.SessionToken)
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if found.UserID != session.UserID {
		t.Errorf("UserID = %q, want %q", found.UserID, session.UserID)
	}
}

func TestSessionTokenIsNeverStoredInPlaintext(t *testing.T) {
	ctx := context.Background()
	service := newService(t)
	session := register(t, service, emailFor(t))

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	var found int
	if err := conn.QueryRow(ctx,
		`SELECT count(*) FROM identity.sessions
		 WHERE session_token_hash = $1 OR refresh_token_hash = $2`,
		session.SessionToken, session.RefreshToken).Scan(&found); err != nil {
		t.Fatalf("querying sessions: %v", err)
	}
	if found != 0 {
		t.Error("a token is stored in plaintext, so reading the session table yields usable credentials")
	}
}

func TestLookupRefusesAnUnknownToken(t *testing.T) {
	ctx := context.Background()
	service := newService(t)

	if _, err := service.Lookup(ctx, "ses_not-a-real-token"); err == nil {
		t.Error("Lookup accepted a token that was never issued")
	}
}

func TestLogoutEndsTheSession(t *testing.T) {
	ctx := context.Background()
	service := newService(t)
	session := register(t, service, emailFor(t))

	if err := service.Revoke(ctx, session.SessionToken, "logout"); err != nil {
		t.Fatalf("Revoke returned error: %v", err)
	}
	if _, err := service.Lookup(ctx, session.SessionToken); err == nil {
		t.Error("the session still resolves after logout")
	}
}

// Logging out twice is not an error. A client retrying a logout should not see
// a failure for having succeeded.
func TestLogoutIsIdempotent(t *testing.T) {
	ctx := context.Background()
	service := newService(t)
	session := register(t, service, emailFor(t))

	if err := service.Revoke(ctx, session.SessionToken, "logout"); err != nil {
		t.Fatalf("first Revoke returned error: %v", err)
	}
	if err := service.Revoke(ctx, session.SessionToken, "logout"); err != nil {
		t.Errorf("second Revoke returned error: %v", err)
	}
}

// ─────────────────────────────────────────────────────── rotation and reuse

func TestRefreshRotatesTheTokens(t *testing.T) {
	ctx := context.Background()
	service := newService(t)
	session := register(t, service, emailFor(t))

	rotated, err := service.Refresh(ctx, session.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}

	if rotated.SessionToken == session.SessionToken {
		t.Error("the session token was not rotated")
	}
	if rotated.RefreshToken == session.RefreshToken {
		t.Error("the refresh token was not rotated")
	}
	if _, err := service.Lookup(ctx, rotated.SessionToken); err != nil {
		t.Errorf("the rotated session does not resolve: %v", err)
	}
}

// Rotation is what makes reuse detectable, so the old token must stop working
// the moment a new one is issued.
func TestTheRetiredRefreshTokenStopsWorking(t *testing.T) {
	ctx := context.Background()
	service := newService(t)
	session := register(t, service, emailFor(t))

	if _, err := service.Refresh(ctx, session.RefreshToken); err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}

	if _, err := service.Refresh(ctx, session.RefreshToken); err == nil {
		t.Error("the retired refresh token was accepted a second time")
	}
}

// The heart of ADR-0003. Presenting a retired token means either a stolen token
// or a client bug, and both are handled the same way: everything descended from
// that login is revoked. Being logged out is a cheap failure; an attacker
// keeping a foothold is not.
func TestReusingARetiredRefreshTokenRevokesTheWholeFamily(t *testing.T) {
	ctx := context.Background()
	service := newService(t)
	session := register(t, service, emailFor(t))

	rotated, err := service.Refresh(ctx, session.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}
	if _, err := service.Lookup(ctx, rotated.SessionToken); err != nil {
		t.Fatalf("the rotated session should work before the reuse: %v", err)
	}

	// The attacker presents the token they stole before the legitimate refresh.
	if _, err := service.Refresh(ctx, session.RefreshToken); err == nil {
		t.Fatal("the reused token was accepted")
	}

	if _, err := service.Lookup(ctx, rotated.SessionToken); err == nil {
		t.Error("the current session still works after a reuse was detected, so the family was not revoked")
	}
	if _, err := service.Refresh(ctx, rotated.RefreshToken); err == nil {
		t.Error("the current refresh token still works after a reuse was detected")
	}
}

// Revoking one family must not touch another. A person logged in on a phone and
// a laptop loses only the compromised one.
func TestRevokingOneFamilyLeavesAnotherAlone(t *testing.T) {
	ctx := context.Background()
	service := newService(t)
	email := emailFor(t)
	first := register(t, service, email)

	second, err := service.Authenticate(ctx, email, goodPassword)
	if err != nil {
		t.Fatalf("second Authenticate returned error: %v", err)
	}

	if _, err := service.Refresh(ctx, first.RefreshToken); err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}
	if _, err := service.Refresh(ctx, first.RefreshToken); err == nil {
		t.Fatal("the reused token was accepted")
	}

	if _, err := service.Lookup(ctx, second.SessionToken); err != nil {
		t.Errorf("the other session was revoked along with the compromised one: %v", err)
	}
}

func TestRefreshRefusesAnUnknownToken(t *testing.T) {
	ctx := context.Background()
	service := newService(t)

	if _, err := service.Refresh(ctx, "ref_never-issued"); err == nil {
		t.Error("Refresh accepted a token that was never issued")
	}
}

// ───────────────────────────────────────────────────────────────── helpers

func register(t *testing.T, service *identity.Service, email string) identity.Session {
	t.Helper()
	ctx := context.Background()

	if _, err := service.Register(ctx, identity.RegisterInput{
		Email: email, Password: goodPassword, AccountType: identity.AccountCandidate,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	session, err := service.Authenticate(ctx, email, goodPassword)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	return session
}

// seedUserWithHash inserts a user with a chosen password hash, which is how a
// hash under old parameters is created without waiting for the parameters to
// change.
func seedUserWithHash(t *testing.T, email, hash string) string {
	t.Helper()
	ctx := context.Background()

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	var userID string
	if err := conn.QueryRow(ctx,
		`INSERT INTO identity.users (id, email, email_verified)
		 VALUES (gen_random_uuid(), $1, true) RETURNING id`, email).Scan(&userID); err != nil {
		t.Fatalf("seeding user: %v", err)
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO identity.credentials (user_id, password_hash) VALUES ($1, $2)`,
		userID, hash); err != nil {
		t.Fatalf("seeding credentials: %v", err)
	}
	return userID
}

func storedHash(t *testing.T, userID string) string {
	t.Helper()
	ctx := context.Background()

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	var hash string
	if err := conn.QueryRow(ctx,
		`SELECT password_hash FROM identity.credentials WHERE user_id = $1`, userID).Scan(&hash); err != nil {
		t.Fatalf("reading credentials: %v", err)
	}
	return hash
}

// ───────────────────────────────────────────────────────────────── describe

// Describe backs GET /me. It reads by user id rather than by session token,
// because the session lookup has already established who is acting and
// re-deriving it from the token would be a second chance to get it wrong.
func TestDescribeReturnsTheUser(t *testing.T) {
	ctx := context.Background()
	service := newService(t)

	email := emailFor(t)
	outcome, err := service.Register(ctx, identity.RegisterInput{
		Email: email, Password: goodPassword, AccountType: identity.AccountCandidate,
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	user, err := service.Describe(ctx, outcome.UserID)
	if err != nil {
		t.Fatalf("Describe returned error: %v", err)
	}
	if user.ID != outcome.UserID {
		t.Errorf("ID = %q, want %q", user.ID, outcome.UserID)
	}
	if !strings.EqualFold(user.Email, email) {
		t.Errorf("Email = %q, want %q", user.Email, email)
	}
	// A newly registered address is not verified. Reporting otherwise would let
	// the interface offer actions that verification is supposed to gate.
	if user.EmailVerified {
		t.Error("a newly registered user is reported as verified")
	}
}

// An unknown id is ErrNotFound rather than an empty user, because an empty user
// has an empty id, and a caller that forgot to check the error would then act
// as a user who does not exist.
func TestDescribeReportsNotFoundForAnUnknownUser(t *testing.T) {
	ctx := context.Background()
	service := newService(t)

	if _, err := service.Describe(ctx, "01a0301d-aa10-7000-8f3e-000000000000"); !errors.Is(err, identity.ErrNotFound) {
		t.Errorf("Describe of an unknown user returned %v, want ErrNotFound", err)
	}
}

// Timestamps leave this system in a browser, a log line and a database column,
// and are compared across processes. A service wired with a local clock would
// emit offsets that differ from everything else, so the normalisation is at the
// single point of use rather than in the wiring.
func TestTimestampsAreUTCWhateverClockIsInjected(t *testing.T) {
	ctx := context.Background()

	sydney, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		t.Skipf("timezone data unavailable: %v", err)
	}

	// Deliberately not UTC, standing in for a process running with a local
	// timezone set.
	service := identity.NewService(identity.NewRepository(pool), func() time.Time {
		return time.Now().In(sydney)
	})

	email := emailFor(t)
	if _, err := service.Register(ctx, identity.RegisterInput{
		Email: email, Password: goodPassword, AccountType: identity.AccountCandidate,
	}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	session, err := service.Authenticate(ctx, email, goodPassword)
	if err != nil {
		t.Fatalf("Authenticate returned error: %v", err)
	}

	for name, stamp := range map[string]time.Time{
		"ExpiresAt":       session.ExpiresAt,
		"RefreshExpires":  session.RefreshExpires,
		"AuthenticatedAt": session.AuthenticatedAt,
	} {
		if _, offset := stamp.Zone(); offset != 0 {
			t.Errorf("%s = %s, which carries a %d second offset rather than being UTC",
				name, stamp.Format(time.RFC3339), offset)
		}
	}
}
