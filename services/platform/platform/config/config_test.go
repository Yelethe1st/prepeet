package config_test

import (
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
)

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
