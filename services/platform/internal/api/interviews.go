package api

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
)

// The interview creation surface: CAT-04's server half. The wizard submits a
// catalogue selection; the server validates it against the catalogue - the
// browser's filtering is a courtesy, never the rule - and only practice can
// be created here at all. A screening interview is a recruiter's act through
// campaign endpoints, and the contract's mode enum refuses anything else
// before a handler runs.

// Interviews is what the API needs from the interview context, declared here
// per ADR-0005 and wired in cmd.
type Interviews interface {
	// CreatePractice validates the selection and creates a composing
	// practice session owned by the given user. A selection the catalogue
	// does not offer - or a consent version that is no longer the published
	// one - fails with a *ValidationError naming every bad field.
	CreatePractice(ctx context.Context, userID string, selection InterviewSelection) (InterviewSession, error)
	// PracticeConsent answers the currently published consent text, whose
	// version is what creation must echo back.
	PracticeConsent(ctx context.Context) (PracticeConsent, error)
}

// InterviewSelection is the wizard's validated choice set.
type InterviewSelection struct {
	Discipline string
	Role       string
	Shape      string
	Minutes    int
	Persona    string
	Recording  RecordingConsent
}

// RecordingConsent is what the session keeps and which text said so.
type RecordingConsent struct {
	Preference     string
	ConsentVersion string
}

// PracticeConsent mirrors the contract's consent document at the port.
type PracticeConsent struct {
	Version            string
	Title              string
	Statements         []string
	AudioAndTranscript ConsentChoiceView
	TranscriptOnly     ConsentChoiceView
}

// ConsentChoiceView is one recording choice, explained, costs named.
type ConsentChoiceView struct {
	Label       string
	Explanation string
	Forfeits    []string
}

// InterviewSession mirrors the contract's InterviewSession at the port.
type InterviewSession struct {
	ID                  string
	Mode                string
	State               string
	Config              InterviewSelection
	RecordingPreference string
	ConsentVersion      string
	CreatedAt           time.Time
}

// interviews handles the /interviews operations.
type interviews struct {
	authentication *authentication
	flows          Interviews
}

// CreateInterview creates a practice session from a catalogue selection.
func (i *interviews) CreateInterview(ctx context.Context, request prepeetapi.CreateInterviewRequestObject) (prepeetapi.CreateInterviewResponseObject, error) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		return i.authentication.rejectedSession(ctx), nil
	}
	principal, err := i.authentication.identity.Lookup(ctx, presented)
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}

	// The contract's enum names practice alone, and the check is enforced
	// here because the generated decoder does not: a screening interview is
	// a recruiter's act, and no candidate token makes it otherwise.
	if request.Body.Mode != prepeetapi.CreateInterviewRequestMode("practice") {
		return i.authentication.failed(ctx, Invalid("mode", "MODE_NOT_CREATABLE",
			"Only a practice interview can be created here. Screening interviews are created by a recruiter.")), nil
	}

	// The generated decoder does not enforce enums; the preference is
	// checked here for the same reason the mode is.
	preference := string(request.Body.Recording.Preference)
	if preference != "audio_and_transcript" && preference != "transcript_only" {
		return i.authentication.failed(ctx, Invalid("recording.preference", "RECORDING_PREFERENCE_UNKNOWN",
			"Choose what the session keeps: the audio and the transcript, or the transcript only.")), nil
	}

	created, err := i.flows.CreatePractice(ctx, principal.UserID, InterviewSelection{
		Discipline: request.Body.Discipline,
		Role:       request.Body.Role,
		Shape:      request.Body.Shape,
		Minutes:    request.Body.Minutes,
		Persona:    request.Body.Persona,
		Recording: RecordingConsent{
			Preference:     preference,
			ConsentVersion: request.Body.Recording.ConsentVersion,
		},
	})
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}

	id, err := uuid.Parse(created.ID)
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}
	return prepeetapi.CreateInterview201JSONResponse{
		Body: prepeetapi.InterviewSession{
			ID:    id,
			Mode:  prepeetapi.InterviewSessionMode(created.Mode),
			State: created.State,
			Config: prepeetapi.InterviewConfig{
				Discipline: created.Config.Discipline,
				Role:       created.Config.Role,
				Shape:      created.Config.Shape,
				Minutes:    created.Config.Minutes,
				Persona:    created.Config.Persona,
			},
			RecordingPreference: prepeetapi.InterviewSessionRecordingPreference(created.RecordingPreference),
			ConsentVersion:      created.ConsentVersion,
			CreatedAt:           created.CreatedAt,
		},
		Headers: prepeetapi.CreateInterview201ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// GetPracticeConsent answers the current consent text with its version.
func (i *interviews) GetPracticeConsent(ctx context.Context, _ prepeetapi.GetPracticeConsentRequestObject) (prepeetapi.GetPracticeConsentResponseObject, error) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		return i.authentication.rejectedSession(ctx), nil
	}
	if _, err := i.authentication.identity.Lookup(ctx, presented); err != nil {
		return i.authentication.failed(ctx, err), nil
	}

	consent, err := i.flows.PracticeConsent(ctx)
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}
	return prepeetapi.GetPracticeConsent200JSONResponse{
		Body: prepeetapi.PracticeConsent{
			Version:    consent.Version,
			Title:      consent.Title,
			Statements: consent.Statements,
			Choices: struct {
				AudioAndTranscript prepeetapi.ConsentChoice `json:"audio_and_transcript"`
				TranscriptOnly     prepeetapi.ConsentChoice `json:"transcript_only"`
			}{
				AudioAndTranscript: consentChoiceBody(consent.AudioAndTranscript),
				TranscriptOnly:     consentChoiceBody(consent.TranscriptOnly),
			},
		},
		Headers: prepeetapi.GetPracticeConsent200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

func consentChoiceBody(choice ConsentChoiceView) prepeetapi.ConsentChoice {
	body := prepeetapi.ConsentChoice{Label: choice.Label, Explanation: choice.Explanation}
	if len(choice.Forfeits) > 0 {
		forfeits := choice.Forfeits
		body.Forfeits = &forfeits
	}
	return body
}

// The failure type must speak these operations' responses.
var (
	_ prepeetapi.CreateInterviewResponseObject    = failure{}
	_ prepeetapi.GetPracticeConsentResponseObject = failure{}
)

func (f failure) VisitCreateInterviewResponse(w http.ResponseWriter) error    { return f.write(w) }
func (f failure) VisitGetPracticeConsentResponse(w http.ResponseWriter) error { return f.write(w) }
