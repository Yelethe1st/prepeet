package main

import (
	"context"
	"errors"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/internal/candidate"
)

// candidateAdapter presents the candidate context as the port the API layer
// declared: ADR-0005's translation, in the one place allowed to see both.
type candidateAdapter struct {
	service *candidate.Service
}

var _ api.CandidateProfiles = candidateAdapter{}

func (a candidateAdapter) GetProfile(ctx context.Context, userID string) (api.Profile, error) {
	profile, err := a.service.GetProfile(ctx, userID)
	if err != nil {
		return api.Profile{}, err
	}
	return toAPIProfile(profile), nil
}

func (a candidateAdapter) SaveProfile(ctx context.Context, userID string, profile api.Profile) (api.Profile, error) {
	stored, err := a.service.SaveProfile(ctx, userID, fromAPIProfile(profile))
	if err != nil {
		return api.Profile{}, translateProfileError(err)
	}
	return toAPIProfile(stored), nil
}

// translateProfileError maps the context's stable refusals onto field errors,
// so the form shows each beside the input that earned it.
func translateProfileError(err error) error {
	for _, mapping := range []struct {
		sentinel error
		field    string
		code     string
	}{
		{candidate.ErrCareerContextTooLong, "career_context", "CAREER_CONTEXT_TOO_LONG"},
		{candidate.ErrTooManyEntries, "disciplines", "TOO_MANY_ENTRIES"},
		{candidate.ErrEntryTooLong, "disciplines", "ENTRY_TOO_LONG"},
		{candidate.ErrPressureUnknown, "default_pressure", "PRESSURE_UNKNOWN"},
		{candidate.ErrDurationOutOfRange, "default_duration_minutes", "DURATION_OUT_OF_RANGE"},
	} {
		if errors.Is(err, mapping.sentinel) {
			return &api.ErrProfileInvalid{
				Field: mapping.field, Code: mapping.code, Message: mapping.sentinel.Error(),
			}
		}
	}
	return err
}

func toAPIProfile(p candidate.Profile) api.Profile {
	return api.Profile{
		Disciplines: p.Disciplines, TargetRoles: p.TargetRoles,
		Seniority: p.Seniority, CareerContext: p.CareerContext,
		DefaultDurationMinutes: p.DefaultDurationMinutes,
		DefaultStyle:           p.DefaultStyle, DefaultPressure: p.DefaultPressure,
		ExtendedTime: p.ExtendedTime, Captions: p.Captions, ReducedMotion: p.ReducedMotion,
		AccessibilityNotes:   p.AccessibilityNotes,
		NotifyProductUpdates: p.NotifyProductUpdates, NotifyPracticeReminders: p.NotifyPracticeReminders,
	}
}

func fromAPIProfile(p api.Profile) candidate.Profile {
	return candidate.Profile{
		Disciplines: p.Disciplines, TargetRoles: p.TargetRoles,
		Seniority: p.Seniority, CareerContext: p.CareerContext,
		DefaultDurationMinutes: p.DefaultDurationMinutes,
		DefaultStyle:           p.DefaultStyle, DefaultPressure: p.DefaultPressure,
		ExtendedTime: p.ExtendedTime, Captions: p.Captions, ReducedMotion: p.ReducedMotion,
		AccessibilityNotes:   p.AccessibilityNotes,
		NotifyProductUpdates: p.NotifyProductUpdates, NotifyPracticeReminders: p.NotifyPracticeReminders,
	}
}
