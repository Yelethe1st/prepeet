package main

import (
	"context"
	"errors"
	"strings"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/internal/identity"
)

// membersAdapter presents member administration as the port the API declared.
type membersAdapter struct {
	members *identity.Members
}

func (a membersAdapter) List(ctx context.Context, tenantID string) ([]api.TenantMember, error) {
	listed, err := a.members.List(ctx, tenantID)
	if err != nil {
		return nil, translateMemberError(err)
	}
	out := make([]api.TenantMember, 0, len(listed))
	for _, member := range listed {
		out = append(out, toAPIMember(member))
	}
	return out, nil
}

func (a membersAdapter) Invite(ctx context.Context, tenantID, actorID, email, role string) (api.TenantMember, error) {
	invited, err := a.members.Invite(ctx, tenantID, actorID, email, role)
	if err != nil {
		return api.TenantMember{}, translateMemberError(err)
	}
	return toAPIMember(invited), nil
}

func (a membersAdapter) ChangeRole(ctx context.Context, tenantID, actorID, membershipID, role string, expectedVersion int) (api.TenantMember, error) {
	changed, err := a.members.ChangeRole(ctx, tenantID, actorID, membershipID, role, expectedVersion)
	if err != nil {
		return api.TenantMember{}, translateMemberError(err)
	}
	return toAPIMember(changed), nil
}

func (a membersAdapter) Revoke(ctx context.Context, tenantID, actorID, membershipID string, expectedVersion int) error {
	return translateMemberError(a.members.Revoke(ctx, tenantID, actorID, membershipID, expectedVersion))
}

func toAPIMember(member identity.Member) api.TenantMember {
	return api.TenantMember{
		MembershipID: member.MembershipID, UserID: member.UserID,
		Email: member.Email, Role: member.Role, Status: member.Status,
		Version: member.Version, CreatedAt: member.CreatedAt,
	}
}

// translateMemberError maps the administration refusals onto the HTTP
// surface's words. The unknown-address refusal names the field, and its
// message deliberately does not confirm or deny anything beyond "no active
// account": the endpoint needs member_manage, so it is not an oracle the
// public can query, but the words stay careful anyway.
func translateMemberError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, identity.ErrMemberNotFound):
		return api.ErrMemberMissing
	case errors.Is(err, identity.ErrMemberExists),
		errors.Is(err, identity.ErrMemberOwner),
		errors.Is(err, identity.ErrMemberStale):
		return api.ErrMemberConflict
	case errors.Is(err, identity.ErrMemberRoleInvalid):
		return api.Invalid("role", "MEMBER_ROLE_INVALID", memberMessage(identity.ErrMemberRoleInvalid))
	case errors.Is(err, identity.ErrMemberUnknownEmail):
		return api.Invalid("email", "MEMBER_UNKNOWN_EMAIL", memberMessage(identity.ErrMemberUnknownEmail))
	}
	return err
}

// memberMessage strips the sentinel's package-and-code prefix down to the
// sentence a person reads.
func memberMessage(err error) string {
	text := err.Error()
	if idx := strings.LastIndex(text, ": "); idx >= 0 {
		return strings.ToUpper(text[idx+2:idx+3]) + text[idx+3:] + "."
	}
	return text
}
