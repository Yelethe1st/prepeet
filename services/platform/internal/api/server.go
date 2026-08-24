package api

import (
	"context"
	"errors"
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
	Identity    Identity
	Environment config.Environment
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

	handlers := &server{
		authentication: authentication{
			identity:    cfg.Identity,
			environment: cfg.Environment,
		},
		health: cfg.Health,
	}

	strict := prepeetapi.NewStrictHandlerWithOptions(handlers,
		[]prepeetapi.StrictMiddlewareFunc{carryCredentials},
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

// carryCredentials moves the session and refresh cookies into the context.
//
// A strict middleware rather than an ordinary one, because this is the layer
// that still has both the raw request and the handler's context. An ordinary
// middleware could read the cookies but would have to smuggle them through the
// request context anyway, and would run before the generated router had decided
// which operation this is.
func carryCredentials(f prepeetapi.StrictHandlerFunc, _ string) prepeetapi.StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
		return f(withCredentials(ctx, r), w, r, request)
	}
}

// server implements the whole generated interface.
//
// It is a thin composition of per-area handlers rather than one type with every
// method, so adding the session endpoints does not mean adding methods to the
// type that serves authentication.
type server struct {
	authentication
	health *health.Registry
}

// The probes are declared in the contract as well as served unversioned by
// platform/httpserver, because a contract that omitted them would be describing
// a different service from the one that runs. The orchestrator uses the
// unversioned paths; these exist so the document is complete and so the
// generated client can reach them.
func (s *server) GetLiveness(_ context.Context, _ prepeetapi.GetLivenessRequestObject) (prepeetapi.GetLivenessResponseObject, error) {
	return prepeetapi.GetLiveness200JSONResponse{Status: prepeetapi.Alive}, nil
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
	if report.Status != health.StatusReady {
		return prepeetapi.GetReadiness503JSONResponse(body), nil
	}
	return prepeetapi.GetReadiness200JSONResponse(body), nil
}

var _ prepeetapi.StrictServerInterface = (*server)(nil)
