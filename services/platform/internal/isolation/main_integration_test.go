//go:build integration

// The world the attacks are run against.
//
// Two real workspaces, built through the product's own flows rather than by
// INSERT, because an attack against a fixture proves the fixture. Northwind
// and Orbital each register an organisation, each owner signs in and selects
// their workspace, and Orbital invites a recruiter, which is the row every
// cross-tenant attempt in this package is aimed at.
//
// The target is deliberately not an owner. Member administration refuses to
// touch an owner row whatever tenant is asking, so an attack aimed at one
// would be refused for a reason that has nothing to do with tenancy, and would
// pass just as well with every isolation guard removed. That is the shape of
// mistake this suite exists to avoid making.
package isolation_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/internal/identity"
	"github.com/Yelethe1st/prepeet/services/platform/platform/authz"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
)

var (
	// adminURL owns the schemas. Used to migrate, and to observe what a
	// tenant-scoped connection is forbidden to see: an assertion that a row
	// under the other tenant is still intact cannot be made from inside the
	// policy that hides it.
	adminURL  string
	adminPool *pgxpool.Pool
	// appPool connects as prepeet_app, the role the api and worker use, which
	// cannot bypass row-level security.
	appPool *pgxpool.Pool

	service *identity.Service
	members *identity.Members
	server  http.Handler

	northwind workspace
	orbital   workspace
)

// workspace is one tenant with its owner signed in and acting under it.
type workspace struct {
	tenantID     string
	ownerID      string
	ownerEmail   string
	sessionToken string
}

const password = "correct horse battery staple"

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("prepeet"),
		tcpostgres.WithUsername("prepeet_migrator"),
		tcpostgres.WithPassword("migrator-password"),
		// Not ForListeningPort, and the second occurrence rather than the
		// first, for the reasons written out in platform/database: PostgreSQL
		// accepts connections before it answers them, and the official image
		// logs readiness once for the temporary server that runs its
		// initialisation scripts.
		//
		// The five minutes has to be said twice, and that is not belt and
		// braces. There are two independent 60 second defaults here, and both
		// have to be raised or the ceiling stays at one minute: the log
		// strategy's own startup timeout, and the outer deadline that
		// WithWaitStrategy hard-codes around whatever it is given. The other
		// suites in this module set the first to two minutes and then wrap it
		// in WithWaitStrategy, so their stated two minutes is really one.
		//
		// It matters here more than elsewhere. On a machine already running
		// other suites' containers, one minute is not enough, and what a
		// timeout produces is the worst possible failure for this package: a
		// package that fails with no test named, which reads exactly like the
		// deliberate weakening that was being tested at the time and is not.
		// Two rounds of probes reported exactly that, and only reading the
		// container log rather than the summary told the two apart.
		//
		// Five minutes is a ceiling, not a delay: on an idle machine the
		// container is ready in seconds either way.
		testcontainers.WithWaitStrategyAndDeadline(5*time.Minute,
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Minute)),
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
	if err := database.Migrate(ctx, adminURL, database.MigrateOptions{
		AppPassword: "app-password",
	}); err != nil {
		fmt.Fprintf(os.Stderr, "migrating: %v\n", err)
		os.Exit(1)
	}

	if adminPool, err = pgxpool.New(ctx, adminURL); err != nil {
		fmt.Fprintf(os.Stderr, "admin pool: %v\n", err)
		os.Exit(1)
	}
	if appPool, err = pgxpool.New(ctx, appURL()); err != nil {
		fmt.Fprintf(os.Stderr, "app pool: %v\n", err)
		os.Exit(1)
	}

	service = identity.NewService(identity.NewRepository(appPool), time.Now)
	members = identity.NewMembers(identity.NewRepository(appPool))
	if server, err = newServer(); err != nil {
		fmt.Fprintf(os.Stderr, "building the API: %v\n", err)
		os.Exit(1)
	}
	if err := seed(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "seeding: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	appPool.Close()
	adminPool.Close()
	if err := container.Terminate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "terminating PostgreSQL: %v\n", err)
	}
	os.Exit(code)
}

// appURL rewrites the admin connection string to connect as the application
// role, which is the one whose authority these tests care about.
func appURL() string {
	cfg, err := pgx.ParseConfig(adminURL)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("postgres://prepeet_app:app-password@%s:%d/%s?sslmode=disable",
		cfg.Host, cfg.Port, cfg.Database)
}

// seed builds both workspaces through the product's flows.
func seed(ctx context.Context) error {
	var err error
	if northwind, err = register(ctx, "priya@northwind.example", "Northwind Health System"); err != nil {
		return fmt.Errorf("northwind: %w", err)
	}
	if orbital, err = register(ctx, "wren@orbital.example", "Orbital Labs"); err != nil {
		return fmt.Errorf("orbital: %w", err)
	}

	return nil
}

// register creates an organisation, signs its owner in and selects the
// workspace, which is the state every recruiter session is in.
func register(ctx context.Context, email, name string) (workspace, error) {
	outcome, err := service.Register(ctx, identity.RegisterInput{
		Email: email, Password: password,
		AccountType: identity.AccountOrganisation, OrganisationName: name,
	})
	if err != nil {
		return workspace{}, fmt.Errorf("registering: %w", err)
	}
	session, err := service.Authenticate(ctx, email, password)
	if err != nil {
		return workspace{}, fmt.Errorf("authenticating: %w", err)
	}
	if err := service.SelectTenant(ctx, session.SessionToken, outcome.TenantID); err != nil {
		return workspace{}, fmt.Errorf("selecting the workspace: %w", err)
	}
	return workspace{
		tenantID: outcome.TenantID, ownerID: outcome.UserID,
		ownerEmail: email, sessionToken: session.SessionToken,
	}, nil
}

// ─────────────────────────────────────────────────── the API under attack

// newServer builds the real HTTP surface over the real identity context.
//
// The adapters below are the ones cmd/api wires in production, narrowed to
// what this suite drives. Serving through a fake would test the fake: the
// question here is whether a request naming another workspace's identifier
// reaches that workspace's row, and only the real chain from handler to policy
// to SQL to row-level security can answer it.
func newServer() (http.Handler, error) {
	return api.NewServer(api.ServerConfig{
		Identity:    liveIdentity{service: service},
		Members:     liveMembers{members: members},
		Candidates:  undriven{},
		Documents:   undriven{},
		Catalog:     undriven{},
		Interviews:  undriven{},
		Billing:     undriven{},
		Environment: config.EnvironmentLocal,
	})
}

// undriven stands in for the ports this suite does not exercise.
//
// Every interface is embedded as a nil value, so a call this suite did not
// intend to make panics with a stack that names it rather than returning a
// plausible zero value. A stub that answers politely is how a test comes to
// pass against a surface it never reached.
type undriven struct {
	api.CandidateProfiles
	api.CandidateDocuments
	api.Catalog
	api.Interviews
	api.TenantBilling
}

// liveIdentity presents the identity context as the port the API declared,
// the way cmd/api's adapter does, for the operations these attacks travel
// through: resolving a session, deciding a capability, and choosing a
// workspace.
type liveIdentity struct {
	// The remaining operations of the port are deliberately absent. A nil
	// interface panics; an embedded fake would answer.
	api.Identity
	service *identity.Service
}

func (l liveIdentity) Lookup(ctx context.Context, sessionToken string) (api.Principal, error) {
	row, err := l.service.Lookup(ctx, sessionToken)
	if err != nil {
		return api.Principal{}, api.ErrSessionRejected
	}
	return api.Principal{
		UserID: row.UserID, SessionID: row.ID,
		AuthenticatedAt: row.AuthenticatedAt, ActiveTenantID: row.ActiveTenantID,
	}, nil
}

func (l liveIdentity) Authorize(ctx context.Context, sessionToken, capability string) (api.Principal, error) {
	row, err := l.service.Authorize(ctx, sessionToken, authz.Capability(capability))
	if err != nil {
		var forbidden *identity.ForbiddenError
		if errors.As(err, &forbidden) {
			return api.Principal{}, api.ErrForbidden
		}
		return api.Principal{}, api.ErrSessionRejected
	}
	return api.Principal{
		UserID: row.UserID, SessionID: row.ID,
		AuthenticatedAt: row.AuthenticatedAt, ActiveTenantID: row.ActiveTenantID,
	}, nil
}

func (l liveIdentity) SelectTenant(ctx context.Context, sessionToken, tenantID string) (api.Principal, error) {
	if err := l.service.SelectTenant(ctx, sessionToken, tenantID); err != nil {
		if errors.Is(err, identity.ErrNoMembership) {
			return api.Principal{}, api.ErrForbidden
		}
		return api.Principal{}, api.ErrSessionRejected
	}
	return l.Lookup(ctx, sessionToken)
}

// liveMembers presents member administration as the port, as cmd/api does.
type liveMembers struct{ members *identity.Members }

func (l liveMembers) List(ctx context.Context, tenantID string) ([]api.TenantMember, error) {
	listed, err := l.members.List(ctx, tenantID)
	if err != nil {
		return nil, translate(err)
	}
	out := make([]api.TenantMember, 0, len(listed))
	for _, member := range listed {
		out = append(out, api.TenantMember{
			MembershipID: member.MembershipID, UserID: member.UserID,
			Email: member.Email, Role: member.Role, Status: member.Status,
			Version: member.Version, CreatedAt: member.CreatedAt,
		})
	}
	return out, nil
}

func (l liveMembers) Invite(ctx context.Context, tenantID, actorID, email, role string) (api.TenantMember, error) {
	invited, err := l.members.Invite(ctx, tenantID, actorID, email, role)
	if err != nil {
		return api.TenantMember{}, translate(err)
	}
	return api.TenantMember{
		MembershipID: invited.MembershipID, UserID: invited.UserID, Email: invited.Email,
		Role: invited.Role, Status: invited.Status, Version: invited.Version,
		CreatedAt: invited.CreatedAt,
	}, nil
}

func (l liveMembers) ChangeRole(ctx context.Context, tenantID, actorID, membershipID, role string, expectedVersion int) (api.TenantMember, error) {
	changed, err := l.members.ChangeRole(ctx, tenantID, actorID, membershipID, role, expectedVersion)
	if err != nil {
		return api.TenantMember{}, translate(err)
	}
	return api.TenantMember{
		MembershipID: changed.MembershipID, UserID: changed.UserID, Email: changed.Email,
		Role: changed.Role, Status: changed.Status, Version: changed.Version,
		CreatedAt: changed.CreatedAt,
	}, nil
}

func (l liveMembers) Revoke(ctx context.Context, tenantID, actorID, membershipID string, expectedVersion int) error {
	return translate(l.members.Revoke(ctx, tenantID, actorID, membershipID, expectedVersion))
}

// translate maps the context's refusals onto the port's, as cmd/api does.
//
// The mapping that matters to this suite is the first: a membership in
// another workspace and a membership that does not exist arrive as the same
// refusal, and therefore as the same 404, because a distinguishable response
// is an oracle for which identifiers are real.
func translate(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, identity.ErrMemberNotFound):
		return api.ErrMemberMissing
	case errors.Is(err, identity.ErrMemberExists),
		errors.Is(err, identity.ErrMemberOwner),
		errors.Is(err, identity.ErrMemberStale):
		return api.ErrMemberConflict
	}
	return err
}

// request sends one request to the real handler as the given session.
//
// The session token travels in the cookie the browser would send, so the whole
// authentication path runs rather than being stepped around.
func request(t *testing.T, sessionToken, method, path string, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if sessionToken != "" {
		req.AddCookie(&http.Cookie{Name: api.SessionCookieName, Value: sessionToken})
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, req)
	return recorder
}
