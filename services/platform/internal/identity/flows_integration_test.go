//go:build integration

package identity_test

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Yelethe1st/prepeet/services/platform/internal/identity"
	"github.com/Yelethe1st/prepeet/services/platform/internal/notification"
	"github.com/Yelethe1st/prepeet/services/platform/platform/ratelimit"
)

// IAM-02's flows against real PostgreSQL and the real email queue.
//
// The mailer here is the same adapter shape cmd wires, pointed at the real
// notification queue, so what is asserted is what the person experiences: the
// row that will become their email, carrying the link that will actually
// work. A fake mailer would prove the service called a method, which is a
// different and lesser fact.

// queueMailer adapts notification.Queue to identity.Mailer, as cmd does.
type queueMailer struct{ queue *notification.Queue }

func (m queueMailer) SendEmailVerification(ctx context.Context, tx pgx.Tx, recipient, link string, expires int) error {
	_, err := m.queue.Enqueue(ctx, tx, recipient, notification.VerifyEmail{Link: link, ExpiresMinutes: expires})
	return err
}

func (m queueMailer) SendPasswordReset(ctx context.Context, tx pgx.Tx, recipient, link string, expires int) error {
	_, err := m.queue.Enqueue(ctx, tx, recipient, notification.PasswordReset{Link: link, ExpiresMinutes: expires})
	return err
}

func (m queueMailer) SendMagicLink(ctx context.Context, tx pgx.Tx, recipient, link string, expires int) error {
	_, err := m.queue.Enqueue(ctx, tx, recipient, notification.MagicLink{Link: link, ExpiresMinutes: expires})
	return err
}

func (m queueMailer) SendOTP(ctx context.Context, tx pgx.Tx, recipient, code string, expires int) error {
	_, err := m.queue.Enqueue(ctx, tx, recipient, notification.OTP{Code: code, ExpiresMinutes: expires})
	return err
}

// flowService builds a service with flows enabled and a generous cooldown, so
// only the tests about the cooldown exercise the cooldown.
func flowService(t *testing.T, clock func() time.Time) *identity.Service {
	t.Helper()
	return identity.NewService(identity.NewRepository(pool), clock).WithTokenFlows(identity.TokenFlows{
		Mailer:  queueMailer{queue: notification.NewQueue(pool)},
		Resend:  ratelimit.NewMemory(ratelimit.Rule{Limit: 100, Window: time.Minute}, clock),
		BaseURL: "https://app.example.test",
	})
}

// tokenFromEmail reads the newest queued email for an address and extracts
// the secret, exactly as the person would.
func tokenFromEmail(t *testing.T, recipient, kind string) string {
	t.Helper()

	var body string
	err := pool.QueryRow(context.Background(), `
		SELECT body FROM notification.emails
		WHERE recipient = $1 AND sent_at IS NULL
		ORDER BY id DESC LIMIT 1`, recipient).Scan(&body)
	if err != nil {
		t.Fatalf("no email was queued for %s: %v", recipient, err)
	}

	if kind == "code" {
		match := regexp.MustCompile(`\b\d{6}\b`).FindString(body)
		if match == "" {
			t.Fatalf("the email carries no six-digit code:\n%s", body)
		}
		return match
	}
	match := regexp.MustCompile(`token=([A-Za-z0-9_-]+)`).FindStringSubmatch(body)
	if match == nil {
		t.Fatalf("the email carries no token link:\n%s", body)
	}
	return match[1]
}

// registerFlow creates an account and returns its address.
func registerFlow(t *testing.T, service *identity.Service) string {
	t.Helper()
	email := emailFor(t)
	if _, err := service.Register(context.Background(), identity.RegisterInput{
		Email: email, Password: goodPassword, AccountType: identity.AccountCandidate,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	return email
}

// ─────────────────────────────────────────────────────── email verification

func TestRegistrationQueuesAVerificationEmailAndTheLinkVerifies(t *testing.T) {
	ctx := context.Background()
	service := flowService(t, time.Now)
	email := registerFlow(t, service)

	plaintext := tokenFromEmail(t, email, "link")
	if err := service.ConfirmEmailVerification(ctx, plaintext); err != nil {
		t.Fatalf("the link from the email did not verify: %v", err)
	}

	var verified bool
	if err := pool.QueryRow(ctx,
		"SELECT email_verified FROM identity.users WHERE email = $1", email).Scan(&verified); err != nil {
		t.Fatalf("reading the user: %v", err)
	}
	if !verified {
		t.Fatal("the address is still unverified after its link was accepted")
	}
}

func TestAVerificationLinkWorksExactlyOnce(t *testing.T) {
	ctx := context.Background()
	service := flowService(t, time.Now)
	email := registerFlow(t, service)
	plaintext := tokenFromEmail(t, email, "link")

	if err := service.ConfirmEmailVerification(ctx, plaintext); err != nil {
		t.Fatalf("first use: %v", err)
	}
	if err := service.ConfirmEmailVerification(ctx, plaintext); !errors.Is(err, identity.ErrTokenUsed) {
		t.Fatalf("second use = %v, want ErrTokenUsed: its own outcome, not a generic failure", err)
	}
}

func TestAnExpiredLinkSaysExpired(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	clock := &movableClock{t: now}
	service := flowService(t, clock.Now)
	email := registerFlow(t, service)
	plaintext := tokenFromEmail(t, email, "link")

	clock.t = now.Add(31 * time.Minute)
	if err := service.ConfirmEmailVerification(ctx, plaintext); !errors.Is(err, identity.ErrTokenExpired) {
		t.Fatalf("after expiry = %v, want ErrTokenExpired", err)
	}
}

func TestANewRequestSupersedesTheOldLink(t *testing.T) {
	ctx := context.Background()
	service := flowService(t, time.Now)
	email := registerFlow(t, service)
	first := tokenFromEmail(t, email, "link")

	if err := service.RequestEmailVerification(ctx, email); err != nil {
		t.Fatalf("resend: %v", err)
	}
	second := tokenFromEmail(t, email, "link")
	if first == second {
		t.Fatal("the resend re-sent the same token")
	}

	// The prototype promises only the newest email works, and that the person
	// holding the old one is told why rather than told "invalid".
	if err := service.ConfirmEmailVerification(ctx, first); !errors.Is(err, identity.ErrTokenSuperseded) {
		t.Fatalf("old link = %v, want ErrTokenSuperseded", err)
	}
	if err := service.ConfirmEmailVerification(ctx, second); err != nil {
		t.Fatalf("newest link: %v", err)
	}
}

func TestAGarbageTokenIsInvalidNotAnError(t *testing.T) {
	service := flowService(t, time.Now)
	if err := service.ConfirmEmailVerification(context.Background(), "vrf_notatoken"); !errors.Is(err, identity.ErrTokenInvalid) {
		t.Fatalf("garbage = %v, want ErrTokenInvalid", err)
	}
}

func TestATokenForOneFlowIsRefusedByAnother(t *testing.T) {
	ctx := context.Background()
	service := flowService(t, time.Now)
	email := registerFlow(t, service)
	verification := tokenFromEmail(t, email, "link")

	// A verification token pasted into the reset endpoint must not reset
	// anything: purpose is part of the token's identity.
	if err := service.ConfirmPasswordReset(ctx, verification, "an entirely new password"); !errors.Is(err, identity.ErrTokenInvalid) {
		t.Fatalf("cross-purpose = %v, want ErrTokenInvalid", err)
	}
}

// ─────────────────────────────────────────────────────── password recovery

func TestRecoveryChangesThePasswordAndRevokesEverySession(t *testing.T) {
	ctx := context.Background()
	service := flowService(t, time.Now)
	email := registerFlow(t, service)

	// A live session that predates the reset, standing in for the attacker
	// whose knowledge of the old password is the reason resets exist.
	session, err := service.Authenticate(ctx, email, goodPassword)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	if err := service.RequestPasswordReset(ctx, email); err != nil {
		t.Fatalf("request: %v", err)
	}
	link := tokenFromEmail(t, email, "link")

	const newPassword = "completely different passphrase"
	if err := service.ConfirmPasswordReset(ctx, link, newPassword); err != nil {
		t.Fatalf("reset: %v", err)
	}

	if _, err := service.Authenticate(ctx, email, goodPassword); !errors.Is(err, identity.ErrCredentialsInvalid) {
		t.Error("the old password still signs in")
	}
	if _, err := service.Authenticate(ctx, email, newPassword); err != nil {
		t.Errorf("the new password does not sign in: %v", err)
	}
	if _, err := service.Lookup(ctx, session.SessionToken); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Error("a session from before the reset still works; the attacker keeps their foothold")
	}
}

func TestAnUnknownAddressGetsTheSameAnswerAsAKnownOne(t *testing.T) {
	ctx := context.Background()
	service := flowService(t, time.Now)

	unknown := "nobody-" + emailFor(t)
	if err := service.RequestPasswordReset(ctx, unknown); err != nil {
		t.Fatalf("unknown address = %v, want nil: the response must not say which addresses exist", err)
	}

	var queued int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM notification.emails WHERE recipient = $1", unknown).Scan(&queued); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if queued != 0 {
		t.Fatal("an email was queued for an address with no account")
	}
}

// ─────────────────────────────────────────────────────────── resend cooldown

func TestResendIsRateLimitedWithAVisibleCooldown(t *testing.T) {
	ctx := context.Background()
	clock := &movableClock{t: time.Now()}
	service := identity.NewService(identity.NewRepository(pool), clock.Now).WithTokenFlows(identity.TokenFlows{
		Mailer:  queueMailer{queue: notification.NewQueue(pool)},
		Resend:  ratelimit.NewMemory(ratelimit.Rule{Limit: 1, Window: time.Minute}, clock.Now),
		BaseURL: "https://app.example.test",
	})
	email := registerFlow(t, service)

	// Registration spent the window's one send; the immediate retry cools down.
	var cooldown *identity.CooldownError
	err := service.RequestEmailVerification(ctx, email)
	if !errors.As(err, &cooldown) {
		t.Fatalf("immediate resend = %v, want a CooldownError", err)
	}
	if cooldown.RetryAfter <= 0 {
		t.Fatal("the cooldown carries no duration, so the interface has no countdown to show")
	}

	clock.t = clock.t.Add(61 * time.Second)
	if err := service.RequestEmailVerification(ctx, email); err != nil {
		t.Fatalf("after the cooldown = %v, want nil", err)
	}
}

func TestTheCooldownDoesNotRevealWhetherAnAddressExists(t *testing.T) {
	// If only known addresses cooled down, the cooldown would be the oracle
	// the identical responses exist to prevent.
	ctx := context.Background()
	service := identity.NewService(identity.NewRepository(pool), time.Now).WithTokenFlows(identity.TokenFlows{
		Mailer:  queueMailer{queue: notification.NewQueue(pool)},
		Resend:  ratelimit.NewMemory(ratelimit.Rule{Limit: 1, Window: time.Minute}, time.Now),
		BaseURL: "https://app.example.test",
	})

	unknown := "ghost-" + emailFor(t)
	if err := service.RequestPasswordReset(ctx, unknown); err != nil {
		t.Fatalf("first: %v", err)
	}
	var cooldown *identity.CooldownError
	if err := service.RequestPasswordReset(ctx, unknown); !errors.As(err, &cooldown) {
		t.Fatalf("an unknown address does not cool down, which distinguishes it from a known one: %v", err)
	}
}

// ───────────────────────────────────────────────────────────── magic link

func TestAMagicLinkSignsInAndVerifiesTheAddress(t *testing.T) {
	ctx := context.Background()
	service := flowService(t, time.Now)
	email := registerFlow(t, service)

	if err := service.RequestMagicLink(ctx, email); err != nil {
		t.Fatalf("request: %v", err)
	}
	link := tokenFromEmail(t, email, "link")

	session, err := service.ConsumeMagicLink(ctx, link)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if _, err := service.Lookup(ctx, session.SessionToken); err != nil {
		t.Fatalf("the issued session does not look up: %v", err)
	}

	var verified bool
	if err := pool.QueryRow(ctx,
		"SELECT email_verified FROM identity.users WHERE email = $1", email).Scan(&verified); err != nil {
		t.Fatalf("reading: %v", err)
	}
	if !verified {
		t.Error("arriving by emailed link proves control of the address, and it was not recorded")
	}
}

func TestAMagicLinkIsSingleUse(t *testing.T) {
	ctx := context.Background()
	service := flowService(t, time.Now)
	email := registerFlow(t, service)

	if err := service.RequestMagicLink(ctx, email); err != nil {
		t.Fatalf("request: %v", err)
	}
	link := tokenFromEmail(t, email, "link")

	if _, err := service.ConsumeMagicLink(ctx, link); err != nil {
		t.Fatalf("first use: %v", err)
	}
	if _, err := service.ConsumeMagicLink(ctx, link); !errors.Is(err, identity.ErrTokenUsed) {
		t.Fatalf("second use = %v, want ErrTokenUsed: a replayed sign-in link must not mint a second session", err)
	}
}

// ───────────────────────────────────────────────────────────── one-time code

func TestAnOTPSignsIn(t *testing.T) {
	ctx := context.Background()
	service := flowService(t, time.Now)
	email := registerFlow(t, service)

	if err := service.RequestOTP(ctx, email); err != nil {
		t.Fatalf("request: %v", err)
	}
	code := tokenFromEmail(t, email, "code")

	session, err := service.ConfirmOTP(ctx, email, code)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if session.SessionToken == "" {
		t.Fatal("no session was issued")
	}

	if _, err := service.ConfirmOTP(ctx, email, code); !errors.Is(err, identity.ErrCodeIncorrect) {
		// Single use: once consumed there is no live code, and the absence
		// answers exactly as a wrong guess does.
		t.Fatalf("replayed code = %v, want ErrCodeIncorrect", err)
	}
}

func TestFiveWrongGuessesKillTheCode(t *testing.T) {
	ctx := context.Background()
	service := flowService(t, time.Now)
	email := registerFlow(t, service)

	if err := service.RequestOTP(ctx, email); err != nil {
		t.Fatalf("request: %v", err)
	}
	code := tokenFromEmail(t, email, "code")

	wrong := "000000"
	if wrong == code {
		wrong = "000001"
	}

	var last error
	for range 5 {
		_, last = service.ConfirmOTP(ctx, email, wrong)
	}
	if !errors.Is(last, identity.ErrTooManyAttempts) {
		t.Fatalf("fifth wrong guess = %v, want ErrTooManyAttempts", last)
	}

	// The right code arriving after the cap must not work: five wrong
	// neighbours is the signature of guessing, not of typing.
	if _, err := service.ConfirmOTP(ctx, email, code); err == nil {
		t.Fatal("the code still works after being guessed at")
	}
}

// movableClock lets a test travel to a token's expiry instead of waiting.
type movableClock struct{ t time.Time }

func (c *movableClock) Now() time.Time { return c.t }

// The email content and the store must agree about lifetime: the email states
// the expiry the person will actually be held to.
func TestTheEmailStatesTheLifetimeTheStoreEnforces(t *testing.T) {
	ctx := context.Background()
	service := flowService(t, time.Now)
	email := registerFlow(t, service)

	var body string
	if err := pool.QueryRow(ctx, `
		SELECT body FROM notification.emails WHERE recipient = $1 ORDER BY id DESC LIMIT 1`,
		email).Scan(&body); err != nil {
		t.Fatalf("reading the email: %v", err)
	}
	if !strings.Contains(body, "30 minutes") {
		t.Fatalf("the verification email does not state the 30-minute lifetime:\n%s", body)
	}
}

// The race guard, tested at the layer it lives in.
//
// The service's replay answer comes from reading used_at, so a broken guard
// on the consume statement is invisible to every sequential test: the read
// catches the replay first. The guard exists for two concurrent presentations
// that both passed the read, and the only way to test it is to call the
// consume twice directly, where the second call must lose on the row update
// itself. Removing the guard columns from MarkActionTokenUsed fails this test
// and no other, which was discovered by removing them.
func TestTwoConcurrentConsumesOfOneTokenHaveOneWinner(t *testing.T) {
	ctx := context.Background()
	service := flowService(t, time.Now)
	email := registerFlow(t, service)
	_ = tokenFromEmail(t, email, "link")

	repo := identity.NewRepository(pool)

	var tokenID, userID string
	if err := pool.QueryRow(ctx, `
		SELECT t.id::text, t.user_id::text FROM identity.action_tokens t
		JOIN identity.users u ON u.id = t.user_id
		WHERE u.email = $1 ORDER BY t.created_at DESC LIMIT 1`, email).
		Scan(&tokenID, &userID); err != nil {
		t.Fatalf("reading the token row: %v", err)
	}

	first, err := repo.ConsumeForEmailVerification(ctx, tokenID, userID)
	if err != nil {
		t.Fatalf("first consume: %v", err)
	}
	second, err := repo.ConsumeForEmailVerification(ctx, tokenID, userID)
	if err != nil {
		t.Fatalf("second consume: %v", err)
	}

	if !first || second {
		t.Fatalf("first=%v second=%v; exactly one presentation may win", first, second)
	}
}

// ─────────────────────────────────────────────── invitation candidate provisioning

// A candidate the invitation was sent to who has no account gets one: a
// passwordless, verified account, and a working session, so acceptance can sign
// them in on the strength of the token that reached them.
func TestProvisionCandidateSessionCreatesANewCandidate(t *testing.T) {
	ctx := context.Background()
	service := flowService(t, time.Now)
	email := emailFor(t)

	userID, session, err := service.ProvisionCandidateSession(ctx, email)
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if userID == "" {
		t.Fatal("no candidate id returned")
	}
	if _, err := service.Lookup(ctx, session.SessionToken); err != nil {
		t.Fatalf("the issued session does not look up: %v", err)
	}

	var verified bool
	var hash string
	if err := pool.QueryRow(ctx,
		`SELECT u.email_verified, coalesce(c.password_hash, '')
		 FROM identity.users u JOIN identity.credentials c ON c.user_id = u.id
		 WHERE u.id = $1`, userID).Scan(&verified, &hash); err != nil {
		t.Fatalf("reading the provisioned candidate: %v", err)
	}
	if !verified {
		t.Error("arriving with an emailed token proves control of the address; it was not marked verified")
	}
	if hash != "" {
		t.Error("a candidate provisioned from an invitation must have no password")
	}
}

// An address that already has an account resolves to that same account rather
// than a second one, and its password is left untouched. This is the no-leak
// property: acceptance returns a session either way, so it cannot be used to
// learn whether the address was already registered.
func TestProvisionCandidateSessionResolvesAnExistingAccount(t *testing.T) {
	ctx := context.Background()
	service := flowService(t, time.Now)

	email := emailFor(t)
	registered, err := service.Register(ctx, identity.RegisterInput{
		Email: email, Password: goodPassword, AccountType: identity.AccountCandidate,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	userID, session, err := service.ProvisionCandidateSession(ctx, email)
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if userID != registered.UserID {
		t.Fatalf("provision made a new account %q for an existing address %q", userID, registered.UserID)
	}
	if _, err := service.Lookup(ctx, session.SessionToken); err != nil {
		t.Fatalf("the issued session does not look up: %v", err)
	}

	// The existing password is intact: provisioning resolved, it did not
	// overwrite. The original credentials still authenticate.
	if _, err := service.Authenticate(ctx, email, goodPassword); err != nil {
		t.Fatalf("provisioning clobbered an existing account's password: %v", err)
	}
}

// Provisioning the same new address twice resolves to one account, so a
// candidate who clicks the link twice does not end up with two.
func TestProvisionCandidateSessionIsIdempotent(t *testing.T) {
	ctx := context.Background()
	service := flowService(t, time.Now)
	email := emailFor(t)

	first, _, err := service.ProvisionCandidateSession(ctx, email)
	if err != nil {
		t.Fatalf("first provision: %v", err)
	}
	second, _, err := service.ProvisionCandidateSession(ctx, email)
	if err != nil {
		t.Fatalf("second provision: %v", err)
	}
	if first != second {
		t.Fatalf("provisioning one address made two accounts: %q and %q", first, second)
	}
}
