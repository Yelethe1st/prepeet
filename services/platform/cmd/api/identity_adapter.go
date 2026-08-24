package main

import (
	"context"
	"errors"

	"github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/internal/identity"
)

// identityAdapter presents the identity context as the port the API layer
// declared.
//
// This translation is what ADR-0005 costs, and it lives here because cmd is the
// one place allowed to see both contexts. The module boundary test enforces
// that: internal/api importing internal/identity does not compile.
//
// It is worth the cost for a specific reason rather than on principle. The two
// vocabularies differ where it matters: identity distinguishes ErrNotFound from
// ErrCredentialsInvalid because its own logic needs to, and the API must not,
// since a response that could tell them apart is an account-existence oracle.
// The collapse happens here, once, rather than being a rule every handler
// remembers.
type identityAdapter struct {
	service *identity.Service
}

func (a identityAdapter) Register(ctx context.Context, input api.Registration) error {
	_, err := a.service.Register(ctx, identity.RegisterInput{
		Email:            input.Email,
		Password:         input.Password,
		AccountType:      identity.AccountType(input.AccountType),
		OrganisationName: input.OrganisationName,
	})
	// The outcome is deliberately discarded. It reports whether an account was
	// created, and the HTTP layer must not know: a handler holding that fact is
	// one refactor away from responding differently.
	return a.translate(err)
}

func (a identityAdapter) Authenticate(ctx context.Context, email, password string) (api.Session, error) {
	session, err := a.service.Authenticate(ctx, email, password)
	if err != nil {
		return api.Session{}, a.translate(err)
	}
	return sessionFrom(session), nil
}

func (a identityAdapter) Refresh(ctx context.Context, refreshToken string) (api.Session, error) {
	session, err := a.service.Refresh(ctx, refreshToken)
	if err != nil {
		return api.Session{}, a.translate(err)
	}
	return sessionFrom(session), nil
}

func (a identityAdapter) Lookup(ctx context.Context, sessionToken string) (api.Principal, error) {
	row, err := a.service.Lookup(ctx, sessionToken)
	if err != nil {
		return api.Principal{}, a.translate(err)
	}
	return api.Principal{
		UserID:          row.UserID,
		SessionID:       row.ID,
		AuthenticatedAt: row.AuthenticatedAt,
	}, nil
}

func (a identityAdapter) Revoke(ctx context.Context, sessionToken, reason string) error {
	return a.translate(a.service.Revoke(ctx, sessionToken, reason))
}

func (a identityAdapter) Describe(ctx context.Context, userID string) (api.User, error) {
	user, err := a.service.Describe(ctx, userID)
	if err != nil {
		return api.User{}, a.translate(err)
	}
	memberships := make([]api.Membership, 0, len(user.Memberships))
	for _, membership := range user.Memberships {
		memberships = append(memberships, api.Membership{
			TenantID:   membership.TenantID,
			TenantName: membership.TenantName,
			Status:     membership.Status,
		})
	}

	// Role is deliberately not carried across. The contract's Membership says
	// which tenants a person belongs to and whether each is active; what they
	// may do there is an authorization answer, and putting a role in the
	// response invites a client to decide from it. IAM-04 already owns that
	// decision and answers it server side.
	return api.User{
		ID:            user.ID,
		Email:         user.Email,
		EmailVerified: user.EmailVerified,
		Memberships:   memberships,
	}, nil
}

func sessionFrom(session identity.Session) api.Session {
	return api.Session{
		UserID:          session.UserID,
		SessionToken:    session.SessionToken,
		RefreshToken:    session.RefreshToken,
		ExpiresAt:       session.ExpiresAt,
		RefreshExpires:  session.RefreshExpires,
		AuthenticatedAt: session.AuthenticatedAt,
	}
}

// translate maps identity's vocabulary onto the API's.
//
// The default case passes the error through unchanged, so an unrecognised
// failure becomes a 500 rather than being silently reclassified as a client
// error. Mapping the other way, defaulting to a 4xx, would hide outages as
// validation failures.
func (a identityAdapter) translate(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, identity.ErrCredentialsInvalid):
		return api.ErrCredentialsRejected

	case errors.Is(err, identity.ErrSessionInvalid), errors.Is(err, identity.ErrNotFound):
		// ErrNotFound collapses into a rejected session on purpose. It reaches
		// here from a lookup for a token that does not exist, and "no such
		// session" and "that session is over" must be one answer.
		return api.ErrSessionRejected

	case errors.Is(err, identity.ErrEmailInvalid):
		return api.Invalid("email", "EMAIL_INVALID", err.Error())

	case errors.Is(err, identity.ErrPasswordTooShort), errors.Is(err, identity.ErrPasswordTooLong):
		return api.Invalid("password", "PASSWORD_INVALID", err.Error())

	case errors.Is(err, identity.ErrAccountType):
		return api.Invalid("account_type", "ACCOUNT_TYPE_INVALID", err.Error())

	case errors.Is(err, identity.ErrOrganisationName):
		return api.Invalid("organisation_name", "ORGANISATION_NAME_REQUIRED", err.Error())

	default:
		return err
	}
}

var _ api.Identity = identityAdapter{}
