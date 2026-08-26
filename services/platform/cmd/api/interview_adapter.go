package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/internal/catalog"
	"github.com/Yelethe1st/prepeet/services/platform/internal/content"
	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// interviewAdapter presents session creation as the port the API declared.
// It is the enforcement point CAT-03 left open: the one place that sees both
// the catalogue and the interview context, so the selection is validated
// against the former before the latter ever hears about it.
type interviewAdapter struct {
	catalogue *catalog.Service
	sessions  *interview.Store
	registry  *content.Store
}

// PracticeConsent answers the currently published consent text.
func (a interviewAdapter) PracticeConsent(ctx context.Context) (api.PracticeConsent, error) {
	document, version, err := a.currentConsent(ctx)
	if err != nil {
		return api.PracticeConsent{}, err
	}
	return api.PracticeConsent{
		Version:    version,
		Title:      document.Title,
		Statements: document.Statements,
		AudioAndTranscript: api.ConsentChoiceView{
			Label:       document.Choices.AudioAndTranscript.Label,
			Explanation: document.Choices.AudioAndTranscript.Explanation,
			Forfeits:    document.Choices.AudioAndTranscript.Forfeits,
		},
		TranscriptOnly: api.ConsentChoiceView{
			Label:       document.Choices.TranscriptOnly.Label,
			Explanation: document.Choices.TranscriptOnly.Explanation,
			Forfeits:    document.Choices.TranscriptOnly.Forfeits,
		},
	}, nil
}

// currentConsent resolves and parses the published consent text. Platform
// content: practice has no tenant, so no tenant override applies.
func (a interviewAdapter) currentConsent(ctx context.Context) (interview.ConsentDocument, string, error) {
	artifact, err := a.registry.Resolve(ctx, interview.ConsentReference, "")
	if err != nil {
		return interview.ConsentDocument{}, "", fmt.Errorf("resolving the consent text: %w", err)
	}
	document, err := interview.ParseConsent(artifact.Body)
	if err != nil {
		return interview.ConsentDocument{}, "", err
	}
	return document, artifact.Version, nil
}

func (a interviewAdapter) CreatePractice(ctx context.Context, userID string, selection api.InterviewSelection) (api.InterviewSession, error) {
	// Practice reads the platform catalogue: a practice session has no
	// tenant, by the schema's own CHECK, so no tenant override applies.
	catalogue, err := a.catalogue.Catalogue(ctx, "")
	if err != nil {
		return api.InterviewSession{}, err
	}
	if refused := selectionErrors(catalogue, selection); refused != nil {
		return api.InterviewSession{}, refused
	}

	// The consent version must be the one currently published: a session may
	// only record a version whose exact words the person was actually shown,
	// and a stale one means the text changed under them - the wizard
	// refetches and shows the new words rather than silently upgrading the
	// agreement.
	_, currentVersion, err := a.currentConsent(ctx)
	if err != nil {
		return api.InterviewSession{}, err
	}
	if selection.Recording.ConsentVersion != currentVersion {
		return api.InterviewSession{}, api.Invalid("recording.consent_version", "CONSENT_STALE",
			"The consent text has changed since it was shown. Review the current text and choose again.")
	}

	config, err := json.Marshal(map[string]any{
		"discipline": selection.Discipline,
		"role":       selection.Role,
		"shape":      selection.Shape,
		"minutes":    selection.Minutes,
		"persona":    selection.Persona,
	})
	if err != nil {
		return api.InterviewSession{}, fmt.Errorf("encoding the selection: %w", err)
	}

	session := interview.Session{
		ID:          id.New().String(),
		Mode:        "practice",
		CandidateID: userID,
		// The blueprint is the shape's plan artifact: what composition will
		// resolve and pin. The full selection rides the bundle through the
		// session's own config.
		BlueprintID:         "plan/" + selection.Shape,
		Config:              config,
		RecordingPreference: selection.Recording.Preference,
		ConsentVersion:      selection.Recording.ConsentVersion,
	}
	actor := interview.Actor{ID: userID, Type: "user"}
	if err := a.sessions.Create(ctx, session, actor); err != nil {
		return api.InterviewSession{}, err
	}

	// Straight into composing: creation IS the request to compose, and a
	// draft that waited for a second command would be a state the wizard
	// has no button for. The workflow itself starts from the created event
	// in the worker, so a crash between here and there retries from the
	// outbox rather than losing the composition.
	created, err := a.sessions.Get(ctx, session.ID, session.Mode, session.CandidateID, "")
	if err != nil {
		return api.InterviewSession{}, err
	}
	composing, err := a.sessions.Transition(ctx, created, interview.StateComposing, interview.Effects{}, actor)
	if err != nil {
		return api.InterviewSession{}, err
	}

	return api.InterviewSession{
		ID: composing.ID, Mode: composing.Mode, State: string(composing.State),
		Config:              selection,
		RecordingPreference: composing.RecordingPreference,
		ConsentVersion:      composing.ConsentVersion,
		CreatedAt:           composing.CreatedAt,
	}, nil
}

// GetPractice reads one practice session under its owner's scope. Absence
// and somebody else's session answer identically, because the store's own
// scoping makes the row not exist for anyone but the owner.
func (a interviewAdapter) GetPractice(ctx context.Context, userID, sessionID string) (api.InterviewSession, error) {
	session, err := a.sessions.Get(ctx, sessionID, "practice", userID, "")
	if errors.Is(err, interview.ErrNotFound) {
		return api.InterviewSession{}, api.ErrSessionMissing
	}
	if err != nil {
		return api.InterviewSession{}, err
	}

	var selection api.InterviewSelection
	var config struct {
		Discipline string `json:"discipline"`
		Role       string `json:"role"`
		Shape      string `json:"shape"`
		Minutes    int    `json:"minutes"`
		Persona    string `json:"persona"`
	}
	if err := json.Unmarshal(session.Config, &config); err == nil {
		selection = api.InterviewSelection{
			Discipline: config.Discipline, Role: config.Role,
			Shape: config.Shape, Minutes: config.Minutes, Persona: config.Persona,
		}
	}

	return api.InterviewSession{
		ID: session.ID, Mode: session.Mode, State: string(session.State),
		Config:              selection,
		RecordingPreference: session.RecordingPreference,
		ConsentVersion:      session.ConsentVersion,
		FailureCode:         session.FailureCode,
		CreatedAt:           session.CreatedAt,
	}, nil
}

// selectionErrors maps the catalogue's refusals onto the API's validation
// error, every field at once.
func selectionErrors(catalogue catalog.Catalogue, selection api.InterviewSelection) *api.ValidationError {
	refusals := catalogue.Validate(catalog.Selection{
		Discipline: selection.Discipline,
		Role:       selection.Role,
		Shape:      selection.Shape,
		Minutes:    selection.Minutes,
		Persona:    selection.Persona,
	})
	if len(refusals) == 0 {
		return nil
	}
	fields := make([]api.FieldError, 0, len(refusals))
	for _, refusal := range refusals {
		fields = append(fields, api.FieldError{
			Field: refusal.Field, Code: refusal.Code, Message: refusal.Message,
		})
	}
	return &api.ValidationError{Fields: fields}
}
