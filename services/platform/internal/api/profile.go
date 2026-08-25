package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
)

// The candidate's own profile at the HTTP boundary. Implements PRO-01's
// surface: the session decides whose profile, and there is no way to name
// anybody else's - owner scoping is the absence of a parameter, not a check.

// CandidateProfiles is what the API needs from the candidate context,
// declared here per ADR-0005 and wired in cmd.
type CandidateProfiles interface {
	// GetProfile returns the profile, empty if never saved. Absence is not an
	// error: the first visit must not look broken.
	GetProfile(ctx context.Context, userID string) (Profile, error)
	// SaveProfile validates, normalises and stores the whole profile,
	// returning it as stored.
	SaveProfile(ctx context.Context, userID string, profile Profile) (Profile, error)
}

// Profile mirrors the contract's CandidateProfile at the port.
type Profile struct {
	Disciplines             []string
	TargetRoles             []string
	Seniority               string
	CareerContext           string
	DefaultDurationMinutes  int
	DefaultStyle            string
	DefaultPressure         string
	ExtendedTime            bool
	Captions                bool
	ReducedMotion           bool
	AccessibilityNotes      string
	NotifyProductUpdates    bool
	NotifyPracticeReminders bool
}

// ErrProfileInvalid carries a field-level refusal from the candidate context.
type ErrProfileInvalid struct {
	Field   string
	Code    string
	Message string
}

func (e *ErrProfileInvalid) Error() string { return "api: " + e.Code + ": " + e.Message }

// profile handles the /me/profile operations.
type profile struct {
	authentication *authentication
	candidates     CandidateProfiles
}

// GetProfile reads the signed-in candidate's profile.
func (p *profile) GetProfile(ctx context.Context, _ prepeetapi.GetProfileRequestObject) (prepeetapi.GetProfileResponseObject, error) {
	principal, failed := p.authenticated(ctx)
	if failed != nil {
		return *failed, nil
	}

	stored, err := p.candidates.GetProfile(ctx, principal.UserID)
	if err != nil {
		return p.authentication.failed(ctx, err), nil
	}
	return prepeetapi.GetProfile200JSONResponse{
		Body:    profileBody(stored),
		Headers: prepeetapi.GetProfile200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// SaveProfile replaces the signed-in candidate's profile.
func (p *profile) SaveProfile(ctx context.Context, request prepeetapi.SaveProfileRequestObject) (prepeetapi.SaveProfileResponseObject, error) {
	principal, failed := p.authenticated(ctx)
	if failed != nil {
		return *failed, nil
	}

	stored, err := p.candidates.SaveProfile(ctx, principal.UserID, profileFromBody(request.Body))
	if err != nil {
		var invalid *ErrProfileInvalid
		if errors.As(err, &invalid) {
			return p.authentication.failed(ctx, Invalid(invalid.Field, invalid.Code, invalid.Message)), nil
		}
		return p.authentication.failed(ctx, err), nil
	}
	return prepeetapi.SaveProfile200JSONResponse{
		Body:    profileBody(stored),
		Headers: prepeetapi.SaveProfile200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// authenticated resolves the session, or produces the refusal.
func (p *profile) authenticated(ctx context.Context) (Principal, *failure) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		refused := p.authentication.rejectedSession(ctx)
		return Principal{}, &refused
	}
	principal, err := p.authentication.identity.Lookup(ctx, presented)
	if err != nil {
		refused := p.authentication.failed(ctx, err)
		return Principal{}, &refused
	}
	return principal, nil
}

// The mapping, in both directions. Optional strings travel as pointers in the
// generated types; empty and absent mean the same thing on this record.

func profileBody(profile Profile) prepeetapi.CandidateProfile {
	body := prepeetapi.CandidateProfile{
		Disciplines:             profile.Disciplines,
		TargetRoles:             profile.TargetRoles,
		ExtendedTime:            profile.ExtendedTime,
		Captions:                profile.Captions,
		ReducedMotion:           profile.ReducedMotion,
		NotifyProductUpdates:    profile.NotifyProductUpdates,
		NotifyPracticeReminders: profile.NotifyPracticeReminders,
	}
	setIf(&body.Seniority, profile.Seniority)
	setIf(&body.CareerContext, profile.CareerContext)
	setIf(&body.DefaultStyle, profile.DefaultStyle)
	setIf(&body.AccessibilityNotes, profile.AccessibilityNotes)
	if profile.DefaultDurationMinutes != 0 {
		minutes := profile.DefaultDurationMinutes
		body.DefaultDurationMinutes = &minutes
	}
	if profile.DefaultPressure != "" {
		pressure := prepeetapi.CandidateProfileDefaultPressure(profile.DefaultPressure)
		body.DefaultPressure = &pressure
	}
	return body
}

func profileFromBody(body *prepeetapi.CandidateProfile) Profile {
	profile := Profile{
		Disciplines:             body.Disciplines,
		TargetRoles:             body.TargetRoles,
		ExtendedTime:            body.ExtendedTime,
		Captions:                body.Captions,
		ReducedMotion:           body.ReducedMotion,
		NotifyProductUpdates:    body.NotifyProductUpdates,
		NotifyPracticeReminders: body.NotifyPracticeReminders,
	}
	profile.Seniority = deref(body.Seniority)
	profile.CareerContext = deref(body.CareerContext)
	profile.DefaultStyle = deref(body.DefaultStyle)
	profile.AccessibilityNotes = deref(body.AccessibilityNotes)
	if body.DefaultDurationMinutes != nil {
		profile.DefaultDurationMinutes = *body.DefaultDurationMinutes
	}
	if body.DefaultPressure != nil {
		profile.DefaultPressure = string(*body.DefaultPressure)
	}
	return profile
}

func setIf(target **string, value string) {
	if value != "" {
		copied := value
		*target = &copied
	}
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// The failure type must speak these operations' responses.
var (
	_ prepeetapi.GetProfileResponseObject  = failure{}
	_ prepeetapi.SaveProfileResponseObject = failure{}
)

func (f failure) VisitGetProfileResponse(w http.ResponseWriter) error  { return f.write(w) }
func (f failure) VisitSaveProfileResponse(w http.ResponseWriter) error { return f.write(w) }
