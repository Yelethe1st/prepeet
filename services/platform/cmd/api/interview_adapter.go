package main

import (
	"context"
	"encoding/json"
	"fmt"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/internal/catalog"
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
		BlueprintID: "plan/" + selection.Shape,
		Config:      config,
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
		Config:    selection,
		CreatedAt: composing.CreatedAt,
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
