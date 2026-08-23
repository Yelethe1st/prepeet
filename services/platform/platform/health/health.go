// Package health reports whether this process is able to serve traffic.
//
// It answers two different questions, and the distinction matters to the
// deployment: liveness asks whether the process is running at all, readiness
// asks whether its dependencies are reachable. A process that is alive but
// unready should stop receiving traffic without being restarted.
//
// Implements part of PLT-01 and supports the traced-request exit criteria in
// docs/delivery/tickets/02-platform-foundation.md.
package health

import (
	"context"
	"sync"
)

// Status is the readiness of the service or of one dependency.
type Status string

const (
	// StatusReady means every registered dependency answered successfully.
	StatusReady Status = "ready"
	// StatusUnready means at least one dependency did not.
	StatusUnready Status = "unready"
)

// CheckFunc probes one dependency. It must respect ctx: a dependency that
// hangs must not hang the readiness probe, or a stuck database takes the whole
// deployment's health reporting down with it.
type CheckFunc func(ctx context.Context) error

// Result is one dependency's outcome.
//
// Detail is deliberately never populated from the dependency's error. A driver
// error routinely carries a host, a port and sometimes a credential, and this
// value is serialised into an unauthenticated response body. See
// docs/security/data-classification.md.
type Result struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Report is the aggregate outcome across every registered dependency.
type Report struct {
	Status Status   `json:"status"`
	Checks []Result `json:"checks"`
}

// Registry holds the dependency checks for one process.
//
// The zero value is not usable; call NewRegistry.
type Registry struct {
	mu     sync.RWMutex
	names  []string // preserved so an operator sees a stable order
	checks map[string]CheckFunc
}

// NewRegistry returns an empty registry. An empty registry reports ready,
// because a service with no dependencies has nothing to be unready about.
func NewRegistry() *Registry {
	return &Registry{checks: make(map[string]CheckFunc)}
}

// Register adds a dependency check under name. Registering the same name twice
// replaces the check and keeps its original position in the report.
func (r *Registry) Register(name string, check CheckFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.checks[name]; !exists {
		r.names = append(r.names, name)
	}
	r.checks[name] = check
}

// Check runs every registered dependency check concurrently and aggregates the
// outcome. One failure makes the whole service unready: partial readiness would
// let a deployment that cannot reach its database keep taking traffic.
//
// Results are returned in registration order regardless of completion order, so
// an operator reading two consecutive probes sees the same shape both times.
func (r *Registry) Check(ctx context.Context) Report {
	r.mu.RLock()
	names := make([]string, len(r.names))
	copy(names, r.names)
	checks := make(map[string]CheckFunc, len(r.checks))
	for name, check := range r.checks {
		checks[name] = check
	}
	r.mu.RUnlock()

	results := make([]Result, len(names))
	var wg sync.WaitGroup
	for i, name := range names {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			status := StatusReady
			if err := checks[name](ctx); err != nil {
				status = StatusUnready
			}
			results[i] = Result{Name: name, Status: status}
		}(i, name)
	}
	wg.Wait()

	report := Report{Status: StatusReady, Checks: results}
	for _, result := range results {
		if result.Status != StatusReady {
			report.Status = StatusUnready
			break
		}
	}
	return report
}
