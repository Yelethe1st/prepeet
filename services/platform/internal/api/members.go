package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
)

// The member administration surface: TEN-02 at the HTTP boundary.
//
// Every operation is decided by the one policy path - the contract declares the
// capability and the Identity port answers with the session's authority or a
// refusal - and the active tenant and acting administrator come from the
// session alone. The request never names a tenant, so administering somebody
// else's workspace is not a parameter away from a bug.

// TenantMembers is what the API needs from member administration, declared
// here per ADR-0005 and wired in cmd.
type TenantMembers interface {
	List(ctx context.Context, tenantID string) ([]TenantMember, error)
	Invite(ctx context.Context, tenantID, actorID, email, role string) (TenantMember, error)
	ChangeRole(ctx context.Context, tenantID, actorID, membershipID, role string, expectedVersion int) (TenantMember, error)
	Revoke(ctx context.Context, tenantID, actorID, membershipID string, expectedVersion int) error
}

// TenantMember mirrors the contract's Member at the port.
type TenantMember struct {
	MembershipID string
	UserID       string
	Email        string
	Role         string
	Status       string
	Version      int
	CreatedAt    time.Time
}

// Member administration refusals the port maps onto responses.
var (
	// ErrMemberMissing covers absence and another workspace's membership alike.
	ErrMemberMissing = errors.New("api: no such membership")
	// ErrMemberConflict means the membership is not in a state the operation
	// applies to, or changed since it was read; nothing changed.
	ErrMemberConflict = errors.New("api: that membership is not in a state this operation applies to")
)

// members handles the /tenant/members operations.
type members struct {
	authentication *authentication
	flows          TenantMembers
}

// authorized resolves the session's authority for the capability the contract
// declares for the operation being served.
//
// The capability is not a parameter, because it is not this layer's to choose.
// packages/contracts/api/openapi.yaml declares it per operation and the
// middleware carries the declaration through; a handler naming its own would be
// a second answer to a question the contract has already answered, and the two
// would eventually differ.
func (m *members) authorized(ctx context.Context) (Principal, *failure) {
	capability := requiredCapability(ctx)
	if capability == "" {
		// Reached only if this handler is wired to an operation the contract
		// says needs no authority. Refusing is the safe reading: asking the
		// policy path for permission to do nothing would let it through.
		refused := m.authentication.rejectedSession(ctx)
		return Principal{}, &refused
	}

	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		refused := m.authentication.rejectedSession(ctx)
		return Principal{}, &refused
	}
	principal, err := m.authentication.identity.Authorize(ctx, presented, capability)
	if err != nil {
		refused := m.authentication.failed(ctx, err)
		return Principal{}, &refused
	}
	return principal, nil
}

// ListMembers answers the workspace's people.
func (m *members) ListMembers(ctx context.Context, _ prepeetapi.ListMembersRequestObject) (prepeetapi.ListMembersResponseObject, error) {
	principal, refused := m.authorized(ctx)
	if refused != nil {
		return *refused, nil
	}

	listed, err := m.flows.List(ctx, principal.ActiveTenantID)
	if err != nil {
		return m.authentication.failed(ctx, err), nil
	}
	body := prepeetapi.MemberList{Members: make([]prepeetapi.Member, 0, len(listed))}
	for _, member := range listed {
		encoded, err := memberBody(member)
		if err != nil {
			return m.authentication.failed(ctx, err), nil
		}
		body.Members = append(body.Members, encoded)
	}
	return prepeetapi.ListMembers200JSONResponse{
		Body:    body,
		Headers: prepeetapi.ListMembers200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// InviteMember adds an existing account as an invited member.
func (m *members) InviteMember(ctx context.Context, request prepeetapi.InviteMemberRequestObject) (prepeetapi.InviteMemberResponseObject, error) {
	principal, refused := m.authorized(ctx)
	if refused != nil {
		return *refused, nil
	}

	invited, err := m.flows.Invite(ctx, principal.ActiveTenantID, principal.UserID,
		string(request.Body.Email), string(request.Body.Role))
	if err != nil {
		return m.authentication.failed(ctx, err), nil
	}
	body, err := memberBody(invited)
	if err != nil {
		return m.authentication.failed(ctx, err), nil
	}
	return prepeetapi.InviteMember201JSONResponse{
		Body:    body,
		Headers: prepeetapi.InviteMember201ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// ChangeMemberRole moves a membership to another assignable role.
func (m *members) ChangeMemberRole(ctx context.Context, request prepeetapi.ChangeMemberRoleRequestObject) (prepeetapi.ChangeMemberRoleResponseObject, error) {
	principal, refused := m.authorized(ctx)
	if refused != nil {
		return *refused, nil
	}

	changed, err := m.flows.ChangeRole(ctx, principal.ActiveTenantID, principal.UserID,
		request.MembershipID.String(), string(request.Body.Role), request.Body.ExpectedVersion)
	if err != nil {
		return m.authentication.failed(ctx, err), nil
	}
	body, err := memberBody(changed)
	if err != nil {
		return m.authentication.failed(ctx, err), nil
	}
	return prepeetapi.ChangeMemberRole200JSONResponse{
		Body:    body,
		Headers: prepeetapi.ChangeMemberRole200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// RevokeMember removes a member's access, effective on their next request.
func (m *members) RevokeMember(ctx context.Context, request prepeetapi.RevokeMemberRequestObject) (prepeetapi.RevokeMemberResponseObject, error) {
	principal, refused := m.authorized(ctx)
	if refused != nil {
		return *refused, nil
	}

	if err := m.flows.Revoke(ctx, principal.ActiveTenantID, principal.UserID,
		request.MembershipID.String(), request.Params.ExpectedVersion); err != nil {
		return m.authentication.failed(ctx, err), nil
	}
	return prepeetapi.RevokeMember204Response{
		Headers: prepeetapi.RevokeMember204ResponseHeaders{CacheControl: NoStore},
	}, nil
}

func memberBody(member TenantMember) (prepeetapi.Member, error) {
	membershipID, err := uuid.Parse(member.MembershipID)
	if err != nil {
		return prepeetapi.Member{}, err
	}
	userID, err := uuid.Parse(member.UserID)
	if err != nil {
		return prepeetapi.Member{}, err
	}
	return prepeetapi.Member{
		MembershipID: membershipID,
		UserID:       userID,
		Email:        member.Email,
		Role:         prepeetapi.TenantRole(member.Role),
		Status:       prepeetapi.MemberStatus(member.Status),
		Version:      member.Version,
		CreatedAt:    member.CreatedAt,
	}, nil
}

// The failure type must speak these operations' responses.
var (
	_ prepeetapi.ListMembersResponseObject      = failure{}
	_ prepeetapi.InviteMemberResponseObject     = failure{}
	_ prepeetapi.ChangeMemberRoleResponseObject = failure{}
	_ prepeetapi.RevokeMemberResponseObject     = failure{}
)

func (f failure) VisitListMembersResponse(w http.ResponseWriter) error      { return f.write(w) }
func (f failure) VisitInviteMemberResponse(w http.ResponseWriter) error     { return f.write(w) }
func (f failure) VisitChangeMemberRoleResponse(w http.ResponseWriter) error { return f.write(w) }
func (f failure) VisitRevokeMemberResponse(w http.ResponseWriter) error     { return f.write(w) }
