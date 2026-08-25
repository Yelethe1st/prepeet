package main

import (
	"context"
	"errors"

	"github.com/Yelethe1st/prepeet/services/platform/platform/objectstore"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/internal/candidate"
)

// candidateAdapter presents the candidate context as the port the API layer
// declared: ADR-0005's translation, in the one place allowed to see both.
type candidateAdapter struct {
	service   *candidate.Service
	documents *candidate.Documents
}

var (
	_ api.CandidateProfiles  = candidateAdapter{}
	_ api.CandidateDocuments = candidateAdapter{}
)

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

// The document half of the same adapter.

func (a candidateAdapter) StartUpload(ctx context.Context, userID, mediaType string, sizeBytes int64, partCount int) (api.StartedUpload, error) {
	started, err := a.documents.StartUpload(ctx, userID, mediaType, sizeBytes, partCount)
	if err != nil {
		return api.StartedUpload{}, translateDocumentError(err)
	}
	return api.StartedUpload{
		Document: toAPIDocument(started.Document),
		UploadID: started.UploadID, PartURLs: started.PartURLs, ExpiresAt: started.ExpiresAt,
	}, nil
}

func (a candidateAdapter) CompleteUpload(ctx context.Context, userID, documentID, uploadID, sha256 string, parts []api.UploadPart, sizeBytes int64) (api.Document, error) {
	completed := make([]objectstore.CompletedPart, 0, len(parts))
	for _, part := range parts {
		completed = append(completed, objectstore.CompletedPart{Number: part.Number, ETag: part.ETag})
	}
	stored, err := a.documents.CompleteUpload(ctx, userID, documentID, uploadID, sha256, completed, sizeBytes)
	if err != nil {
		return api.Document{}, translateDocumentError(err)
	}
	return toAPIDocument(stored), nil
}

func (a candidateAdapter) AbortUpload(ctx context.Context, userID, documentID string) error {
	return translateDocumentError(a.documents.AbortUpload(ctx, userID, documentID))
}

func (a candidateAdapter) DeleteDocument(ctx context.Context, userID, documentID string) error {
	return translateDocumentError(a.documents.Delete(ctx, userID, documentID))
}

func (a candidateAdapter) ListDocuments(ctx context.Context, userID string) ([]api.Document, error) {
	stored, err := a.documents.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]api.Document, 0, len(stored))
	for _, document := range stored {
		out = append(out, toAPIDocument(document))
	}
	return out, nil
}

func translateDocumentError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, candidate.ErrDocumentNotFound):
		return api.ErrDocumentMissing
	case errors.Is(err, candidate.ErrDocumentState):
		return api.ErrDocumentConflict
	case errors.Is(err, candidate.ErrDocumentType):
		return &api.ErrProfileInvalid{Field: "media_type", Code: "DOCUMENT_TYPE_UNSUPPORTED", Message: candidate.ErrDocumentType.Error()}
	case errors.Is(err, candidate.ErrDocumentTooLarge):
		return &api.ErrProfileInvalid{Field: "size_bytes", Code: "DOCUMENT_TOO_LARGE", Message: candidate.ErrDocumentTooLarge.Error()}
	case errors.Is(err, candidate.ErrDocumentParts):
		return &api.ErrProfileInvalid{Field: "part_count", Code: "DOCUMENT_PARTS_INVALID", Message: candidate.ErrDocumentParts.Error()}
	}
	return err
}

func toAPIDocument(d candidate.Document) api.Document {
	return api.Document{
		ID: d.ID, Kind: d.Kind, Version: d.Version, MediaType: d.MediaType,
		SizeBytes: d.SizeBytes, State: d.State, SHA256: d.SHA256,
		CreatedAt: d.CreatedAt, StoredAt: d.StoredAt, DeletedAt: d.DeletedAt,
	}
}
