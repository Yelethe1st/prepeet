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
	"fmt"
	"net"
	"slices"
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
	}

	if !slices.Contains(environments, cfg.Environment) {
		return Config{}, fmt.Errorf("config: PREPEET_ENVIRONMENT is %q, want one of %v",
			cfg.Environment, environments)
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
