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
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/platform/grpcdial"
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
// OAuthProvider is one configured sign-in provider.
//
// The endpoints carry defaults because they are stable and knowing them is not
// the deployer's job; the credentials carry none, because a default credential
// is a credential somebody forgot to set.
type OAuthProvider struct {
	ClientID     string
	ClientSecret string
	AuthorizeURL string
	TokenURL     string
	UserInfoURL  string
	// RedirectURI must match what is registered with the provider exactly.
	// It has no default: it is the one field that differs per deployment and
	// a wrong one fails at the provider with a message nobody sees.
	RedirectURI string
}

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
	// SMTPAddress is the mail relay, as host:port. Empty means this process
	// sends no email, which is how cmd/api and cmd/migrate start without one;
	// the worker warns loudly, because a silent worker is verification emails
	// silently not arriving.
	SMTPAddress string
	// SMTPUsername and SMTPPassword authenticate to the relay. Both empty is
	// unauthenticated. A password is never sent over a connection that could
	// not be upgraded, whatever SMTPAllowPlaintext says; see platform/email.
	SMTPUsername string
	SMTPPassword string
	// EmailFrom is the sender address on every message.
	EmailFrom string
	// SMTPAllowPlaintext permits sending to a relay that offers no STARTTLS.
	// A message body carries magic links and verification tokens, so this
	// follows the same rule as the intelligence hop: true by default in local
	// and preview, where Mailpit offers no upgrade, and a declaration
	// everywhere else. See platform/email.
	SMTPAllowPlaintext bool
	// IntelligenceAddress is the Python intelligence plane's gRPC endpoint, as
	// host:port. Empty means this process runs no workflows that need it; the
	// worker then skips registering the interview task queue and says so.
	IntelligenceAddress string
	// IntelligenceTLS secures that hop. It carries interview briefs out and
	// transcripts back, so it carries candidate speech. Deployed environments
	// must either configure certificate material or declare plaintext in as
	// many words; local and preview default to plaintext so that `make dev`
	// needs no certificate authority. See platform/grpcdial.
	IntelligenceTLS grpcdial.Config
	// The object store. Empty endpoint means real S3; the local stack points
	// it at LocalStack. Empty bucket disables the document surface loudly at
	// startup rather than at the first upload.
	LiveKitURL       string
	LiveKitAPIKey    string
	LiveKitAPISecret string
	// LiveKitAPIURL is the server's HTTP address for control-plane calls
	// (egress); distinct from the ws URL browsers dial.
	LiveKitAPIURL string
	// AgentToken authenticates the voice agent's writes into the timeline.
	// Empty disables the internal surface entirely.
	AgentToken string

	// Authentication rate limits (SEC-10). Configuration rather than
	// constants, so they can be tightened during an incident without a
	// deployment. Zero disables a counter, which is a local run.
	AuthAttemptsPerAddress int
	AuthAttemptsPerNetwork int
	AuthAttemptWindow      time.Duration
	// TrustProxyHeaders says a proxy in front of this process sets
	// X-Forwarded-For and may be believed. False means the transport's
	// own remote address is used, which nobody can forge.
	TrustProxyHeaders bool

	S3Endpoint     string
	S3Region       string
	S3Bucket       string
	S3AccessKey    string
	S3SecretKey    string
	S3UsePathStyle bool
	// WebBaseURL is where emailed links point, such as https://app.prepeet.com.
	// The links land in inboxes, so a wrong value is not a broken page but a
	// thousand dead links that cannot be recalled.
	WebBaseURL string

	// OAuthGoogle and OAuthMicrosoft are the providers this deployment offers.
	//
	// Empty means not offered, which is the honest default: the sign-in screen
	// draws a button per configured provider, so a deployment that has not set
	// these shows email and password alone rather than a button that fails at
	// the token endpoint.
	//
	// The endpoints are here rather than compiled in because a provider is
	// configuration, per IAM-08, and because Google and Microsoft both move
	// them: pinning a URL in code is a release to fix a redirect.
	OAuthGoogle    OAuthProvider
	OAuthMicrosoft OAuthProvider

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

		TemporalAddress: value(lookup, "PREPEET_TEMPORAL_ADDRESS", ""),
		SMTPAddress:     value(lookup, "PREPEET_SMTP_ADDRESS", ""),
		SMTPUsername:    value(lookup, "PREPEET_SMTP_USERNAME", ""),
		SMTPPassword:    value(lookup, "PREPEET_SMTP_PASSWORD", ""),
		EmailFrom:       value(lookup, "PREPEET_EMAIL_FROM", ""),
		WebBaseURL:      value(lookup, "PREPEET_WEB_BASE_URL", ""),
		OAuthGoogle: OAuthProvider{
			ClientID:     value(lookup, "PREPEET_OAUTH_GOOGLE_CLIENT_ID", ""),
			ClientSecret: value(lookup, "PREPEET_OAUTH_GOOGLE_CLIENT_SECRET", ""),
			AuthorizeURL: value(lookup, "PREPEET_OAUTH_GOOGLE_AUTHORIZE_URL",
				"https://accounts.google.com/o/oauth2/v2/auth"),
			TokenURL: value(lookup, "PREPEET_OAUTH_GOOGLE_TOKEN_URL",
				"https://oauth2.googleapis.com/token"),
			UserInfoURL: value(lookup, "PREPEET_OAUTH_GOOGLE_USERINFO_URL",
				"https://openidconnect.googleapis.com/v1/userinfo"),
			RedirectURI: value(lookup, "PREPEET_OAUTH_GOOGLE_REDIRECT_URI", ""),
		},
		OAuthMicrosoft: OAuthProvider{
			ClientID:     value(lookup, "PREPEET_OAUTH_MICROSOFT_CLIENT_ID", ""),
			ClientSecret: value(lookup, "PREPEET_OAUTH_MICROSOFT_CLIENT_SECRET", ""),
			AuthorizeURL: value(lookup, "PREPEET_OAUTH_MICROSOFT_AUTHORIZE_URL",
				"https://login.microsoftonline.com/common/oauth2/v2.0/authorize"),
			TokenURL: value(lookup, "PREPEET_OAUTH_MICROSOFT_TOKEN_URL",
				"https://login.microsoftonline.com/common/oauth2/v2.0/token"),
			UserInfoURL: value(lookup, "PREPEET_OAUTH_MICROSOFT_USERINFO_URL",
				"https://graph.microsoft.com/oidc/userinfo"),
			RedirectURI: value(lookup, "PREPEET_OAUTH_MICROSOFT_REDIRECT_URI", ""),
		},
		LiveKitURL:             value(lookup, "PREPEET_LIVEKIT_URL", ""),
		LiveKitAPIKey:          value(lookup, "PREPEET_LIVEKIT_API_KEY", ""),
		LiveKitAPISecret:       value(lookup, "PREPEET_LIVEKIT_API_SECRET", ""),
		LiveKitAPIURL:          value(lookup, "PREPEET_LIVEKIT_API_URL", ""),
		AgentToken:             value(lookup, "PREPEET_AGENT_TOKEN", ""),
		AuthAttemptsPerAddress: number(lookup, "PREPEET_AUTH_ATTEMPTS_PER_ADDRESS", 10),
		AuthAttemptsPerNetwork: number(lookup, "PREPEET_AUTH_ATTEMPTS_PER_NETWORK", 60),
		AuthAttemptWindow:      window(lookup, "PREPEET_AUTH_ATTEMPT_WINDOW", 15*time.Minute),
		TrustProxyHeaders:      value(lookup, "PREPEET_TRUST_PROXY_HEADERS", "") == "true",
		S3Endpoint:             value(lookup, "PREPEET_S3_ENDPOINT", ""),
		S3Region:               value(lookup, "PREPEET_S3_REGION", "eu-west-2"),
		S3Bucket:               value(lookup, "PREPEET_S3_BUCKET", ""),
		S3AccessKey:            value(lookup, "PREPEET_S3_ACCESS_KEY", ""),
		S3SecretKey:            value(lookup, "PREPEET_S3_SECRET_KEY", ""),
		S3UsePathStyle:         value(lookup, "PREPEET_S3_PATH_STYLE", "") == "true",
		SMTPAllowPlaintext:     value(lookup, "PREPEET_SMTP_ALLOW_PLAINTEXT", "") == "true",
		IntelligenceAddress:    value(lookup, "PREPEET_INTELLIGENCE_ADDRESS", ""),
		IntelligenceTLS: grpcdial.Config{
			CAFile:   value(lookup, "PREPEET_INTELLIGENCE_TLS_CA_FILE", ""),
			CertFile: value(lookup, "PREPEET_INTELLIGENCE_TLS_CERT_FILE", ""),
			KeyFile:  value(lookup, "PREPEET_INTELLIGENCE_TLS_KEY_FILE", ""),
			Insecure: value(lookup, "PREPEET_INTELLIGENCE_TLS_INSECURE", "") == "true",
		},
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

	// The relay's. Not a startup refusal, because whether a relay offers
	// STARTTLS is only knowable when the conversation happens; platform/email
	// refuses the send, and this decides whether it may be waved through.
	if !cfg.SMTPAllowPlaintext {
		switch cfg.Environment {
		case EnvironmentLocal, EnvironmentPreview:
			cfg.SMTPAllowPlaintext = true
		default:
		}
	}

	// The intelligence hop's transport. Only a process that dials it needs a
	// decision, and the decision differs by environment, which is why it is
	// made here rather than in grpcdial: that package cannot tell whether the
	// plaintext it was handed is a laptop or an oversight, and this can.
	if cfg.IntelligenceAddress != "" {
		if !cfg.IntelligenceTLS.Insecure && cfg.IntelligenceTLS.CAFile == "" &&
			cfg.IntelligenceTLS.CertFile == "" {
			switch cfg.Environment {
			case EnvironmentLocal, EnvironmentPreview:
				// The local stack serves plaintext, so say so explicitly here
				// rather than leaving grpcdial to infer it from emptiness.
				cfg.IntelligenceTLS.Insecure = true
			default:
				return Config{}, fmt.Errorf("config: the %s environment dials the intelligence "+
					"plane over plaintext; set PREPEET_INTELLIGENCE_TLS_CA_FILE, or "+
					"PREPEET_INTELLIGENCE_TLS_INSECURE=true if a service mesh encrypts it",
					cfg.Environment)
			}
		}
		// Shape checked here so half a pair fails at startup with its cause
		// named, rather than at the first dial with a TLS error nobody can
		// read. The material itself is read when the connection is made.
		if err := cfg.IntelligenceTLS.Validate(); err != nil {
			return Config{}, fmt.Errorf("config: the intelligence plane transport: %w", err)
		}
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

// number returns a positive integer setting, or the fallback.
//
// A value that does not parse, or is negative, falls back rather than
// failing: a typo in a limit during an incident must not stop the process
// from starting, and the fallback is a working limit rather than none.
func number(lookup Lookup, key string, fallback int) int {
	raw := value(lookup, key, "")
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

// window returns a duration setting, or the fallback, on the same terms.
func window(lookup Lookup, key string, fallback time.Duration) time.Duration {
	raw := value(lookup, key, "")
	if raw == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
