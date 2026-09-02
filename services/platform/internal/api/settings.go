package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	prepeetapi "github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
	"github.com/Yelethe1st/prepeet/services/platform/platform/authz"
)

// TenantSettings is a workspace's configuration as this package needs it.
//
// Flattened rather than mirroring the store's nested document, because the
// handler's job is to answer the contract and the contract's shape is its own.
// A change to how the store nests things should not reach the wire, and a
// change to the wire should not reach the store.
type TenantSettings struct {
	// Version is 0 for a workspace that has never saved, whose document is the
	// platform defaults. A change must name the version it was made against.
	Version     int
	LegalName   string
	DisplayName string
	ChangedBy   string
	ChangedAt   time.Time
}

// ErrSettingsConflict means somebody else changed the settings first.
//
// Its own error rather than a generic failure, because the answer a person
// needs is "reload and look at what changed", not "try again", and a retry of
// the same stale version would fail identically forever.
var ErrSettingsConflict = errors.New("api: the settings changed while you were editing")

// TenantConfiguration reads and changes a workspace's settings.
type TenantConfiguration interface {
	Current(ctx context.Context, tenantID string) (TenantSettings, error)
	// Save appends a version. The version carried on next is the one the
	// change was made against, and a mismatch is ErrSettingsConflict.
	Save(ctx context.Context, tenantID, actorID string, next TenantSettings) (TenantSettings, error)
}

// settingsHandlers serves the workspace configuration.
type settingsHandlers struct {
	authentication *authentication
	settings       TenantConfiguration
}

// GetTenantSettings answers the configuration, and whether this caller may
// change it.
//
// The second part is the whole of TEN-01's criterion. A read-only member has to
// meet a page they cannot edit rather than a page they cannot open: a 403 on
// the settings screen reads as a broken product, and the recruiter role's own
// description already draws the line in the right place, saying they cannot
// change how the workspace is configured rather than that they cannot see it.
//
// Editability is decided here and served, not inferred in the browser from a
// role name. A browser deciding would be a second copy of the authorization
// rules, and the copy that drifts is the one nobody re-reads.
func (s *settingsHandlers) GetTenantSettings(ctx context.Context, _ prepeetapi.GetTenantSettingsRequestObject) (prepeetapi.GetTenantSettingsResponseObject, error) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		return s.authentication.rejectedSession(ctx), nil
	}

	// The capability the contract declares for this operation, which is the
	// read one. Asking for manage here would put the page behind the authority
	// to change it, which is the bug this ticket is about.
	principal, err := s.authentication.identity.Authorize(ctx, presented, requiredCapabilityFrom(ctx))
	if err != nil {
		return s.authentication.failed(ctx, err), nil
	}

	current, err := s.settings.Current(ctx, principal.ActiveTenantID)
	if err != nil {
		return s.authentication.failed(ctx, err), nil
	}

	// A different question, and deliberately not an authorization: may this
	// caller change what they are reading. Its answer is a boolean and never a
	// refusal, which is why it does not go through Authorize.
	editable := s.authentication.identity.Holds(ctx, presented, string(authz.TenantSettingsManage))

	body := prepeetapi.TenantSettings{
		Version: current.Version, Editable: editable,
		Settings: prepeetapi.TenantSettingsDocument{
			Organisation: struct {
				DisplayName string `json:"display_name"`
				LegalName   string `json:"legal_name"`
			}{DisplayName: current.DisplayName, LegalName: current.LegalName},
		},
	}
	if current.ChangedBy != "" {
		changedBy := current.ChangedBy
		body.ChangedBy = &changedBy
	}
	if !current.ChangedAt.IsZero() {
		changedAt := current.ChangedAt
		body.ChangedAt = &changedAt
	}

	return prepeetapi.GetTenantSettings200JSONResponse{
		Body:    body,
		Headers: prepeetapi.GetTenantSettings200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// requiredCapabilityFrom reads what the contract declared for this operation,
// which carryCredentials put in the context.
func requiredCapabilityFrom(ctx context.Context) string {
	capability, _ := ctx.Value(capabilityContextKey{}).(string)
	return capability
}

// SaveTenantSettings appends a new version of the configuration.
//
// Gated on the manage capability, which the contract declares for this
// operation, so the read capability admits somebody to the page and to nothing
// else.
func (s *settingsHandlers) SaveTenantSettings(ctx context.Context, request prepeetapi.SaveTenantSettingsRequestObject) (prepeetapi.SaveTenantSettingsResponseObject, error) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		return s.authentication.rejectedSession(ctx), nil
	}
	principal, err := s.authentication.identity.Authorize(ctx, presented, requiredCapabilityFrom(ctx))
	if err != nil {
		return s.authentication.failed(ctx, err), nil
	}

	saved, err := s.settings.Save(ctx, principal.ActiveTenantID, principal.UserID, TenantSettings{
		Version:     request.Body.Version,
		LegalName:   request.Body.Settings.Organisation.LegalName,
		DisplayName: request.Body.Settings.Organisation.DisplayName,
	})
	if errors.Is(err, ErrSettingsConflict) {
		// A conflict is not a failure of the request but a fact about the
		// world, and the person needs to see what changed rather than retry.
		return failure{
			status: http.StatusConflict, code: "SETTINGS_CONFLICT",
			message: "Somebody else changed these settings while you were editing. " +
				"Reload to see the current values before saving again.",
			environment: s.authentication.environment,
		}, nil
	}
	if err != nil {
		return s.authentication.failed(ctx, err), nil
	}

	return prepeetapi.SaveTenantSettings200JSONResponse{
		Body: prepeetapi.TenantSettings{
			Version: saved.Version, Editable: true,
			Settings: prepeetapi.TenantSettingsDocument{
				Organisation: struct {
					DisplayName string `json:"display_name"`
					LegalName   string `json:"legal_name"`
				}{DisplayName: saved.DisplayName, LegalName: saved.LegalName},
			},
		},
		Headers: prepeetapi.SaveTenantSettings200ResponseHeaders{CacheControl: NoStore},
	}, nil
}
