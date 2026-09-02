package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
	"github.com/Yelethe1st/prepeet/services/platform/platform/health"
	"github.com/Yelethe1st/prepeet/services/platform/platform/httpserver"
)

// basePath is where the contract says this API lives.
//
// Taken from the contract's servers block rather than chosen here. ADR-0004
// puts the version in the path because a new version is a project rather than a
// release, and two places deciding the prefix is how they come to disagree.
const basePath = "/api/v1"

// ServerConfig is what the API layer needs to serve.
type ServerConfig struct {
	Identity Identity
	// Candidates serves the profile operations; owner-scoped by construction,
	// since the port takes only the session's own user.
	Candidates CandidateProfiles
	// Documents serves the CV flows, owner-scoped the same way.
	Documents CandidateDocuments
	// Catalog serves the interview catalogue from the artifact registry.
	Catalog Catalog
	// Interviews serves session creation from a validated selection.
	Interviews Interviews
	// Members serves workspace member administration.
	Members TenantMembers
	// Billing serves the usage and quota reads.
	Billing TenantBilling
	// Progression serves the candidate's own competency history.
	Progression Progression
	// SensitiveReads records access to restricted content, for the operations
	// the contract declares auditable.
	SensitiveReads SensitiveReadAuditor
	// Settings serves the workspace configuration, read only.
	Settings TenantConfiguration
	// Recruiting serves SCR-01's campaign surface.
	Recruiting Recruiting
	// Invitations serves SCR-04's invitation surface.
	Invitations Invitations
	Environment config.Environment
	// AgentToken authenticates the voice agent's internal writes. Empty
	// disables the internal operations: they answer 401 to everything.
	AgentToken string
	// AttemptsPerAddress and AttemptsPerNetwork count authentication
	// attempts (SEC-10). Nil allows everything, which is a local run and
	// never a deployment.
	AttemptsPerAddress Limiter
	AttemptsPerNetwork Limiter
	// TrustProxyHeaders says the deployment sits behind a proxy whose
	// X-Forwarded-For may be believed. False means the transport's own
	// remote address is used, which nobody can forge.
	TrustProxyHeaders bool
	// Health is consulted by the readiness probe. Optional: a nil registry
	// reports ready, which is correct for a process with no dependencies.
	Health *health.Registry
}

// NewServer builds the HTTP handler for the whole API.
//
// It returns an error rather than panicking on a missing dependency, so a
// wiring mistake is a startup failure with a message rather than a nil
// dereference on the first request that happens to need it.
func NewServer(cfg ServerConfig) (http.Handler, error) {
	if cfg.Identity == nil {
		return nil, errors.New("api: an Identity is required")
	}
	if cfg.Candidates == nil {
		return nil, errors.New("api: a CandidateProfiles is required")
	}
	if cfg.Documents == nil {
		return nil, errors.New("api: a CandidateDocuments is required")
	}
	if cfg.Catalog == nil {
		return nil, errors.New("api: a Catalog is required")
	}
	if cfg.Interviews == nil {
		return nil, errors.New("api: an Interviews is required")
	}
	if cfg.Members == nil {
		return nil, errors.New("api: a TenantMembers is required")
	}
	if cfg.Billing == nil {
		return nil, errors.New("api: a TenantBilling is required")
	}
	if cfg.Progression == nil {
		return nil, errors.New("api: a Progression is required")
	}
	if cfg.Settings == nil {
		return nil, errors.New("api: a TenantConfiguration is required")
	}
	if cfg.Recruiting == nil {
		return nil, errors.New("api: a Recruiting is required")
	}
	if cfg.Invitations == nil {
		return nil, errors.New("api: an Invitations is required")
	}

	handlers := &server{
		authentication: authentication{
			identity:    cfg.Identity,
			environment: cfg.Environment,
			limits: limits{
				perAddress: cfg.AttemptsPerAddress,
				perNetwork: cfg.AttemptsPerNetwork,
			},
		},
		health: cfg.Health,
	}
	handlers.profile = profile{
		authentication: &handlers.authentication,
		candidates:     cfg.Candidates,
	}
	handlers.documents = documents{
		authentication: &handlers.authentication,
		flows:          cfg.Documents,
	}
	handlers.catalog = catalog{
		authentication: &handlers.authentication,
		source:         cfg.Catalog,
	}
	handlers.interviews = interviews{
		agentToken:     cfg.AgentToken,
		authentication: &handlers.authentication,
		flows:          cfg.Interviews,
	}
	handlers.members = members{
		authentication: &handlers.authentication,
		flows:          cfg.Members,
	}
	handlers.billingHandlers = billingHandlers{
		authentication: &handlers.authentication,
		ledger:         cfg.Billing,
	}
	handlers.progression = progression{
		authentication: &handlers.authentication,
		history:        cfg.Progression,
	}
	handlers.settingsHandlers = settingsHandlers{
		authentication: &handlers.authentication,
		settings:       cfg.Settings,
	}
	handlers.campaignHandlers = campaignHandlers{
		authentication: &handlers.authentication,
		campaigns:      cfg.Recruiting,
	}
	handlers.invitationHandlers = invitationHandlers{
		authentication: &handlers.authentication,
		campaigns:      cfg.Recruiting,
		invitations:    cfg.Invitations,
	}

	// What each operation requires, read from the contract rather than named
	// in each handler. A document the server cannot read is a wiring failure
	// worth refusing to start for: every authorization decision below depends
	// on it, and starting without it would mean serving with the doors open.
	required, err := requiredCapabilities()
	if err != nil {
		return nil, err
	}

	// Which reads are events. Refused at construction rather than at the first
	// read, for the same reason: a contract declaring an audit the deployment
	// cannot write is a promise nothing keeps.
	auditable, err := sensitiveOperations()
	if err != nil {
		return nil, err
	}
	if len(auditable) > 0 && cfg.SensitiveReads == nil {
		return nil, fmt.Errorf(
			"api: the contract declares %d sensitive read(s) and no SensitiveReadAuditor was given",
			len(auditable))
	}

	strict := prepeetapi.NewStrictHandlerWithOptions(handlers,
		// Order matters and reads backwards. The generated router wraps in slice
		// order, so the last entry is outermost and runs first. The audit is
		// listed first, which makes it the inner one, so credentials are in the
		// context by the time it needs to name who read.
		[]prepeetapi.StrictMiddlewareFunc{
			auditSensitiveReads(cfg.SensitiveReads, cfg.Identity, auditable, cfg.Environment),
			carryCredentials(cfg.TrustProxyHeaders, required),
		},
		prepeetapi.StrictHTTPServerOptions{
			// The generated defaults answer with http.Error, which is plain
			// text. That would make a malformed body the one failure in this
			// API a client cannot parse, and docs/contracts/public-api.md says
			// every failure uses one envelope.
			RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
				httpserver.WriteError(w, r, http.StatusBadRequest,
					string(prepeetapi.VALIDATIONFAILED),
					"The request body could not be read.", false)
			},
			ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
				// Reached when writing a response fails partway. The status is
				// already sent, so this is best effort, and the error's own
				// text is not echoed for the usual reason.
				httpserver.WriteError(w, r, http.StatusInternalServerError,
					string(prepeetapi.INTERNAL),
					"Something went wrong on our side. Please try again.", true)
			},
		})

	return httpserver.New(httpserver.Config{
		Health: cfg.Health,
		Routes: func(mux *http.ServeMux) {
			prepeetapi.HandlerWithOptions(strict, prepeetapi.StdHTTPServerOptions{
				BaseRouter: mux,
				BaseURL:    basePath,
				ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
					httpserver.WriteError(w, r, http.StatusBadRequest,
						string(prepeetapi.VALIDATIONFAILED),
						"The request could not be read.", false)
				},
			})
		},
	}), nil
}

// carryCredentials moves the session and refresh cookies, and the capability
// the contract declares for this operation, into the context.
//
// A strict middleware rather than an ordinary one, because this is the layer
// that still has both the raw request and the handler's context. An ordinary
// middleware could read the cookies but would have to smuggle them through the
// request context anyway, and would run before the generated router had decided
// which operation this is. The capability needs exactly that decision: the
// router has matched an operation by then, and its identifier is what indexes
// the contract's declaration.
func carryCredentials(trustProxy bool, required map[string]string) prepeetapi.StrictMiddlewareFunc {
	return func(f prepeetapi.StrictHandlerFunc, operationID string) prepeetapi.StrictHandlerFunc {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
			ctx = withCredentials(ctx, r, trustProxy)
			return f(withRequiredCapability(ctx, required[operationID]), w, r, request)
		}
	}
}

// server implements the whole generated interface.
//
// It is a thin composition of per-area handlers rather than one type with every
// method, so adding the session endpoints does not mean adding methods to the
// type that serves authentication.
type server struct {
	authentication
	profile
	documents
	catalog
	interviews
	members
	billingHandlers
	progression
	settingsHandlers
	campaignHandlers
	invitationHandlers
	health *health.Registry
}

// The probes are declared in the contract as well as served unversioned by
// platform/httpserver, because a contract that omitted them would be describing
// a different service from the one that runs. The orchestrator uses the
// unversioned paths; these exist so the document is complete and so the
// generated client can reach them.
func (s *server) GetLiveness(_ context.Context, _ prepeetapi.GetLivenessRequestObject) (prepeetapi.GetLivenessResponseObject, error) {
	return prepeetapi.GetLiveness200JSONResponse{
		Body:    prepeetapi.Liveness{Status: prepeetapi.Alive},
		Headers: prepeetapi.GetLiveness200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

func (s *server) GetReadiness(ctx context.Context, _ prepeetapi.GetReadinessRequestObject) (prepeetapi.GetReadinessResponseObject, error) {
	registry := s.health
	if registry == nil {
		registry = health.NewRegistry()
	}

	report := registry.Check(ctx)
	checks := make([]prepeetapi.HealthCheck, 0, len(report.Checks))
	for _, check := range report.Checks {
		checks = append(checks, prepeetapi.HealthCheck{
			Name:   check.Name,
			Status: prepeetapi.HealthCheckStatus(check.Status),
		})
	}

	body := prepeetapi.Readiness{
		Status: prepeetapi.ReadinessStatus(report.Status),
		Checks: checks,
	}
	// A probe answer is never cacheable. A cached readiness would keep traffic
	// flowing to a process that has already failed, which is the one caching
	// mistake that turns a degradation into an outage.
	if report.Status != health.StatusReady {
		return prepeetapi.GetReadiness503JSONResponse{
			Body:    body,
			Headers: prepeetapi.GetReadiness503ResponseHeaders{CacheControl: NoStore},
		}, nil
	}
	return prepeetapi.GetReadiness200JSONResponse{
		Body:    body,
		Headers: prepeetapi.GetReadiness200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

var _ prepeetapi.StrictServerInterface = (*server)(nil)
