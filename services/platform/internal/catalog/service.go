package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Reference is the registry reference the catalogue lives under. One
// document rather than one per collection, because the combination rules
// cross the collections and splitting them would let the halves version
// apart.
const Reference = "catalog"

// Source is what the service needs from the artifact registry, declared
// here per ADR-0005: resolve one reference for one tenant's view and answer
// the published body with its digest.
type Source interface {
	ResolveBody(ctx context.Context, reference, tenantID string) (body json.RawMessage, digest string, err error)
}

// Service serves parsed catalogues from the registry.
//
// Parsing is cached by digest, so a repeated request costs one registry read
// and no re-validation; a newly published version has a new digest and
// misses the cache by construction, which is what makes the cache safe to
// never invalidate.
type Service struct {
	source Source

	mu     sync.RWMutex
	parsed map[string]Catalogue
}

// NewService wires the catalogue over the registry.
func NewService(source Source) *Service {
	return &Service{source: source, parsed: map[string]Catalogue{}}
}

// Catalogue answers one tenant's view: the platform's published catalogue,
// unless the tenant has published an override under the same reference.
func (s *Service) Catalogue(ctx context.Context, tenantID string) (Catalogue, error) {
	body, digest, err := s.source.ResolveBody(ctx, Reference, tenantID)
	if err != nil {
		return Catalogue{}, fmt.Errorf("catalog: resolving the catalogue: %w", err)
	}

	s.mu.RLock()
	cached, hit := s.parsed[digest]
	s.mu.RUnlock()
	if hit {
		return cached, nil
	}

	parsed, err := Parse(body)
	if err != nil {
		// A published catalogue that does not parse is a publication bug the
		// validating state should have refused; surfacing it beats serving a
		// stale or empty catalogue as if nothing were wrong.
		return Catalogue{}, err
	}
	s.mu.Lock()
	s.parsed[digest] = parsed
	s.mu.Unlock()
	return parsed, nil
}
