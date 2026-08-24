package temporal

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel"
	sdkclient "go.temporal.io/sdk/client"
	"go.temporal.io/sdk/contrib/opentelemetry"
	"go.temporal.io/sdk/interceptor"

	"github.com/Yelethe1st/prepeet/services/platform/platform/telemetry"
)

// Config is what this process needs to reach Temporal.
type Config struct {
	// Address is the frontend, as host:port.
	Address string
	// Namespace separates environments, per ADR-0007.
	Namespace string
	// CertFile and KeyFile are the client certificate pair. Empty for a
	// self-hosted server on a private network; both set for Temporal Cloud.
	CertFile string
	KeyFile  string

	Logger *slog.Logger
}

// ErrNotConfigured means this process has no Temporal address.
//
// A distinct error rather than a dial failure, because "this process does not
// use Temporal" and "Temporal is unreachable" need opposite responses and would
// otherwise be told apart by reading an error string.
var ErrNotConfigured = errors.New("temporal: no address is configured")

// Dial connects to Temporal.
//
// This is the one place a client is built, which is what makes ADR-0007's
// reversibility claim true: moving to Temporal Cloud changes the address, the
// namespace and the certificate pair, all of which arrive here as configuration.
//
// Three things are attached to every client rather than left to call sites,
// because each is a property of the system rather than of a caller:
//
//   - The data converter, so ADR-0007's payload rule holds on every path into
//     workflow history rather than on the ones somebody remembered.
//   - The OpenTelemetry interceptor, so a trace that started in an HTTP handler
//     continues into the workflow and its activities. Without it PLT-08's claim
//     that one trace spans the journey breaks at exactly the boundary where a
//     trace is most useful, since a workflow is where the minutes are spent.
//   - The logger, so workflow logs are scrubbed and correlated like everything
//     else.
func Dial(ctx context.Context, cfg Config) (sdkclient.Client, error) {
	if cfg.Address == "" {
		return nil, ErrNotConfigured
	}
	if cfg.Namespace == "" {
		return nil, errors.New("temporal: a namespace is required; it separates environments per ADR-0007")
	}

	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}

	tracer, err := opentelemetry.NewTracer(opentelemetry.TracerOptions{
		Tracer: otel.Tracer("platform/temporal"),
	})
	if err != nil {
		return nil, fmt.Errorf("temporal: building the tracing interceptor: %w", err)
	}

	options := sdkclient.Options{
		HostPort:      cfg.Address,
		Namespace:     cfg.Namespace,
		DataConverter: NewDataConverter(),
		Logger:        newLogAdapter(log),
		Interceptors:  []interceptor.ClientInterceptor{interceptor.NewTracingInterceptor(tracer)},
	}

	if cfg.CertFile != "" {
		certificate, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			// The paths are named and the material is not, since a key file
			// that failed to parse must not have its contents in a log line.
			return nil, fmt.Errorf("temporal: loading the client certificate from %s and %s: %w",
				cfg.CertFile, cfg.KeyFile, err)
		}
		options.ConnectionOptions = sdkclient.ConnectionOptions{
			TLS: &tls.Config{
				Certificates: []tls.Certificate{certificate},
				MinVersion:   tls.VersionTLS12,
			},
		}
	}

	client, err := sdkclient.DialContext(ctx, options)
	if err != nil {
		// The address is scrubbed on the way into the error rather than at
		// whatever logs it. Relying on the call site is the convention this
		// codebase keeps replacing with a mechanism, and the SDK echoes the
		// address back in its own message too, so the whole error is scrubbed
		// rather than just the part this line contributes.
		return nil, errors.New(telemetry.Scrub(
			fmt.Sprintf("temporal: dialling %s: %v", cfg.Address, err)))
	}
	return client, nil
}

// Check reports whether Temporal is reachable, for the readiness probe.
//
// It returns a bare error rather than a description, because health output is
// public and a dependency failure message is an invitation to map the
// deployment. platform/health already enforces that; this keeps the habit.
func Check(client sdkclient.Client) func(context.Context) error {
	return func(ctx context.Context) error {
		if client == nil {
			return ErrNotConfigured
		}
		if _, err := client.CheckHealth(ctx, &sdkclient.CheckHealthRequest{}); err != nil {
			return errors.New("temporal is not reachable")
		}
		return nil
	}
}
