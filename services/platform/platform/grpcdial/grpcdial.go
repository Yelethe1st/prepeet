// Package grpcdial builds the transport half of a gRPC client connection.
//
// It exists because the worker's dial to the intelligence plane asked for
// insecure credentials unconditionally, with a comment beside it claiming the
// deployed path got TLS. No code anywhere provided that path, on either end.
// Briefs go out over this hop and transcripts come back, so it carries
// candidate speech.
//
// The package is deliberately mechanical: it turns configuration into a dial
// option and refuses configurations it cannot honour. Whether plaintext is
// acceptable is not a question it can answer, because it does not know which
// environment it is running in. platform/config decides that, where the
// environment name is; here the only rule is that plaintext must be asked for
// in as many words. A caller that configures nothing gets an error rather than
// an unencrypted connection, because the failure this package was written for
// is a deployment that meant to be encrypted and quietly was not.
package grpcdial

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Config is one connection's transport security.
type Config struct {
	// CAFile is the authority the server's certificate is verified against.
	// Empty alongside a certificate pair means the system roots, which is
	// right for a managed endpoint and wrong for an internal one, so the
	// internal deployments set it.
	CAFile string
	// CertFile and KeyFile are this client's own certificate, presented when
	// the server requires one. Both empty is one-way TLS.
	CertFile string
	KeyFile  string
	// Insecure declares plaintext. It is a field rather than the absence of
	// the others so that an unconfigured deployment fails instead of
	// downgrading: forgetting to set something must not be the same as
	// choosing plaintext.
	Insecure bool
}

// DialOption answers the transport credentials this configuration describes,
// reading the certificate material named by the paths.
func (c Config) DialOption() (grpc.DialOption, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if c.Insecure {
		return grpc.WithTransportCredentials(insecure.NewCredentials()), nil
	}

	config := &tls.Config{MinVersion: tls.VersionTLS12}
	if c.CAFile != "" {
		pool, err := loadPool(c.CAFile)
		if err != nil {
			return nil, err
		}
		config.RootCAs = pool
	}
	if c.CertFile != "" {
		// The paths are named and the material is not: a key that failed to
		// parse must not have its contents in whatever logs the error.
		pair, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("grpcdial: loading the client certificate from %s and %s: %w",
				c.CertFile, c.KeyFile, err)
		}
		config.Certificates = []tls.Certificate{pair}
	}
	return grpc.WithTransportCredentials(credentials.NewTLS(config)), nil
}

// Validate refuses the configurations that would silently mean something other
// than they appear to.
//
// Shape only: the paths are not read here, so a process can validate its
// configuration before the material exists. Startup calls this, and the dial
// reads the files, exactly as platform/temporal splits the same two jobs.
func (c Config) Validate() error {
	if (c.CertFile == "") != (c.KeyFile == "") {
		return errors.New("grpcdial: the client certificate and key must be set together or not at all")
	}
	configured := c.CAFile != "" || c.CertFile != ""
	if c.Insecure && configured {
		return errors.New("grpcdial: Insecure and certificate material are both set; " +
			"one of them is not doing what whoever set it expected")
	}
	if !c.Insecure && !configured {
		return errors.New("grpcdial: no certificate material and Insecure is not set; " +
			"plaintext has to be asked for, so that forgetting is not the same as choosing")
	}
	return nil
}

// loadPool reads an authority into a pool of exactly that authority.
//
// A pool that parsed nothing is refused rather than returned empty, because an
// empty RootCAs is not "no verification" but "the system roots", and a
// deployment pointing at an internal endpoint would then verify against the
// public authorities and appear to work.
func loadPool(path string) (*x509.CertPool, error) {
	material, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("grpcdial: reading the certificate authority %s: %w", path, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(material) {
		return nil, fmt.Errorf("grpcdial: the certificate authority %s contains no certificate", path)
	}
	return pool, nil
}

// TraceOption instruments a connection so a call carries the caller's trace.
//
// Separate from DialOption because it answers a different question. DialOption
// decides who may be talked to; this decides whether the conversation is
// visible. Keeping them apart means a caller cannot accidentally get tracing
// and lose the transport rule, or the reverse, by picking one constructor.
//
// PLT-08's criterion is one trace with no broken links, and this hop was the
// most obvious break: the intelligence plane did the slowest work in the
// product and nothing it did could be connected to the request that caused it.
// The stats handler injects W3C trace context into the call's metadata, which
// is the same format the outbox stores and the same one the Python side reads,
// so no part of the journey needs a private agreement with another part.
func TraceOption() grpc.DialOption {
	return grpc.WithStatsHandler(otelgrpc.NewClientHandler(
		otelgrpc.WithPropagators(tracePropagator)))
}

// TraceServerOption is the serving half, for a Go gRPC server.
//
// Unused today because the only gRPC server is Python's. It is here so that the
// two halves are written and read together: a future Go server that forgot to
// continue the trace would be a break nobody notices, and finding the client
// half alone invites exactly that.
func TraceServerOption() grpc.ServerOption {
	return grpc.StatsHandler(otelgrpc.NewServerHandler(
		otelgrpc.WithPropagators(tracePropagator)))
}

// tracePropagator is named here rather than taken from otel's global, which
// defaults to a noop until telemetry.Setup installs one. Depending on the
// global would mean a process that traced correctly and a test that silently
// propagated nothing, and the difference would only show up as a broken link in
// production. It matches what telemetry.Setup installs and what the outbox
// stores, deliberately: one format across every hop is the whole point.
var tracePropagator = propagation.NewCompositeTextMapPropagator(
	propagation.TraceContext{}, propagation.Baggage{})
