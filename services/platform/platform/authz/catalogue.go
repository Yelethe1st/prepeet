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

import "slices"

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

// The capabilities themselves and their requirements are generated from
// packages/contracts/authz/capabilities.yaml into catalogue.gen.go.
//
// The contract is the source rather than this file, per ADR-0004, and for the
// reason that ADR gives for the API contract: which authority reaches a
// candidate's practice history, what needs recent authentication, and what needs
// an elevation with a ticket are questions legal and security must be able to
// answer from one artifact without reading Go.
//
// What stays here is the vocabulary the generated file is written in, and the
// prose explaining why the vocabulary is shaped this way. A generator cannot
// produce that, and a contract carrying it would be describing an
// implementation.

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

// Catalogue returns every capability the system defines.
//
// Sorted, so a caller iterating it produces stable output. Exported because a
// capability that exists and cannot be enumerated is one that cannot be
// reviewed, published to a client, or checked against its contract.
func Catalogue() []Capability {
	names := make([]Capability, 0, len(catalogue))
	for capability := range catalogue {
		names = append(names, capability)
	}
	slices.Sort(names)
	return names
}

// RequirementOf returns what a capability requires, and whether it is known.
//
// The second result is false for an unknown capability rather than the zero
// Requirement being returned alone, because a zero Requirement requires nothing
// and a caller that ignored the boolean would read a typo as an unrestricted
// capability.
func RequirementOf(capability Capability) (Requirement, bool) {
	requirement, known := catalogue[capability]
	return requirement, known
}

// Roles returns every role the catalogue defines, sorted.
func Roles() []Role {
	names := make([]Role, 0, len(bundles))
	for role := range bundles {
		names = append(names, role)
	}
	slices.Sort(names)
	return names
}

// CapabilitiesOf returns what a role grants.
//
// An unknown role grants nothing. There is no branch for that, and the absence
// is deliberate rather than an oversight: a missing key yields the zero slice,
// so deny by default is a property of the lookup instead of a check somebody
// could remove. An earlier version tested for the key explicitly, and deleting
// that test left every assertion green, which is how it was noticed.
//
// The result is a copy. Handing out the package's own slice would let a caller
// grant itself authority by appending to what it was given, which is a strange
// way to be compromised and an easy one.
func CapabilitiesOf(role Role) []Capability {
	return slices.Clone(bundles[role])
}

// RoleRequiresMembership reports whether a role is held through belonging to a
// tenant.
//
// The untenanted role is what somebody holds with no membership at all, which
// is every candidate practising alone. It is a role rather than a special case
// so that "what can this person do" has one answer.
func RoleRequiresMembership(role Role) bool {
	return role != RoleCandidate
}
