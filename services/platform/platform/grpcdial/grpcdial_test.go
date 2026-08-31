package grpcdial_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/Yelethe1st/prepeet/services/platform/platform/grpcdial"
)

// authority issues the certificates one test needs, writing them where the
// configuration expects paths. Real material and a real handshake: asserting
// on which dial options were built would prove only that the code did what the
// test already assumed, and the failure being guarded against is a
// configuration that looks encrypted and is not.
type authority struct {
	dir         string
	certificate *x509.Certificate
	key         *ecdsa.PrivateKey
	rootPEM     []byte
}

func newAuthority(t *testing.T) *authority {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating the authority key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "prepeet test authority"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating the authority certificate: %v", err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parsing the authority certificate: %v", err)
	}
	root := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return &authority{dir: t.TempDir(), certificate: parsed, key: key, rootPEM: root}
}

// rootFile writes the authority's own certificate and answers its path.
func (a *authority) rootFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(a.dir, "ca.pem")
	if err := os.WriteFile(path, a.rootPEM, 0o600); err != nil {
		t.Fatalf("writing the root: %v", err)
	}
	return path
}

// issue writes a leaf certificate and its key, answering both paths.
func (a *authority) issue(t *testing.T, name string, server bool) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating the leaf key: %v", err)
	}
	usage := x509.ExtKeyUsageClientAuth
	if server {
		usage = x509.ExtKeyUsageServerAuth
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
	}
	if server {
		template.DNSNames = []string{name}
		template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, a.certificate, &key.PublicKey, a.key)
	if err != nil {
		t.Fatalf("creating the leaf certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshalling the leaf key: %v", err)
	}
	certPath := filepath.Join(a.dir, name+".pem")
	keyPath := filepath.Join(a.dir, name+".key")
	write := func(path string, block *pem.Block) {
		if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
	}
	write(certPath, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	write(keyPath, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPath, keyPath
}

// serve starts a health server on a loopback port with the given transport,
// answering its address. A health check is the smallest real call.
func serve(t *testing.T, creds credentials.TransportCredentials) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	options := []grpc.ServerOption{}
	if creds != nil {
		options = append(options, grpc.Creds(creds))
	}
	server := grpc.NewServer(options...)
	healthpb.RegisterHealthServer(server, health.NewServer())
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	return listener.Addr().String()
}

// probe makes one call over the dialled connection, so the handshake happens.
func probe(t *testing.T, address string, config grpcdial.Config) error {
	t.Helper()
	option, err := config.DialOption()
	if err != nil {
		t.Fatalf("building the dial option: %v", err)
	}
	conn, err := grpc.NewClient(address, option)
	if err != nil {
		t.Fatalf("dialling: %v", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	return err
}

// serverCredentials builds the server side of the handshake for a test.
func serverCredentials(t *testing.T, a *authority, requireClient bool) credentials.TransportCredentials {
	t.Helper()
	certPath, keyPath := a.issue(t, "localhost", true)
	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("loading the server pair: %v", err)
	}
	config := &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}
	if requireClient {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(a.rootPEM) {
			t.Fatal("the authority did not parse")
		}
		config.ClientCAs = pool
		config.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return credentials.NewTLS(config)
}

func TestPlaintextIsRefusedByATLSServer(t *testing.T) {
	t.Parallel()
	a := newAuthority(t)
	address := serve(t, serverCredentials(t, a, false))

	// The downgrade this package exists to prevent: the worker's dial used to
	// be insecure with no alternative, and nothing anywhere would notice.
	err := probe(t, address, grpcdial.Config{Insecure: true})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("a plaintext client reached a TLS server: %v", err)
	}
}

func TestAClientHoldingTheAuthorityIsServed(t *testing.T) {
	t.Parallel()
	a := newAuthority(t)
	address := serve(t, serverCredentials(t, a, false))

	if err := probe(t, address, grpcdial.Config{CAFile: a.rootFile(t)}); err != nil {
		t.Fatalf("the handshake failed: %v", err)
	}
}

func TestAClientNotHoldingTheAuthorityIsRefused(t *testing.T) {
	t.Parallel()
	a := newAuthority(t)
	address := serve(t, serverCredentials(t, a, false))

	stranger := newAuthority(t)
	err := probe(t, address, grpcdial.Config{CAFile: stranger.rootFile(t)})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("an untrusted certificate was accepted: %v", err)
	}
}

func TestMutualTLSAdmitsAClientWithACertificate(t *testing.T) {
	t.Parallel()
	a := newAuthority(t)
	address := serve(t, serverCredentials(t, a, true))

	certPath, keyPath := a.issue(t, "worker", false)
	config := grpcdial.Config{CAFile: a.rootFile(t), CertFile: certPath, KeyFile: keyPath}
	if err := probe(t, address, config); err != nil {
		t.Fatalf("the mutual handshake failed: %v", err)
	}
}

func TestMutualTLSRefusesAClientWithout(t *testing.T) {
	t.Parallel()
	a := newAuthority(t)
	address := serve(t, serverCredentials(t, a, true))

	err := probe(t, address, grpcdial.Config{CAFile: a.rootFile(t)})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("a client presenting no certificate was admitted: %v", err)
	}
}

func TestConfigRefusesWhatItCannotHonour(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		config grpcdial.Config
		want   string
	}{
		{
			// Silence here is the whole bug: a config meaning to be encrypted
			// and falling back to plaintext without saying so.
			name:   "nothing configured and no declaration",
			config: grpcdial.Config{},
			want:   "Insecure",
		},
		{
			name:   "half a client pair",
			config: grpcdial.Config{CAFile: "/tls/ca.pem", CertFile: "/tls/c.pem"},
			want:   "together",
		},
		{
			name:   "insecure declared alongside material",
			config: grpcdial.Config{Insecure: true, CAFile: "/tls/ca.pem"},
			want:   "both",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := testCase.config.DialOption()
			if err == nil {
				t.Fatal("the configuration was accepted")
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error %q does not mention %q", err, testCase.want)
			}
		})
	}
}

func TestAMissingFileIsNamedAndItsContentsAreNot(t *testing.T) {
	t.Parallel()
	_, err := grpcdial.Config{CAFile: "/nonexistent/ca.pem"}.DialOption()
	if err == nil {
		t.Fatal("a missing authority file was accepted")
	}
	if !strings.Contains(err.Error(), "/nonexistent/ca.pem") {
		t.Fatalf("error %q does not name the path", err)
	}
}

func TestAnUnparseableAuthorityIsRefused(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, []byte("not a certificate"), 0o600); err != nil {
		t.Fatalf("writing: %v", err)
	}
	// An unusable pool would otherwise fall through to the system roots and
	// verify against a public authority, which is not what was asked for.
	if _, err := (grpcdial.Config{CAFile: path}).DialOption(); err == nil {
		t.Fatal("an unparseable authority was accepted")
	}
}

// PLT-08: the Go to Python hop is where a trace most obviously broke. The
// client sent no trace context at all, so everything the intelligence plane did
// was invisible from the request that caused it, and the Python side had
// nothing to continue even once it wanted to.
func TestTheDialCarriesTraceContextToTheServer(t *testing.T) {
	t.Parallel()
	a := newAuthority(t)

	// A server that records whatever traceparent arrives in the metadata.
	arrived := make(chan string, 1)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	server := grpc.NewServer(grpc.UnaryInterceptor(
		func(ctx context.Context, req any, _ *grpc.UnaryServerInfo,
			handler grpc.UnaryHandler) (any, error) {
			md, _ := metadata.FromIncomingContext(ctx)
			values := md.Get("traceparent")
			select {
			case arrived <- firstOrEmpty(values):
			default:
			}
			return handler(ctx, req)
		}))
	healthpb.RegisterHealthServer(server, health.NewServer())
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	spans := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	ctx, span := provider.Tracer("test").Start(context.Background(), "caller")
	defer span.End()

	option, err := grpcdial.Config{Insecure: true}.DialOption()
	if err != nil {
		t.Fatalf("dial option: %v", err)
	}
	conn, err := grpc.NewClient(listener.Addr().String(), option,
		grpcdial.TraceOption())
	if err != nil {
		t.Fatalf("dialling: %v", err)
	}
	defer func() { _ = conn.Close() }()

	timeout, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, _ = healthpb.NewHealthClient(conn).Check(timeout, &healthpb.HealthCheckRequest{})

	select {
	case carried := <-arrived:
		if carried == "" {
			t.Fatal("the call arrived with no traceparent, so the trace ends at the language boundary")
		}
		if !strings.Contains(carried, span.SpanContext().TraceID().String()) {
			t.Fatalf("traceparent %q does not name the calling trace %s",
				carried, span.SpanContext().TraceID())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the call never reached the server")
	}
	_ = a
}

func firstOrEmpty(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
