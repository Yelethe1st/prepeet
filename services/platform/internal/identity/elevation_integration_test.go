//go:build integration

package identity_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Yelethe1st/prepeet/services/platform/internal/identity"
)

// IAM-07 against real PostgreSQL: the grant's requirements, its automatic
// death, its revocation, its visibility, and the per-request audit that
// records access whether or not anything was read.

// operator registers a fresh account and returns its id and a live session.
func operator(t *testing.T, service *identity.Service) (userID, sessionToken string) {
	t.Helper()
	ctx := context.Background()
	email := emailFor(t)

	if _, err := service.Register(ctx, identity.RegisterInput{
		Email: email, Password: goodPassword, AccountType: identity.AccountCandidate,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	session, err := service.Authenticate(ctx, email, goodPassword)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	return session.UserID, session.SessionToken
}

func TestAnElevationNeedsAReasonAndATicket(t *testing.T) {
	ctx := context.Background()
	service := newService(t)
	userID, _ := operator(t, service)

	if _, err := service.Elevate(ctx, userID, "", "INC-401", 0); !errors.Is(err, identity.ErrElevationReason) {
		t.Fatalf("no reason = %v, want ErrElevationReason", err)
	}
	if _, err := service.Elevate(ctx, userID, "incident 401 triage", "", 0); !errors.Is(err, identity.ErrElevationTicket) {
		t.Fatalf("no ticket = %v, want ErrElevationTicket", err)
	}

	// And the cap refuses rather than clamps: an operator silently granted
	// less time than they asked for believes they hold time they do not.
	_, err := service.Elevate(ctx, userID, "reason", "INC-401", 2*time.Hour)
	if err == nil || !strings.Contains(err.Error(), "ELEVATION_TOO_LONG") {
		t.Fatalf("over the cap = %v, want ELEVATION_TOO_LONG", err)
	}
}

func TestAnElevationExpiresByItself(t *testing.T) {
	ctx := context.Background()
	service := newService(t)
	userID, _ := operator(t, service)

	if _, err := service.Elevate(ctx, userID, "incident triage", "INC-402", 10*time.Minute); err != nil {
		t.Fatalf("elevate: %v", err)
	}

	active, err := service.ActiveElevations(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !containsGrantFor(active, userID) {
		t.Fatal("the fresh grant is not visible")
	}

	// No revocation, no job: liveness is the timestamp comparison in the
	// query, so ageing the grant directly - rather than sleeping through a
	// real minute - is exercising exactly the mechanism under test.
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx,
		`UPDATE identity.elevations
		 SET granted_at = now() - interval '2 hours',
		     expires_at = now() - interval '1 second'
		 WHERE user_id = $1`, userID); err != nil {
		t.Fatalf("ageing: %v", err)
	}

	active, err = service.ActiveElevations(ctx)
	if err != nil {
		t.Fatalf("list after expiry: %v", err)
	}
	if containsGrantFor(active, userID) {
		t.Fatal("an expired grant is still listed as active")
	}
}

func TestRevocationEndsAGrantImmediately(t *testing.T) {
	ctx := context.Background()
	service := newService(t)
	userID, _ := operator(t, service)

	grant, err := service.Elevate(ctx, userID, "incident triage", "INC-403", 30*time.Minute)
	if err != nil {
		t.Fatalf("elevate: %v", err)
	}
	if err := service.RevokeElevation(ctx, grant.GrantID, userID); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	active, err := service.ActiveElevations(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if containsGrantFor(active, userID) {
		t.Fatal("a revoked grant is still active")
	}

	// Revoking the corpse is its own distinct answer.
	if err := service.RevokeElevation(ctx, grant.GrantID, userID); !errors.Is(err, identity.ErrElevationGone) {
		t.Fatalf("second revoke = %v, want ErrElevationGone", err)
	}
}

func TestTheActiveListNamesTheOperatorForTheirTeam(t *testing.T) {
	ctx := context.Background()
	service := newService(t)
	userID, _ := operator(t, service)

	if _, err := service.Elevate(ctx, userID, "quota investigation", "INC-404", 15*time.Minute); err != nil {
		t.Fatalf("elevate: %v", err)
	}

	active, err := service.ActiveElevations(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, grant := range active {
		if grant.UserID != userID {
			continue
		}
		if grant.Email == "" || grant.Reason != "quota investigation" || grant.Ticket != "INC-404" {
			t.Fatalf("the visible grant is missing what a teammate reads: %+v", grant)
		}
		return
	}
	t.Fatal("the grant is not in the active list")
}

func TestEveryRequestUnderElevationIsAudited(t *testing.T) {
	// The second criterion, at the choke point: two authenticated requests
	// under an active grant are two audit rows; after revocation, none.
	ctx := context.Background()
	service := newService(t)
	userID, sessionToken := operator(t, service)

	countAudits := func() int {
		conn, err := pgx.Connect(ctx, adminURL)
		if err != nil {
			t.Fatalf("connect: %v", err)
		}
		defer conn.Close(ctx)
		var count int
		if err := conn.QueryRow(ctx, `
			SELECT count(*) FROM audit.events
			WHERE action = 'identity.elevated_request' AND actor_id = $1`, userID).Scan(&count); err != nil {
			t.Fatalf("counting: %v", err)
		}
		return count
	}

	// Before elevation: requests leave no elevated-request rows.
	if _, err := service.Lookup(ctx, sessionToken); err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got := countAudits(); got != 0 {
		t.Fatalf("%d elevated-request rows before any elevation", got)
	}

	grant, err := service.Elevate(ctx, userID, "incident triage", "INC-405", 30*time.Minute)
	if err != nil {
		t.Fatalf("elevate: %v", err)
	}

	for range 2 {
		if _, err := service.Lookup(ctx, sessionToken); err != nil {
			t.Fatalf("elevated lookup: %v", err)
		}
	}
	if got := countAudits(); got != 2 {
		t.Fatalf("two requests under elevation left %d audit rows, want 2", got)
	}

	// And the rows name the grant, so "what did this elevation touch" is
	// answerable grant by grant.
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)
	var grants int
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM audit.events
		WHERE action = 'identity.elevated_request' AND subject_id = $1`, grant.GrantID).Scan(&grants); err != nil {
		t.Fatalf("counting by grant: %v", err)
	}
	if grants != 2 {
		t.Fatalf("the audit rows name the grant %d times, want 2", grants)
	}

	if err := service.RevokeElevation(ctx, grant.GrantID, userID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := service.Lookup(ctx, sessionToken); err != nil {
		t.Fatalf("post-revocation lookup: %v", err)
	}
	if got := countAudits(); got != 2 {
		t.Fatalf("a request after revocation was still audited as elevated: %d rows", got)
	}
}

func TestGrantAndRevocationAreThemselvesAudited(t *testing.T) {
	ctx := context.Background()
	service := newService(t)
	userID, _ := operator(t, service)

	grant, err := service.Elevate(ctx, userID, "why it happened", "INC-406", 20*time.Minute)
	if err != nil {
		t.Fatalf("elevate: %v", err)
	}
	if err := service.RevokeElevation(ctx, grant.GrantID, userID); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	var reason string
	if err := conn.QueryRow(ctx, `
		SELECT detail->>'reason' FROM audit.events
		WHERE action = 'identity.elevation_granted' AND subject_id = $1`, grant.GrantID).Scan(&reason); err != nil {
		t.Fatalf("the grant left no audit row: %v", err)
	}
	if reason != "why it happened" {
		t.Fatalf("the audit detail carries reason %q", reason)
	}

	var revocations int
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM audit.events
		WHERE action = 'identity.elevation_revoked' AND subject_id = $1`, grant.GrantID).Scan(&revocations); err != nil {
		t.Fatalf("counting revocations: %v", err)
	}
	if revocations != 1 {
		t.Fatalf("%d revocation audit rows, want 1", revocations)
	}
}

func containsGrantFor(active []identity.Elevation, userID string) bool {
	for _, grant := range active {
		if grant.UserID == userID {
			return true
		}
	}
	return false
}
