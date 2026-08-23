package authz

import (
	"errors"
	"fmt"
	"time"
)

// stepUpWindow is how recently a subject must have authenticated before a
// destructive or evaluation-changing action. Short enough that a borrowed
// laptop is not enough, long enough that a genuine administrator is not
// re-authenticating between every field.
const stepUpWindow = 15 * time.Minute

// SubjectType distinguishes a person from a workload.
type SubjectType string

const (
	SubjectUser    SubjectType = "user"
	SubjectService SubjectType = "service"
)

// Subject is who is acting.
type Subject struct {
	ID   string
	Type SubjectType
}

// Elevation is a time-bound platform grant.
//
// Reason and ticket are required rather than decorative: the audit record is
// the entire justification for elevation existing, and a grant nobody can
// explain afterwards is indistinguishable from an intrusion.
type Elevation struct {
	GrantID   string
	Reason    string
	Ticket    string
	ExpiresAt time.Time
}

func (e *Elevation) valid(at time.Time) bool {
	return e != nil &&
		e.GrantID != "" && e.Reason != "" && e.Ticket != "" &&
		at.Before(e.ExpiresAt)
}

// Context is the authority a request carries.
//
// It is built once per request from the session and membership, and is then
// read-only. Nothing downstream may add a capability to it.
type Context struct {
	Subject      Subject
	ActiveTenant string
	MembershipID string
	Capabilities []Capability
	Scopes       []Scope
	Elevation    *Elevation

	// AuthenticatedAt is when the subject last proved who they are, which is
	// not the same as when the session was issued.
	AuthenticatedAt time.Time
	IssuedAt        time.Time
	ExpiresAt       time.Time
}

// Request describes what is being asked for.
//
// Tenant, Scope and Owner are deliberately separate rather than one resource
// identifier: authorization is decided from the request, never by parsing an
// identifier, so an identifier can stay opaque.
type Request struct {
	// Tenant the resource belongs to.
	Tenant string
	// Scope the resource sits under, such as a campaign.
	Scope *Scope
	// Owner is the subject who owns the resource, for own-data capabilities.
	Owner string
}

// Decision is the outcome, with the reason it was reached.
//
// The reason is not an error message for a user. It is the line that goes in
// the audit record, and every denial in this product may end up in front of
// someone asking why they were refused.
type Decision struct {
	Allowed bool
	Reason  string
}

func allow(capability Capability) Decision {
	return Decision{Allowed: true, Reason: fmt.Sprintf("allowed: %s", capability)}
}

func deny(format string, args ...any) Decision {
	return Decision{Allowed: false, Reason: fmt.Sprintf(format, args...)}
}

// Can decides whether this context may exercise capability over request.
//
// The order of checks is deliberate. Context validity comes before capability,
// so an expired session is reported as expired rather than as under-privileged.
// Everything fails closed.
func (c Context) Can(capability Capability, request Request, at time.Time) Decision {
	requirement, known := Describe(capability)
	if !known {
		// An unknown capability is a bug in the caller. Denying it rather than
		// ignoring it means the bug surfaces as a refusal instead of as an
		// unchecked path.
		return deny("denied: %q is not in the capability catalogue", capability)
	}

	if c.ExpiresAt.IsZero() || !at.Before(c.ExpiresAt) {
		return deny("denied: the authorization context has expired")
	}

	if !c.holds(capability) {
		return deny("denied: %s is not held by this subject", capability)
	}

	if requirement.StepUp && at.Sub(c.AuthenticatedAt) > stepUpWindow {
		return deny("denied: %s needs authentication within the last %s", capability, stepUpWindow)
	}

	if requirement.Platform {
		if requirement.Privileged && !c.Elevation.valid(at) {
			return deny("denied: %s needs an active elevation with a reason and a ticket", capability)
		}
		return allow(capability)
	}

	if requirement.Tenant {
		if c.ActiveTenant == "" {
			return deny("denied: %s is tenant scoped and no tenant is active", capability)
		}
		if request.Tenant == "" {
			return deny("denied: %s is tenant scoped and the request names no tenant", capability)
		}
		if request.Tenant != c.ActiveTenant {
			return deny("denied: %s applies to the active tenant, not to %s", capability, request.Tenant)
		}
	}

	if requirement.Scope != ScopeNone {
		if request.Scope == nil {
			// Without this, a list endpoint could return every campaign in the
			// tenant by simply not naming one.
			return deny("denied: %s needs a %s scope and the request names none", capability, requirement.Scope)
		}
		if request.Scope.Kind != requirement.Scope {
			return deny("denied: %s needs a %s scope, not a %s scope", capability, requirement.Scope, request.Scope.Kind)
		}
		if !c.scoped(*request.Scope) {
			return deny("denied: %s is not assigned to this subject for %s %s",
				capability, request.Scope.Kind, request.Scope.Value)
		}
	}

	if requirement.Owner {
		if request.Owner == "" {
			return deny("denied: %s applies to own data and the request names no owner", capability)
		}
		if request.Owner != c.Subject.ID {
			// This is the practice and screen boundary. It is a structural
			// denial rather than a filter: no tenant capability can satisfy an
			// own-data requirement, because the subject is not the owner.
			return deny("denied: %s applies to the subject's own data only", capability)
		}
	}

	return allow(capability)
}

// CanGrant reports whether this subject may grant these capabilities to
// someone else.
//
// A subject cannot grant authority it does not hold. Without this rule, a
// recruiter with member management could promote themselves through the
// invitation form, which is privilege escalation with a friendly interface.
func (c Context) CanGrant(capabilities []Capability) error {
	if !c.holds(TenantMemberManage) {
		return errors.New("authz: granting capabilities needs tenant.member_manage")
	}
	for _, capability := range capabilities {
		if _, known := Describe(capability); !known {
			return fmt.Errorf("authz: %q is not in the capability catalogue", capability)
		}
		if !c.holds(capability) {
			return fmt.Errorf("authz: cannot grant %s, which this subject does not hold", capability)
		}
	}
	return nil
}

func (c Context) holds(capability Capability) bool {
	for _, held := range c.Capabilities {
		if held == capability {
			return true
		}
	}
	return false
}

func (c Context) scoped(scope Scope) bool {
	for _, held := range c.Scopes {
		if held.Kind == scope.Kind && held.Value == scope.Value {
			return true
		}
	}
	return false
}
