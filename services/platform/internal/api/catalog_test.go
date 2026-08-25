package api_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
)

// The catalogue surface. What matters at this boundary: the collections come
// from the port - never a list compiled into a binary - the caller's active
// tenant reaches the port so a tenant's catalogue override applies, and the
// combination data (a role's shapes, a shape's minutes) survives the wire.

type fakeCatalog struct {
	catalogue api.CatalogueView
	err       error
	tenants   []string
}

func (f *fakeCatalog) Catalogue(_ context.Context, tenantID string) (api.CatalogueView, error) {
	f.tenants = append(f.tenants, tenantID)
	return f.catalogue, f.err
}

func catalogueFixture() api.CatalogueView {
	return api.CatalogueView{
		Disciplines: []api.Discipline{{ID: "nursing", Name: "Nursing"}},
		Roles: []api.CatalogRole{{
			ID: "rl_rn", Discipline: "nursing", Title: "Registered Nurse",
			Organisation: "Health system",
			Competencies: []string{"Clinical reasoning"},
			Shapes:       []string{"shape_panel"},
		}},
		Shapes: []api.InterviewShape{{
			ID: "shape_panel", Name: "Panel simulation",
			Description: "Rotating viewpoints.", Minutes: []int{40, 60},
		}},
		Personas: []api.CatalogPersona{{
			ID: "per_lena", Name: "Lena", Style: "Panel chair", Voice: "Formal",
			Description: "Runs a panel.", BestFor: "Panels", Shapes: []string{"shape_panel"},
		}},
	}
}

func serveCatalog(t *testing.T, catalog *fakeCatalog) http.Handler {
	t.Helper()
	handler, err := api.NewServer(api.ServerConfig{
		Identity: &fakeIdentity{principal: api.Principal{
			UserID: "00000000-0000-7000-8000-0000000000f9", ActiveTenantID: "tn_1",
		}},
		Candidates:  &fakeCandidates{},
		Documents:   &fakeDocuments{},
		Catalog:     catalog,
		Interviews:  &fakeInterviews{},
		Environment: config.EnvironmentLocal,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return handler
}

func getCatalog(t *testing.T, handler http.Handler, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestTheCatalogueNeedsASession(t *testing.T) {
	handler := serveCatalog(t, &fakeCatalog{catalogue: catalogueFixture()})

	for _, path := range []string{
		"/api/v1/catalog/disciplines", "/api/v1/catalog/roles",
		"/api/v1/catalog/interview-shapes", "/api/v1/catalog/personas",
	} {
		if response := getCatalog(t, handler, path); response.Code != http.StatusUnauthorized {
			t.Errorf("%s without a session got %d, want 401", path, response.Code)
		}
	}
}

func TestTheCollectionsArriveWithTheirCombinationRules(t *testing.T) {
	catalog := &fakeCatalog{catalogue: catalogueFixture()}
	handler := serveCatalog(t, catalog)

	var roles struct {
		Roles []struct {
			Discipline string   `json:"discipline"`
			Shapes     []string `json:"shapes"`
		} `json:"roles"`
	}
	response := getCatalog(t, handler, "/api/v1/catalog/roles", sessionCookie())
	if response.Code != http.StatusOK {
		t.Fatalf("roles: %d: %s", response.Code, response.Body)
	}
	decodeInto(t, response, &roles)
	if roles.Roles[0].Discipline != "nursing" || roles.Roles[0].Shapes[0] != "shape_panel" {
		t.Fatalf("the combination rules did not survive the wire: %+v", roles)
	}

	var shapes struct {
		Shapes []struct {
			Minutes []int `json:"minutes"`
		} `json:"shapes"`
	}
	response = getCatalog(t, handler, "/api/v1/catalog/interview-shapes", sessionCookie())
	decodeInto(t, response, &shapes)
	if len(shapes.Shapes[0].Minutes) != 2 {
		t.Fatalf("shapes = %+v", shapes)
	}

	var personas struct {
		Personas []struct {
			BestFor string   `json:"best_for"`
			Shapes  []string `json:"shapes"`
		} `json:"personas"`
	}
	response = getCatalog(t, handler, "/api/v1/catalog/personas", sessionCookie())
	decodeInto(t, response, &personas)
	if personas.Personas[0].BestFor != "Panels" {
		t.Fatalf("personas = %+v", personas)
	}

	var disciplines struct {
		Disciplines []struct {
			Name string `json:"name"`
		} `json:"disciplines"`
	}
	response = getCatalog(t, handler, "/api/v1/catalog/disciplines", sessionCookie())
	decodeInto(t, response, &disciplines)
	if disciplines.Disciplines[0].Name != "Nursing" {
		t.Fatalf("disciplines = %+v", disciplines)
	}
}

func TestTheActiveTenantReachesThePort(t *testing.T) {
	// A tenant's catalogue override is the registry's tenant pointer; the
	// port must learn which tenant is asking or every tenant gets the
	// platform's catalogue.
	catalog := &fakeCatalog{catalogue: catalogueFixture()}
	handler := serveCatalog(t, catalog)

	getCatalog(t, handler, "/api/v1/catalog/roles", sessionCookie())

	if len(catalog.tenants) != 1 || catalog.tenants[0] != "tn_1" {
		t.Fatalf("the port saw tenants %v, want the session's tn_1", catalog.tenants)
	}
}

func TestACatalogueFailureIsAnHonestError(t *testing.T) {
	handler := serveCatalog(t, &fakeCatalog{err: errors.New("the registry is unreachable")})

	response := getCatalog(t, handler, "/api/v1/catalog/disciplines", sessionCookie())
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500: %s", response.Code, response.Body)
	}
	if body := response.Body.String(); len(body) == 0 {
		t.Fatal("no error envelope")
	}
}
