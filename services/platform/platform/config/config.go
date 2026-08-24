// Package config loads process configuration from the environment.
//
// Configuration is read once at startup and validated immediately. A process
// that starts with configuration it cannot use is worse than one that refuses
// to start, because the failure surfaces later and further from its cause.
//
// The lookup function is injected rather than calling os.Getenv directly, which
// keeps the package testable without mutating global process state.
//
// Implements part of PLT-01.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// Lookup reads one configuration value. os.LookupEnv satisfies it.
type Lookup func(key string) (value string, ok bool)

// Environment names the deployment this process is part of. It is validated
// against a fixed set so a typo cannot silently select different behaviour.
type Environment string

const (
	EnvironmentLocal      Environment = "local"
	EnvironmentPreview    Environment = "preview"
	EnvironmentStaging    Environment = "staging"
	EnvironmentProduction Environment = "production"
)

var environments = []Environment{
	EnvironmentLocal, EnvironmentPreview, EnvironmentStaging, EnvironmentProduction,
}

// Config is the validated configuration for one process.
type Config struct {
	// Address is the TCP address the HTTP server listens on.
	Address string
	// Environment names the deployment. Behaviour that differs by environment
	// reads this rather than sniffing hostnames.
	Environment Environment
	// DatabaseURL is the PostgreSQL connection string. It has no default: a
	// default would let a process start pointed at nothing and fail later,
	// further from the cause.
	DatabaseURL string
	// OTLPEndpoint is the OpenTelemetry collector, as host:port. Empty disables
	// export, which is the local default: an engineer running the stack should
	// not need a collector, and a default endpoint would produce a steady
	// stream of connection errors that teaches everyone to ignore them.
	OTLPEndpoint string
	// TraceSampleRatio is the fraction of traces recorded, between 0 and 1.
	TraceSampleRatio float64
	// TemporalAddress is the Temporal frontend, as host:port. Empty means this
	// process does not use Temporal, which is how cmd/api and cmd/migrate start
	// without it.
	TemporalAddress string
	// TemporalNamespace is derived from Environment rather than configured
	// independently. ADR-0007 separates environments by namespace, and a
	// namespace that could name any environment makes that a matter of getting
	// one variable right.
	TemporalNamespace string
	// TemporalTLSCertFile and TemporalTLSKeyFile are the client certificate
	// pair. Empty for a self-hosted server on a private network; both set for
	// Temporal Cloud, which is the whole of what that swap costs in code.
	TemporalTLSCertFile string
	TemporalTLSKeyFile  string
	// AppDatabasePassword is the password for the prepeet_app role, used only
	// by cmd/migrate when creating the role. It has no default because a
	// default would be a credential in the repository, and in a deployed
	// environment it comes from the secret store per PLT-07.
	AppDatabasePassword string
}

// String renders the configuration for logging with secrets redacted.
//
// Config is logged at startup, which is useful and is also exactly how a
// credential ends up in a telemetry store that SEC-08 then has to scan for.
// Redacting here rather than at each call site means a future field cannot leak
// because someone logged the struct without thinking.
func (c Config) String() string {
	password := "unset"
	if c.AppDatabasePassword != "" {
		password = "[redacted]"
	}
	return fmt.Sprintf("Config{Address:%s Environment:%s DatabaseURL:%s AppDatabasePassword:%s "+
		"OTLPEndpoint:%s TraceSampleRatio:%v TemporalAddress:%s TemporalNamespace:%s}",
		c.Address, c.Environment, redactURL(c.DatabaseURL), password,
		c.OTLPEndpoint, c.TraceSampleRatio, c.TemporalAddress, c.TemporalNamespace)
}

// redactURL removes any password embedded in a connection string, which is the
// usual place one hides.
func redactURL(raw string) string {
	if raw == "" {
		return "unset"
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		// An unparseable URL may still contain a credential, so it is not
		// echoed back.
		return "[unparseable]"
	}
	if parsed.User != nil {
		if _, hasPassword := parsed.User.Password(); hasPassword {
			parsed.User = url.UserPassword(parsed.User.Username(), "redacted")
		}
	}
	return parsed.String()
}

// Load reads and validates configuration.
//
// Every value has a default that is safe for local development, so a new
// engineer can start the stack without a configuration file. Defaults are never
// production credentials: anything secret has no default at all and is added by
// PLT-07 as a required value.
func Load(lookup Lookup) (Config, error) {
	cfg := Config{
		Address:     value(lookup, "PREPEET_ADDRESS", ":8080"),
		Environment: Environment(value(lookup, "PREPEET_ENVIRONMENT", string(EnvironmentLocal))),

		OTLPEndpoint: value(lookup, "PREPEET_OTLP_ENDPOINT", ""),

		TemporalAddress:     value(lookup, "PREPEET_TEMPORAL_ADDRESS", ""),
		TemporalTLSCertFile: value(lookup, "PREPEET_TEMPORAL_TLS_CERT_FILE", ""),
		TemporalTLSKeyFile:  value(lookup, "PREPEET_TEMPORAL_TLS_KEY_FILE", ""),

		// No defaults. See the field comments.
		DatabaseURL:         value(lookup, "PREPEET_DATABASE_URL", ""),
		AppDatabasePassword: value(lookup, "PREPEET_APP_DATABASE_PASSWORD", ""),
	}

	// Sampling defaults to everything. A product with no traffic gains nothing
	// from sampling and loses the one trace it needed.
	ratio, err := strconv.ParseFloat(value(lookup, "PREPEET_TRACE_SAMPLE_RATIO", "1"), 64)
	if err != nil || ratio < 0 || ratio > 1 {
		return Config{}, fmt.Errorf("config: PREPEET_TRACE_SAMPLE_RATIO is %q, want a number between 0 and 1",
			value(lookup, "PREPEET_TRACE_SAMPLE_RATIO", "1"))
	}
	cfg.TraceSampleRatio = ratio

	if !slices.Contains(environments, cfg.Environment) {
		return Config{}, fmt.Errorf("config: PREPEET_ENVIRONMENT is %q, want one of %v",
			cfg.Environment, environments)
	}

	// The namespace follows the environment. An override is permitted because
	// Temporal Cloud qualifies a namespace with an account identifier, but it
	// must still start with the derived name, so a process can add a suffix and
	// cannot point at a different environment.
	derived := "prepeet-" + string(cfg.Environment)
	cfg.TemporalNamespace = value(lookup, "PREPEET_TEMPORAL_NAMESPACE", derived)
	if !strings.HasPrefix(cfg.TemporalNamespace, derived) {
		return Config{}, fmt.Errorf("config: PREPEET_TEMPORAL_NAMESPACE is %q in the %s environment, "+
			"want %q or that with an account suffix", cfg.TemporalNamespace, cfg.Environment, derived)
	}

	// Half a certificate pair fails at dial time with a TLS error nobody can
	// read, so it fails here instead where the cause is named.
	if (cfg.TemporalTLSCertFile == "") != (cfg.TemporalTLSKeyFile == "") {
		return Config{}, errors.New("config: PREPEET_TEMPORAL_TLS_CERT_FILE and " +
			"PREPEET_TEMPORAL_TLS_KEY_FILE must be set together or not at all")
	}

	// SplitHostPort rejects the shapes net.Listen would fail on later, so the
	// process refuses to start rather than failing after it reports healthy.
	if _, port, err := net.SplitHostPort(cfg.Address); err != nil {
		return Config{}, fmt.Errorf("config: PREPEET_ADDRESS is %q, want host:port: %w", cfg.Address, err)
	} else if _, err := net.LookupPort("tcp", port); err != nil {
		return Config{}, fmt.Errorf("config: PREPEET_ADDRESS port is %q, want a port number or known service name", port)
	}

	return cfg, nil
}

// value returns the configured value for key, or fallback when it is unset or
// empty. An empty variable is treated as unset because deployment tooling
// routinely sets one to the empty string.
func value(lookup Lookup, key, fallback string) string {
	if v, ok := lookup(key); ok && v != "" {
		return v
	}
	return fallback
}
