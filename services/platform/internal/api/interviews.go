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
	// does not offer fails with a *ValidationError naming every bad field.
	CreatePractice(ctx context.Context, userID string, selection InterviewSelection) (InterviewSession, error)
}

// InterviewSelection is the wizard's validated choice set.
type InterviewSelection struct {
	Discipline string
	Role       string
	Shape      string
	Minutes    int
	Persona    string
}

// InterviewSession mirrors the contract's InterviewSession at the port.
type InterviewSession struct {
	ID        string
	Mode      string
	State     string
	Config    InterviewSelection
	CreatedAt time.Time
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

	created, err := i.flows.CreatePractice(ctx, principal.UserID, InterviewSelection{
		Discipline: request.Body.Discipline,
		Role:       request.Body.Role,
		Shape:      request.Body.Shape,
		Minutes:    request.Body.Minutes,
		Persona:    request.Body.Persona,
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
			CreatedAt: created.CreatedAt,
		},
		Headers: prepeetapi.CreateInterview201ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// The failure type must speak this operation's responses.
var _ prepeetapi.CreateInterviewResponseObject = failure{}

func (f failure) VisitCreateInterviewResponse(w http.ResponseWriter) error { return f.write(w) }
