// Package authz decides what a subject is allowed to do.
//
// It is the single code path every module calls, so that authorization is one
// decision made in one place rather than a convention repeated in each handler.
// docs/architecture/authorization-model.md fixes the rules; this package
// implements them.
//
// Four properties matter more than the rest:
//
// Deny by default. An unknown capability, an empty context and a missing tenant
// all fail closed. A typo in a handler must skip nothing.
//
// Tenant authority is bounded. Holding a capability says nothing about which
// tenant it applies to, and asking about another tenant is denied even with it.
//
// Membership is not scope. A recruiter added to a tenant is not thereby
// authorized over every campaign in it, and a scoped capability asked without a
// scope is denied, because otherwise a list endpoint could return everything by
// declining to mention one.
//
// A candidate's practice data is unreachable from tenant authority. Not
// filtered out, not hidden: denied structurally, because an employer seeing
// rehearsals a candidate believed were private is the failure this product
// cannot have.
//
// This package decides. It does not enforce alone: repository predicates,
// PostgreSQL row-level security under ADR-0002, object scoping and audit sit
// behind it, and each is expected to hold on its own.
//
// Implements part of IAM-04.
package authz

// Capability is one unit of authority.
//
// The catalogue is a versioned contract. Names describe authority over a
// resource, never an interface element: a capability called after a screen
// would have to change when the screen is renamed, and the two have no reason
// to move together.
type Capability string

// Requirement declares what a capability needs beyond being held.
type Requirement struct {
	// Tenant means the capability is exercised inside one tenant, and the
	// request must name the same one the subject is active in.
	Tenant bool
	// Scope means tenant membership is not enough: the subject needs an
	// explicit assignment covering the resource.
	Scope ScopeKind
	// Owner means the capability applies to the subject's own data only.
	Owner bool
	// Platform means the capability belongs to platform authority, which is
	// separate from tenant authority rather than a superset of it.
	Platform bool
	// Privileged means an active elevation is required, with reason and ticket.
	Privileged bool
	// StepUp means the action is destructive or changes how candidates are
	// evaluated, so it needs recent authentication rather than a session opened
	// this morning.
	StepUp bool
}

// ScopeKind names a dimension a capability can be scoped along.
type ScopeKind string

const (
	ScopeNone     ScopeKind = ""
	ScopeCampaign ScopeKind = "campaign"
	ScopeRole     ScopeKind = "role"
)

// Scope is one assignment held by a subject or named by a request.
type Scope struct {
	Kind  ScopeKind
	Value string
}

// The catalogue. Adding a capability here without a requirement is impossible:
// the map is the catalogue, so every entry declares what it needs.
const (
	// Candidate. These are the candidate's own practice data, which no tenant
	// authority reaches.
	CandidateProfileReadOwn    Capability = "candidate.profile.read_own"
	CandidateProfileWriteOwn   Capability = "candidate.profile.write_own"
	CandidatePracticeReadOwn   Capability = "candidate.practice.read_own"
	CandidatePracticeDeleteOwn Capability = "candidate.practice.delete_own"

	// Session participation.
	SessionCreatePractice         Capability = "session.create_practice"
	SessionAcceptInvitation       Capability = "session.accept_invitation"
	SessionParticipate            Capability = "session.participate"
	SessionReadOwnPractice        Capability = "session.read_own_practice"
	SessionReadScreenConfirmation Capability = "session.read_screen_confirmation"

	// Recruiting, inside one tenant.
	CampaignRead         Capability = "campaign.read"
	CampaignManage       Capability = "campaign.manage"
	InvitationRead       Capability = "invitation.read"
	InvitationManage     Capability = "invitation.manage"
	EvaluationReadScreen Capability = "evaluation.read_screen"
	EvaluationReview     Capability = "evaluation.review"
	EvaluationCompare    Capability = "evaluation.compare"
	AppealManage         Capability = "appeal.manage"

	// Tenant configuration.
	RubricRead              Capability = "rubric.read"
	RubricDraft             Capability = "rubric.draft"
	RubricPublish           Capability = "rubric.publish"
	TenantMemberManage      Capability = "tenant.member_manage"
	TenantSettingsManage    Capability = "tenant.settings_manage"
	TenantRetentionManage   Capability = "tenant.retention_manage"
	TenantBillingRead       Capability = "tenant.billing_read"
	TenantIntegrationManage Capability = "tenant.integration_manage"

	// Platform. Separate authority, not a superset of tenant authority.
	PlatformAnalyticsRead     Capability = "platform.analytics_read"
	PlatformOperationsRead    Capability = "platform.operations_read"
	PlatformOperationsExecute Capability = "platform.operations_execute"
	PlatformQuotaManage       Capability = "platform.quota_manage"
	PlatformAuditRead         Capability = "platform.audit_read"
	PlatformPrivilegedElevate Capability = "platform.privileged_elevate"
)

// catalogue maps every capability to what it requires.
var catalogue = map[Capability]Requirement{
	CandidateProfileReadOwn:    {Owner: true},
	CandidateProfileWriteOwn:   {Owner: true},
	CandidatePracticeReadOwn:   {Owner: true},
	CandidatePracticeDeleteOwn: {Owner: true, StepUp: true},

	SessionCreatePractice:         {Owner: true},
	SessionAcceptInvitation:       {Owner: true},
	SessionParticipate:            {Owner: true},
	SessionReadOwnPractice:        {Owner: true},
	SessionReadScreenConfirmation: {Owner: true},

	CampaignRead:         {Tenant: true},
	CampaignManage:       {Tenant: true, Scope: ScopeCampaign},
	InvitationRead:       {Tenant: true},
	InvitationManage:     {Tenant: true},
	EvaluationReadScreen: {Tenant: true, Scope: ScopeCampaign},
	EvaluationReview:     {Tenant: true, Scope: ScopeCampaign},
	// Comparison additionally requires feature approval, which is a tenant
	// policy check rather than a capability check. See DEC-17.
	EvaluationCompare: {Tenant: true, Scope: ScopeCampaign},
	AppealManage:      {Tenant: true, Scope: ScopeCampaign},

	RubricRead:  {Tenant: true},
	RubricDraft: {Tenant: true},
	// Publishing changes how candidates are evaluated from that moment on.
	RubricPublish:        {Tenant: true, StepUp: true},
	TenantMemberManage:   {Tenant: true, StepUp: true},
	TenantSettingsManage: {Tenant: true},
	// Reducing retention destroys evidence, including evidence an appeal may
	// depend on.
	TenantRetentionManage:   {Tenant: true, StepUp: true},
	TenantBillingRead:       {Tenant: true},
	TenantIntegrationManage: {Tenant: true, StepUp: true},

	PlatformAnalyticsRead:  {Platform: true},
	PlatformOperationsRead: {Platform: true},
	// Executing an operation against tenant data is exceptional, so it is
	// reason-bound, ticket-bound and time-bound.
	PlatformOperationsExecute: {Platform: true, Privileged: true},
	PlatformQuotaManage:       {Platform: true, Privileged: true, StepUp: true},
	PlatformAuditRead:         {Platform: true, Privileged: true},
	PlatformPrivilegedElevate: {Platform: true, StepUp: true},
}

// All returns every capability in the catalogue, in a stable order.
func All() []Capability {
	all := make([]Capability, 0, len(catalogue))
	for capability := range catalogue {
		all = append(all, capability)
	}
	sortCapabilities(all)
	return all
}

// Describe returns a capability's requirements, and whether it is in the
// catalogue at all. A capability that is not is denied rather than assumed
// harmless.
func Describe(capability Capability) (Requirement, bool) {
	requirement, known := catalogue[capability]
	return requirement, known
}

// sortCapabilities orders capabilities lexically so callers and tests see a
// stable list rather than Go's randomised map order.
func sortCapabilities(capabilities []Capability) {
	for i := 1; i < len(capabilities); i++ {
		for j := i; j > 0 && capabilities[j] < capabilities[j-1]; j-- {
			capabilities[j], capabilities[j-1] = capabilities[j-1], capabilities[j]
		}
	}
}
