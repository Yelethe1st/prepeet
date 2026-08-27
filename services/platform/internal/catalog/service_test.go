package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/catalog"
)

// The service over the registry: what it caches, what it does not, and
// what it refuses to serve.
//
// The cache is keyed by digest, which is what makes it safe to never
// invalidate: a newly published catalogue has a new digest and misses by
// construction. A published catalogue that does not parse is surfaced
// rather than served empty, because an empty catalogue offers the wizard
// nothing and says nothing about why.

type fakeSource struct {
	body   json.RawMessage
	digest string
	err    error
	reads  int
	tenant string
}

func (f *fakeSource) ResolveBody(_ context.Context, _, tenantID string) (json.RawMessage, string, error) {
	f.reads++
	f.tenant = tenantID
	return f.body, f.digest, f.err
}

func TestTheCatalogueIsParsedOncePerDigest(t *testing.T) {
	ctx := context.Background()
	source := &fakeSource{body: json.RawMessage(body), digest: "sha256:one"}
	service := catalog.NewService(source)

	first, err := service.Catalogue(ctx, "")
	if err != nil {
		t.Fatalf("catalogue: %v", err)
	}
	if len(first.Roles) == 0 {
		t.Fatal("the catalogue served no roles")
	}

	// A second ask still resolves (the registry is the authority on which
	// digest is current) but does not re-parse.
	if _, err := service.Catalogue(ctx, ""); err != nil {
		t.Fatalf("second: %v", err)
	}
	if source.reads != 2 {
		t.Fatalf("%d registry reads for two asks", source.reads)
	}

	// A newly published catalogue has a new digest and misses the cache,
	// which is how a publication reaches the wizard without invalidation.
	source.digest = "sha256:two"
	if _, err := service.Catalogue(ctx, ""); err != nil {
		t.Fatalf("after publication: %v", err)
	}
}

func TestTheTenantsOwnViewIsAskedFor(t *testing.T) {
	source := &fakeSource{body: json.RawMessage(body), digest: "sha256:one"}
	service := catalog.NewService(source)

	if _, err := service.Catalogue(context.Background(), "tn_1"); err != nil {
		t.Fatalf("catalogue: %v", err)
	}
	if source.tenant != "tn_1" {
		t.Fatalf("the registry was asked for %q", source.tenant)
	}
}

func TestAnUnresolvableOrIncoherentCatalogueIsSurfaced(t *testing.T) {
	ctx := context.Background()

	missing := catalog.NewService(&fakeSource{err: errors.New("no such artifact")})
	if _, err := missing.Catalogue(ctx, ""); err == nil {
		t.Fatal("an unresolvable catalogue was served")
	}

	// A published document that does not parse is a publication bug, and
	// serving an empty catalogue instead would hide it.
	broken := catalog.NewService(&fakeSource{
		body: json.RawMessage(`{"disciplines":[],"shapes":[{"id":"s","name":"S",` +
			`"description":"d","minutes":[15]}],"personas":[],"roles":[{"id":"r",` +
			`"discipline":"ghost","title":"T","organisation":"O","competencies":[],` +
			`"shapes":["s"]}]}`),
		digest: "sha256:broken",
	})
	if _, err := broken.Catalogue(ctx, ""); err == nil {
		t.Fatal("an incoherent catalogue was served as if it were fine")
	}
}

func TestCompetencyIDIsAStableSlug(t *testing.T) {
	// Evidence links to competencies by this identifier, so the same name
	// must always produce the same one, and two names that differ only in
	// punctuation must not collide silently with something else.
	for name, want := range map[string]string{
		"Systems design":                "systems-design",
		"Debugging & incident response": "debugging-incident-response",
		"  Leading   teams  ":           "leading-teams",
		"Clinical Reasoning":            "clinical-reasoning",
		"C++ fluency":                   "c-fluency",
		"":                              "",
	} {
		if got := catalog.CompetencyID(name); got != want {
			t.Fatalf("CompetencyID(%q) = %q, want %q", name, got, want)
		}
	}
	// Stable across calls, which is what makes stored evidence resolvable.
	if catalog.CompetencyID("Systems design") != catalog.CompetencyID("systems design") {
		t.Fatal("the same words produced two identifiers")
	}
}
