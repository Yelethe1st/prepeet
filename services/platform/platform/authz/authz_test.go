package authz_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/platform/authz"
)

const (
	tenantA = "00000000-0000-7000-8000-00000000000a"
	tenantB = "00000000-0000-7000-8000-00000000000b"
)

// now is fixed so a test never depends on the wall clock.
var now = time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)

func recruiter(capabilities ...authz.Capability) authz.Context {
	return authz.Context{
		Subject:      authz.Subject{ID: "usr_1", Type: authz.SubjectUser},
		ActiveTenant: tenantA,
		MembershipID: "mem_1",
		Capabilities: capabilities,
		IssuedAt:     now.Add(-time.Minute),
		ExpiresAt:    now.Add(time.Hour),
	}
}

// ─────────────────────────────────────────────────────────── deny by default

// An unknown capability is denied rather than treated as harmless. A typo in a
// handler must fail closed, not skip the check.
func TestUnknownCapabilityIsDenied(t *testing.T) {
	t.Parallel()

	ctx := recruiter("invitation.manage")

	decision := ctx.Can(authz.Capability("invitation.manage.typo"), authz.Request{Tenant: tenantA}, now)

	if decision.Allowed {
		t.Error("an unknown capability was allowed, want it denied")
	}
	if decision.Reason == "" {
		t.Error("decision has no reason, and an audit record needs one")
	}
}

func TestCapabilityNotHeldIsDenied(t *testing.T) {
	t.Parallel()

	ctx := recruiter(authz.InvitationRead)

	decision := ctx.Can(authz.InvitationManage, authz.Request{Tenant: tenantA}, now)

	if decision.Allowed {
		t.Error("a capability the subject does not hold was allowed")
	}
}

func TestHeldCapabilityIsAllowed(t *testing.T) {
	t.Parallel()

	ctx := recruiter(authz.InvitationManage)

	decision := ctx.Can(authz.InvitationManage, authz.Request{Tenant: tenantA}, now)

	if !decision.Allowed {
		t.Errorf("a held capability was denied: %s", decision.Reason)
	}
}

func TestEmptyContextAllowsNothing(t *testing.T) {
	t.Parallel()

	var ctx authz.Context

	for _, capability := range authz.All() {
		if decision := ctx.Can(capability, authz.Request{Tenant: tenantA}, now); decision.Allowed {
			t.Errorf("the zero context allowed %q, want deny by default", capability)
		}
	}
}

// ─────────────────────────────────────────────────────────────────── expiry

// An expired context is denied whatever it holds. Capabilities outliving their
// session is how a revoked recruiter keeps reading evidence.
func TestExpiredContextIsDenied(t *testing.T) {
	t.Parallel()

	ctx := recruiter(authz.InvitationManage)
	ctx.ExpiresAt = now.Add(-time.Second)

	decision := ctx.Can(authz.InvitationManage, authz.Request{Tenant: tenantA}, now)

	if decision.Allowed {
		t.Error("an expired context was allowed")
	}
	if !strings.Contains(strings.ToLower(decision.Reason), "expired") {
		t.Errorf("reason = %q, want it to say the context expired", decision.Reason)
	}
}

func TestContextExpiringExactlyNowIsDenied(t *testing.T) {
	t.Parallel()

	ctx := recruiter(authz.InvitationManage)
	ctx.ExpiresAt = now

	if ctx.Can(authz.InvitationManage, authz.Request{Tenant: tenantA}, now).Allowed {
		t.Error("a context expiring exactly now was allowed; expiry must not be inclusive")
	}
}

// ─────────────────────────────────────────────────────────────────── tenancy

// The single most important rule in the product. Tenant authority is bounded
// to one tenant, and asking about another is denied even with the capability.
func TestTenantCapabilityIsDeniedForAnotherTenant(t *testing.T) {
	t.Parallel()

	ctx := recruiter(authz.InvitationManage)

	decision := ctx.Can(authz.InvitationManage, authz.Request{Tenant: tenantB}, now)

	if decision.Allowed {
		t.Error("a tenant-scoped capability was allowed against another tenant")
	}
}

// A tenant capability with no active tenant is denied rather than treated as
// applying everywhere.
func TestTenantCapabilityRequiresAnActiveTenant(t *testing.T) {
	t.Parallel()

	ctx := recruiter(authz.InvitationManage)
	ctx.ActiveTenant = ""

	if ctx.Can(authz.InvitationManage, authz.Request{Tenant: tenantA}, now).Allowed {
		t.Error("a tenant capability was allowed with no active tenant")
	}
}

func TestTenantCapabilityRequiresTheRequestToNameATenant(t *testing.T) {
	t.Parallel()

	ctx := recruiter(authz.InvitationManage)

	if ctx.Can(authz.InvitationManage, authz.Request{}, now).Allowed {
		t.Error("a tenant capability was allowed for a request naming no tenant")
	}
}

// ──────────────────────────────────────────────────────────────────── scope

// Membership is not scope. A recruiter added to a tenant is not thereby
// authorized over every campaign in it.
func TestScopedCapabilityRequiresTheMatchingScope(t *testing.T) {
	t.Parallel()

	ctx := recruiter(authz.EvaluationReview)
	ctx.Scopes = []authz.Scope{{Kind: authz.ScopeCampaign, Value: "cmp_icu_autumn"}}

	allowed := ctx.Can(authz.EvaluationReview,
		authz.Request{Tenant: tenantA, Scope: &authz.Scope{Kind: authz.ScopeCampaign, Value: "cmp_icu_autumn"}}, now)
	denied := ctx.Can(authz.EvaluationReview,
		authz.Request{Tenant: tenantA, Scope: &authz.Scope{Kind: authz.ScopeCampaign, Value: "cmp_fpa_q3"}}, now)

	if !allowed.Allowed {
		t.Errorf("the scoped campaign was denied: %s", allowed.Reason)
	}
	if denied.Allowed {
		t.Error("a campaign outside the subject's scope was allowed")
	}
}

// A scoped capability asked without a scope is denied. Otherwise a list
// endpoint could return every campaign by simply not mentioning one, which is
// the projection leak the specification's test matrix calls out.
func TestScopedCapabilityWithoutARequestScopeIsDenied(t *testing.T) {
	t.Parallel()

	ctx := recruiter(authz.EvaluationReview)
	ctx.Scopes = []authz.Scope{{Kind: authz.ScopeCampaign, Value: "cmp_icu_autumn"}}

	if ctx.Can(authz.EvaluationReview, authz.Request{Tenant: tenantA}, now).Allowed {
		t.Error("a scoped capability was allowed with no scope named, which would leak a whole list")
	}
}

// A subject with no scopes at all holds tenant-wide authority only where the
// capability is not scoped. For a scoped capability, no scopes means no access.
func TestNoScopesMeansNoScopedAccess(t *testing.T) {
	t.Parallel()

	ctx := recruiter(authz.EvaluationReview)

	decision := ctx.Can(authz.EvaluationReview,
		authz.Request{Tenant: tenantA, Scope: &authz.Scope{Kind: authz.ScopeCampaign, Value: "cmp_icu_autumn"}}, now)

	if decision.Allowed {
		t.Error("a subject with no campaign scopes was allowed a scoped capability")
	}
}

// ───────────────────────────────────────────── practice and screen separation

// The boundary the whole product rests on. Tenant authority never reaches a
// candidate's own practice data, whatever capabilities it holds.
func TestTenantAuthorityCannotReachCandidatePracticeData(t *testing.T) {
	t.Parallel()

	// Deliberately over-provisioned: every capability in the catalogue.
	ctx := recruiter(authz.All()...)
	ctx.Scopes = []authz.Scope{{Kind: authz.ScopeCampaign, Value: "cmp_icu_autumn"}}

	for _, capability := range authz.All() {
		if !strings.HasPrefix(string(capability), "candidate.") &&
			!strings.HasPrefix(string(capability), "session.read_own") {
			continue
		}
		decision := ctx.Can(capability, authz.Request{
			Tenant: tenantA,
			Owner:  "usr_someone_else",
		}, now)
		if decision.Allowed {
			t.Errorf("tenant authority was allowed %q over a candidate's own data", capability)
		}
	}
}

// A candidate's own-data capability applies to their own data and nothing else.
func TestCandidateOwnCapabilityAppliesOnlyToTheirOwnData(t *testing.T) {
	t.Parallel()

	ctx := authz.Context{
		Subject:      authz.Subject{ID: "usr_daniel", Type: authz.SubjectUser},
		Capabilities: []authz.Capability{authz.CandidatePracticeReadOwn},
		IssuedAt:     now.Add(-time.Minute),
		ExpiresAt:    now.Add(time.Hour),
	}

	own := ctx.Can(authz.CandidatePracticeReadOwn, authz.Request{Owner: "usr_daniel"}, now)
	other := ctx.Can(authz.CandidatePracticeReadOwn, authz.Request{Owner: "usr_amara"}, now)

	if !own.Allowed {
		t.Errorf("a candidate was denied their own practice data: %s", own.Reason)
	}
	if other.Allowed {
		t.Error("a candidate was allowed another candidate's practice data")
	}
}

// An own-data capability asked without an owner is denied, for the same reason
// a scoped capability without a scope is: it would authorize a whole list.
func TestOwnCapabilityWithoutAnOwnerIsDenied(t *testing.T) {
	t.Parallel()

	ctx := authz.Context{
		Subject:      authz.Subject{ID: "usr_daniel", Type: authz.SubjectUser},
		Capabilities: []authz.Capability{authz.CandidatePracticeReadOwn},
		IssuedAt:     now.Add(-time.Minute),
		ExpiresAt:    now.Add(time.Hour),
	}

	if ctx.Can(authz.CandidatePracticeReadOwn, authz.Request{}, now).Allowed {
		t.Error("an own-data capability was allowed with no owner named")
	}
}

// ─────────────────────────────────────────────────────────────── platform

// Tenant authority and platform authority are separate. A tenant admin holding
// every tenant capability is still not a platform operator.
func TestTenantAuthorityIsNotPlatformAuthority(t *testing.T) {
	t.Parallel()

	ctx := recruiter(authz.TenantSettingsManage, authz.TenantMemberManage, authz.InvitationManage)

	if ctx.Can(authz.PlatformAnalyticsRead, authz.Request{}, now).Allowed {
		t.Error("tenant authority was allowed a platform capability")
	}
}

// Platform capabilities are not bounded by the active tenant, but privileged
// ones still require an active elevation.
func TestPrivilegedPlatformCapabilityRequiresAnElevation(t *testing.T) {
	t.Parallel()

	ctx := authz.Context{
		Subject:      authz.Subject{ID: "usr_operator", Type: authz.SubjectUser},
		Capabilities: []authz.Capability{authz.PlatformOperationsExecute},
		IssuedAt:     now.Add(-time.Minute),
		ExpiresAt:    now.Add(time.Hour),
	}

	without := ctx.Can(authz.PlatformOperationsExecute, authz.Request{}, now)

	ctx.Elevation = &authz.Elevation{
		GrantID:   "grant_1",
		Reason:    "incident 4821",
		Ticket:    "OPS-4821",
		ExpiresAt: now.Add(30 * time.Minute),
	}
	with := ctx.Can(authz.PlatformOperationsExecute, authz.Request{}, now)

	if without.Allowed {
		t.Error("a privileged platform capability was allowed with no elevation")
	}
	if !with.Allowed {
		t.Errorf("a privileged platform capability was denied under a valid elevation: %s", with.Reason)
	}
}

func TestExpiredElevationDoesNotGrantPrivilege(t *testing.T) {
	t.Parallel()

	ctx := authz.Context{
		Subject:      authz.Subject{ID: "usr_operator", Type: authz.SubjectUser},
		Capabilities: []authz.Capability{authz.PlatformOperationsExecute},
		IssuedAt:     now.Add(-time.Minute),
		ExpiresAt:    now.Add(time.Hour),
		Elevation: &authz.Elevation{
			GrantID:   "grant_1",
			Reason:    "incident 4821",
			Ticket:    "OPS-4821",
			ExpiresAt: now.Add(-time.Second),
		},
	}

	if ctx.Can(authz.PlatformOperationsExecute, authz.Request{}, now).Allowed {
		t.Error("an expired elevation still granted privilege")
	}
}

// An elevation with no reason or ticket is not an elevation. The audit record
// is the point of it.
func TestElevationWithoutReasonOrTicketIsRejected(t *testing.T) {
	t.Parallel()

	base := authz.Context{
		Subject:      authz.Subject{ID: "usr_operator", Type: authz.SubjectUser},
		Capabilities: []authz.Capability{authz.PlatformOperationsExecute},
		IssuedAt:     now.Add(-time.Minute),
		ExpiresAt:    now.Add(time.Hour),
	}

	for name, elevation := range map[string]authz.Elevation{
		"no reason": {GrantID: "g", Ticket: "OPS-1", ExpiresAt: now.Add(time.Hour)},
		"no ticket": {GrantID: "g", Reason: "incident", ExpiresAt: now.Add(time.Hour)},
		"no grant":  {Reason: "incident", Ticket: "OPS-1", ExpiresAt: now.Add(time.Hour)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := base
			ctx.Elevation = &elevation
			if ctx.Can(authz.PlatformOperationsExecute, authz.Request{}, now).Allowed {
				t.Errorf("an elevation with %s was accepted", name)
			}
		})
	}
}

// ────────────────────────────────────────────────────────── step-up auth

// Publishing a calibration changes how candidates are evaluated, and
// destructive retention changes delete evidence. Both require recent
// authentication rather than a session opened this morning.
func TestSensitiveCapabilityRequiresRecentAuthentication(t *testing.T) {
	t.Parallel()

	ctx := recruiter(authz.RubricPublish)
	ctx.AuthenticatedAt = now.Add(-2 * time.Hour)

	stale := ctx.Can(authz.RubricPublish, authz.Request{Tenant: tenantA}, now)

	ctx.AuthenticatedAt = now.Add(-time.Minute)
	fresh := ctx.Can(authz.RubricPublish, authz.Request{Tenant: tenantA}, now)

	if stale.Allowed {
		t.Error("a sensitive capability was allowed on stale authentication")
	}
	if !fresh.Allowed {
		t.Errorf("a sensitive capability was denied on fresh authentication: %s", fresh.Reason)
	}
}

func TestOrdinaryCapabilityDoesNotRequireRecentAuthentication(t *testing.T) {
	t.Parallel()

	ctx := recruiter(authz.InvitationRead)
	ctx.AuthenticatedAt = now.Add(-8 * time.Hour)

	if decision := ctx.Can(authz.InvitationRead, authz.Request{Tenant: tenantA}, now); !decision.Allowed {
		t.Errorf("an ordinary capability was denied on an old session: %s", decision.Reason)
	}
}

// ───────────────────────────────────────────────────────────── delegation

// A user cannot grant authority they do not hold, which is how privilege
// escalation happens through an invitation form.
func TestCannotGrantACapabilityNotHeld(t *testing.T) {
	t.Parallel()

	ctx := recruiter(authz.TenantMemberManage, authz.InvitationRead)

	if err := ctx.CanGrant([]authz.Capability{authz.InvitationRead}); err != nil {
		t.Errorf("granting a held capability failed: %v", err)
	}
	if err := ctx.CanGrant([]authz.Capability{authz.RubricPublish}); err == nil {
		t.Error("granting a capability the subject does not hold succeeded")
	}
}

func TestGrantingRequiresTheMemberManagementCapability(t *testing.T) {
	t.Parallel()

	ctx := recruiter(authz.InvitationRead)

	if err := ctx.CanGrant([]authz.Capability{authz.InvitationRead}); err == nil {
		t.Error("a subject without member management granted a capability")
	}
}

// ───────────────────────────────────────────────────────────── catalogue

func TestCatalogueIsWellFormed(t *testing.T) {
	t.Parallel()

	all := authz.All()
	if len(all) == 0 {
		t.Fatal("the catalogue is empty")
	}

	seen := make(map[authz.Capability]struct{}, len(all))
	for _, capability := range all {
		if _, duplicate := seen[capability]; duplicate {
			t.Errorf("%q appears twice in the catalogue", capability)
		}
		seen[capability] = struct{}{}

		parts := strings.Split(string(capability), ".")
		if len(parts) < 2 {
			t.Errorf("%q is not namespaced as domain.action", capability)
		}
		if strings.ToLower(string(capability)) != string(capability) {
			t.Errorf("%q is not lowercase", capability)
		}
	}
}

// The catalogue is a contract, not a list of page names. A capability called
// after a page would have to change when the page is renamed, and the two have
// no reason to move together.
//
// "screen" is deliberately absent from the banned list: it is this product's
// word for employer screening mode, as in evaluation.read_screen, and is a
// domain term rather than an interface one.
func TestCatalogueContainsNoPageSpecificNames(t *testing.T) {
	t.Parallel()

	for _, capability := range authz.All() {
		for _, banned := range []string{"page", "button", "modal", "dashboard", "sidebar", "widget"} {
			if strings.Contains(string(capability), banned) {
				t.Errorf("%q names an interface element rather than an authority", capability)
			}
		}
	}
}

func TestEveryCapabilityDeclaresItsRequirements(t *testing.T) {
	t.Parallel()

	for _, capability := range authz.All() {
		if _, known := authz.Describe(capability); !known {
			t.Errorf("%q is in the catalogue but has no declared requirements", capability)
		}
	}
}

// A decision that cannot be explained cannot be audited, and every denial in
// this product may end up in front of someone asking why.
func TestEveryDecisionCarriesAReason(t *testing.T) {
	t.Parallel()

	ctx := recruiter(authz.InvitationManage)

	for _, request := range []authz.Request{
		{Tenant: tenantA},
		{Tenant: tenantB},
		{},
	} {
		if decision := ctx.Can(authz.InvitationManage, request, now); decision.Reason == "" {
			t.Errorf("decision for %+v has no reason", request)
		}
	}
}
