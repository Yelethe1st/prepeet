package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/platform/authz"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
)

/*
 * TEN-01's last criterion: a read-only user sees the settings without
 * controls, not a broken form.
 *
 * That needed a capability the catalogue did not have. Only
 * tenant.settings_manage existed, so a viewer, whose entire role is oversight
 * without authority, could not open the page at all. A 403 on the settings
 * screen reads as a broken product rather than as a boundary, and the recruiter
 * role's own description already drew the line in the right place: it says they
 * cannot change how the workspace is configured, not that they cannot see it.
 *
 * Whether the caller may change what they are reading is served rather than
 * inferred. A browser deciding from a role name would be a second copy of the
 * authorization rules, and the copy that drifts is the one nobody re-reads.
 */

type stubSettings struct {
	settings  api.TenantSettings
	err       error
	saveErr   error
	saved     bool
	sawTenant string
}

func (s *stubSettings) Save(_ context.Context, tenantID, _ string, next api.TenantSettings) (api.TenantSettings, error) {
	s.sawTenant = tenantID
	if s.saveErr != nil {
		return api.TenantSettings{}, s.saveErr
	}
	s.saved = true
	next.Version = s.settings.Version + 1
	return next, nil
}

func (s *stubSettings) Current(_ context.Context, tenantID string) (api.TenantSettings, error) {
	s.sawTenant = tenantID
	return s.settings, s.err
}

func settingsFixture() *stubSettings {
	return &stubSettings{settings: api.TenantSettings{
		Version:     3,
		LegalName:   "Northwind Health Limited",
		DisplayName: "Northwind Health",
		ChangedBy:   "00000000-0000-7000-8000-0000000000a1",
	}}
}

func serveSettings(t *testing.T, settings *stubSettings, held ...authz.Capability) http.Handler {
	t.Helper()
	identity := &fakeIdentity{
		principal: api.Principal{
			UserID: progressionUser, ActiveTenantID: "00000000-0000-7000-8000-00000000d001",
		},
		// Always non-nil, so "holds nothing" is distinguishable from "this test
		// does not care about capabilities", which is what a nil means to the
		// fake and what every test written before capabilities mattered relies
		// on.
		allowed: append([]authz.Capability{}, held...),
	}
	handler, err := api.NewServer(api.ServerConfig{
		Identity: identity, Candidates: &fakeCandidates{}, Documents: &fakeDocuments{},
		Catalog: &fakeCatalog{}, Interviews: &fakeInterviews{}, Members: &fakeMembers{},
		Billing: &fakeBilling{}, Progression: &stubProgression{},
		SensitiveReads: &recordingAuditor{}, Settings: settings,
		Recruiting:  &stubRecruiting{},
		Environment: config.EnvironmentLocal,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return handler
}

func readSettings(t *testing.T, handler http.Handler) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/tenant/settings", nil)
	request.AddCookie(sessionCookie())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	var body map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	return recorder.Code, body
}

func TestAReadOnlyMemberCanReadTheSettings(t *testing.T) {
	// The whole point. Before this capability existed the same request was a
	// 403, and a person whose role is oversight met a wall where a page should
	// have been.
	status, body := readSettings(t, serveSettings(t, settingsFixture(), authz.TenantSettingsRead))

	if status != http.StatusOK {
		t.Fatalf("a read-only member got %d", status)
	}
	if body["version"] != float64(3) {
		t.Fatalf("the version did not survive: %v", body["version"])
	}
}

func TestAReadOnlyMemberIsToldTheyMayNotEdit(t *testing.T) {
	_, body := readSettings(t, serveSettings(t, settingsFixture(), authz.TenantSettingsRead))

	if body["editable"] != false {
		t.Fatalf("a member holding only the read capability was told they may edit: %v", body["editable"])
	}
}

func TestAnAdministratorIsToldTheyMayEdit(t *testing.T) {
	handler := serveSettings(t, settingsFixture(),
		authz.TenantSettingsRead, authz.TenantSettingsManage)

	_, body := readSettings(t, handler)

	if body["editable"] != true {
		t.Fatalf("a member holding manage was not offered controls: %v", body["editable"])
	}
}

func TestSomebodyWithNeitherCapabilityIsRefused(t *testing.T) {
	// Reading is a lesser authority, not no authority. A person outside the
	// workspace still sees nothing.
	status, _ := readSettings(t, serveSettings(t, settingsFixture()))

	if status == http.StatusOK {
		t.Fatalf("a member holding neither capability read the settings")
	}
}

func TestTheSettingsComeFromTheSessionsOwnWorkspace(t *testing.T) {
	// The tenant is never taken from the request. One that was would let a
	// member of one workspace read another's configuration by asking.
	settings := settingsFixture()
	readSettings(t, serveSettings(t, settings, authz.TenantSettingsRead))

	if settings.sawTenant != "00000000-0000-7000-8000-00000000d001" {
		t.Fatalf("the store was asked for %q rather than the session's workspace", settings.sawTenant)
	}
}

func TestSavingRefusesAStaleVersion(t *testing.T) {
	// Two administrators editing the same document should be told they
	// collided, not have one of them quietly lose their work. The store
	// enforces it; this is the handler carrying the refusal through as a
	// conflict rather than as a generic failure.
	settings := settingsFixture()
	settings.saveErr = api.ErrSettingsConflict
	handler := serveSettings(t, settings,
		authz.TenantSettingsRead, authz.TenantSettingsManage)

	status, _ := saveSettings(t, handler, 1)

	if status != http.StatusConflict {
		t.Fatalf("a stale version answered %d, want 409", status)
	}
}

func TestAReadOnlyMemberCannotSave(t *testing.T) {
	// The read capability admits somebody to the page and to nothing else.
	settings := settingsFixture()
	handler := serveSettings(t, settings, authz.TenantSettingsRead)

	status, _ := saveSettings(t, handler, 3)

	if status != http.StatusForbidden {
		t.Fatalf("a read-only member saving answered %d, want 403", status)
	}
	if settings.saved {
		t.Fatal("a read-only member's change reached the store")
	}
}

func TestASaveGoesToTheSessionsOwnWorkspace(t *testing.T) {
	settings := settingsFixture()
	handler := serveSettings(t, settings,
		authz.TenantSettingsRead, authz.TenantSettingsManage)

	saveSettings(t, handler, 3)

	if settings.sawTenant != "00000000-0000-7000-8000-00000000d001" {
		t.Fatalf("the save went to %q rather than the session's workspace", settings.sawTenant)
	}
}

func saveSettings(t *testing.T, handler http.Handler, version int) (int, map[string]any) {
	t.Helper()
	body := `{"version":` + strconv.Itoa(version) +
		`,"settings":{"organisation":{"legal_name":"Northwind Health Limited","display_name":"Northwind Health"},` +
		`"defaults":{},"candidate_experience":{},"notifications":{}}}`
	request := httptest.NewRequest(http.MethodPut, "/api/v1/tenant/settings",
		strings.NewReader(body))
	request.Header.Set("content-type", "application/json")
	request.AddCookie(sessionCookie())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	var decoded map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &decoded)
	return recorder.Code, decoded
}
