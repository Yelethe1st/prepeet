package config_test

import (
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
)

// lookupFrom builds a Lookup over a fixed map, so a test states only the
// variables it cares about and inherits the defaults for everything else.
func lookupFrom(env map[string]string) config.Lookup {
	return func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	}
}

func TestLoadUsesDefaultsWhenNothingIsSet(t *testing.T) {
	t.Parallel()

	got, err := config.Load(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if got.Address != ":8080" {
		t.Errorf("Address = %q, want %q", got.Address, ":8080")
	}
	if got.Environment != "local" {
		t.Errorf("Environment = %q, want %q", got.Environment, "local")
	}
}

func TestLoadReadsTheEnvironment(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"PREPEET_ADDRESS":     ":9090",
		"PREPEET_ENVIRONMENT": "staging",
	}
	got, err := config.Load(func(key string) (string, bool) { v, ok := env[key]; return v, ok })
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if got.Address != ":9090" {
		t.Errorf("Address = %q, want %q", got.Address, ":9090")
	}
	if got.Environment != "staging" {
		t.Errorf("Environment = %q, want %q", got.Environment, "staging")
	}
}

// A misspelled environment name must fail at startup rather than silently
// running production code under local rules.
func TestLoadRejectsAnUnknownEnvironment(t *testing.T) {
	t.Parallel()

	env := map[string]string{"PREPEET_ENVIRONMENT": "prodution"}

	if _, err := config.Load(func(key string) (string, bool) { v, ok := env[key]; return v, ok }); err == nil {
		t.Error("Load returned no error for an unknown environment, want one")
	}
}

func TestLoadRejectsAnUnusableAddress(t *testing.T) {
	t.Parallel()

	for name, address := range map[string]string{
		"no port":    "localhost",
		"not a port": ":http-ish",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			env := map[string]string{"PREPEET_ADDRESS": address}
			if _, err := config.Load(func(key string) (string, bool) { v, ok := env[key]; return v, ok }); err == nil {
				t.Errorf("Load(%q) returned no error, want one", address)
			}
		})
	}
}

// Configuration errors are read by whoever is starting the process, so they
// must name the variable that is wrong.
func TestLoadErrorNamesTheOffendingVariable(t *testing.T) {
	t.Parallel()

	env := map[string]string{"PREPEET_ENVIRONMENT": "nonsense"}

	_, err := config.Load(func(key string) (string, bool) { v, ok := env[key]; return v, ok })
	if err == nil {
		t.Fatal("Load returned no error, want one")
	}
	if !strings.Contains(err.Error(), "PREPEET_ENVIRONMENT") {
		t.Errorf("error = %q, want it to name PREPEET_ENVIRONMENT", err)
	}
}

// Deployment tooling routinely sets a variable to the empty string when it
// means "not configured". Treating that as a configuration error would fail
// deployments over something the operator did not do, so an empty value falls
// back to the default exactly as an unset one does.
func TestEmptyValueIsTreatedAsUnset(t *testing.T) {
	t.Parallel()

	env := map[string]string{"PREPEET_ADDRESS": "", "PREPEET_ENVIRONMENT": ""}

	got, err := config.Load(func(key string) (string, bool) { v, ok := env[key]; return v, ok })
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got.Address != ":8080" {
		t.Errorf("Address = %q, want the default %q", got.Address, ":8080")
	}
	if got.Environment != config.EnvironmentLocal {
		t.Errorf("Environment = %q, want the default %q", got.Environment, config.EnvironmentLocal)
	}
}

// The database URL and the application role's password have no defaults. A
// default database URL would let a process start pointed at nothing, and a
// default password would be a credential in the repository.
func TestDatabaseSettingsHaveNoDefaults(t *testing.T) {
	t.Parallel()

	got, err := config.Load(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if got.DatabaseURL != "" {
		t.Errorf("DatabaseURL = %q, want empty: a default would point a process at nothing", got.DatabaseURL)
	}
	if got.AppDatabasePassword != "" {
		t.Errorf("AppDatabasePassword = %q, want empty: a default would be a credential in the repository", got.AppDatabasePassword)
	}
}

func TestDatabaseSettingsAreReadFromTheEnvironment(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"PREPEET_DATABASE_URL":          "postgres://prepeet@localhost:5432/prepeet",
		"PREPEET_APP_DATABASE_PASSWORD": "from-the-secret-store",
	}

	got, err := config.Load(func(key string) (string, bool) { v, ok := env[key]; return v, ok })
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if got.DatabaseURL != env["PREPEET_DATABASE_URL"] {
		t.Errorf("DatabaseURL = %q, want %q", got.DatabaseURL, env["PREPEET_DATABASE_URL"])
	}
	if got.AppDatabasePassword != env["PREPEET_APP_DATABASE_PASSWORD"] {
		t.Errorf("AppDatabasePassword = %q, want it read from the environment", got.AppDatabasePassword)
	}
}

// A password that reached a log line would sit in the telemetry store, which
// SEC-08 scans precisely to prevent.
func TestConfigDoesNotStringifyTheDatabasePassword(t *testing.T) {
	t.Parallel()

	env := map[string]string{"PREPEET_APP_DATABASE_PASSWORD": "hunter2"}
	got, err := config.Load(func(key string) (string, bool) { v, ok := env[key]; return v, ok })
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if strings.Contains(got.String(), "hunter2") {
		t.Errorf("String() = %q, want the password redacted", got.String())
	}
}

// ─────────────────────────────────────────────────────────────── telemetry

// Telemetry is off unless an endpoint is configured. A default endpoint would
// make every local process try to reach a collector that is not there, and the
// resulting connection errors train people to ignore telemetry logs.
func TestTelemetryIsOffByDefault(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(lookupFrom(map[string]string{}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.OTLPEndpoint != "" {
		t.Errorf("OTLPEndpoint = %q by default, want empty so no collector is assumed", cfg.OTLPEndpoint)
	}
}

func TestTelemetryEndpointIsRead(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(lookupFrom(map[string]string{
		"PREPEET_OTLP_ENDPOINT": "collector.internal:4317",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.OTLPEndpoint != "collector.internal:4317" {
		t.Errorf("OTLPEndpoint = %q, want collector.internal:4317", cfg.OTLPEndpoint)
	}
}

// Sampling defaults to everything. A product with no traffic gains nothing from
// sampling and loses the one trace it needed.
func TestSamplingDefaultsToEverything(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(lookupFrom(map[string]string{}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TraceSampleRatio != 1 {
		t.Errorf("TraceSampleRatio = %v, want 1", cfg.TraceSampleRatio)
	}
}

// A ratio outside the range would silently record nothing or be rejected deep
// inside the SDK, so it is refused at startup where the cause is visible.
func TestAnUnusableSampleRatioIsRefused(t *testing.T) {
	t.Parallel()

	for _, ratio := range []string{"-0.5", "1.5", "many"} {
		if _, err := config.Load(lookupFrom(map[string]string{
			"PREPEET_TRACE_SAMPLE_RATIO": ratio,
		})); err == nil {
			t.Errorf("Load accepted a sample ratio of %q", ratio)
		}
	}
}

func TestASampleRatioIsRead(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(lookupFrom(map[string]string{
		"PREPEET_TRACE_SAMPLE_RATIO": "0.25",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TraceSampleRatio != 0.25 {
		t.Errorf("TraceSampleRatio = %v, want 0.25", cfg.TraceSampleRatio)
	}
}

// ─────────────────────────────────────────────────────────────── temporal

// The namespace is derived from the environment rather than configured
// independently, so the mistake ADR-0007 worries about, a preview worker
// picking up production work, is not expressible.
func TestTheTemporalNamespaceFollowsTheEnvironment(t *testing.T) {
	t.Parallel()

	for environment, want := range map[string]string{
		"local":      "prepeet-local",
		"preview":    "prepeet-preview",
		"staging":    "prepeet-staging",
		"production": "prepeet-production",
	} {
		cfg, err := config.Load(lookupFrom(map[string]string{"PREPEET_ENVIRONMENT": environment}))
		if err != nil {
			t.Fatalf("Load(%s): %v", environment, err)
		}
		if cfg.TemporalNamespace != want {
			t.Errorf("in %s the namespace is %q, want %q", environment, cfg.TemporalNamespace, want)
		}
	}
}

// Temporal Cloud qualifies a namespace with an account, so an override has to
// be possible. It must still start with the derived name, which keeps the
// safety property while allowing the suffix.
func TestATemporalNamespaceOverrideMustStillMatchTheEnvironment(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(lookupFrom(map[string]string{
		"PREPEET_ENVIRONMENT":        "production",
		"PREPEET_TEMPORAL_NAMESPACE": "prepeet-production.a2dd6",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TemporalNamespace != "prepeet-production.a2dd6" {
		t.Errorf("TemporalNamespace = %q", cfg.TemporalNamespace)
	}
}

// The mistake worth making impossible.
func TestATemporalNamespaceFromAnotherEnvironmentIsRefused(t *testing.T) {
	t.Parallel()

	_, err := config.Load(lookupFrom(map[string]string{
		"PREPEET_ENVIRONMENT":        "preview",
		"PREPEET_TEMPORAL_NAMESPACE": "prepeet-production",
	}))
	if err == nil {
		t.Fatal("a preview process was allowed to point at the production namespace")
	}
	if !strings.Contains(err.Error(), "prepeet-preview") {
		t.Errorf("the error does not say what the namespace should have been: %v", err)
	}
}

// Empty means no Temporal, which is how a process that does not need it starts.
func TestTemporalIsOffByDefault(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(lookupFrom(map[string]string{}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TemporalAddress != "" {
		t.Errorf("TemporalAddress = %q by default, want empty", cfg.TemporalAddress)
	}
}

// Client certificates are what Temporal Cloud authenticates with, so both
// arriving together is the whole point of the swap being cheap. One without the
// other is a misconfiguration that would otherwise fail at dial time with a TLS
// error nobody can read.
func TestATemporalClientCertificateWithoutItsKeyIsRefused(t *testing.T) {
	t.Parallel()

	for _, env := range []map[string]string{
		{"PREPEET_TEMPORAL_ADDRESS": "temporal:7233", "PREPEET_TEMPORAL_TLS_CERT_FILE": "/certs/client.pem"},
		{"PREPEET_TEMPORAL_ADDRESS": "temporal:7233", "PREPEET_TEMPORAL_TLS_KEY_FILE": "/certs/client.key"},
	} {
		if _, err := config.Load(lookupFrom(env)); err == nil {
			t.Errorf("half a client certificate was accepted: %v", env)
		}
	}
}

func TestATemporalClientCertificatePairIsRead(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(lookupFrom(map[string]string{
		"PREPEET_TEMPORAL_ADDRESS":       "prepeet.a2dd6.tmprl.cloud:7233",
		"PREPEET_TEMPORAL_TLS_CERT_FILE": "/certs/client.pem",
		"PREPEET_TEMPORAL_TLS_KEY_FILE":  "/certs/client.key",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TemporalTLSCertFile == "" || cfg.TemporalTLSKeyFile == "" {
		t.Error("the certificate pair was not read")
	}
}

// IAM-08: a provider is configuration, and a half-configured one is absent.

func TestAProviderIsAbsentUntilItsCredentialsAreSet(t *testing.T) {
	cfg, err := config.Load(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// The endpoints have sensible defaults, so "configured" cannot mean
	// "some fields are set": it has to mean the credentials are there.
	if cfg.OAuthGoogle.ClientID != "" || cfg.OAuthGoogle.ClientSecret != "" {
		t.Fatalf("google arrived with credentials nobody set: %+v", cfg.OAuthGoogle)
	}
	if cfg.OAuthGoogle.TokenURL == "" || cfg.OAuthMicrosoft.TokenURL == "" {
		t.Fatal("the endpoints lost their defaults, so a deployer has to know them")
	}
}

func TestAProvidersEndpointsCanBeOverridden(t *testing.T) {
	env := map[string]string{}
	env["PREPEET_OAUTH_GOOGLE_CLIENT_ID"] = "client"
	env["PREPEET_OAUTH_GOOGLE_CLIENT_SECRET"] = "secret"
	env["PREPEET_OAUTH_GOOGLE_REDIRECT_URI"] = "https://app.example/auth/callback"
	// Overridable because providers move their endpoints, and pinning one in
	// code makes a redirect change into a release.
	env["PREPEET_OAUTH_GOOGLE_TOKEN_URL"] = "https://stand-in.example/token"

	cfg, err := config.Load(lookupFrom(env))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.OAuthGoogle.TokenURL != "https://stand-in.example/token" {
		t.Fatalf("token url = %q", cfg.OAuthGoogle.TokenURL)
	}
	if cfg.OAuthGoogle.RedirectURI != "https://app.example/auth/callback" {
		t.Fatalf("redirect = %q", cfg.OAuthGoogle.RedirectURI)
	}
}
