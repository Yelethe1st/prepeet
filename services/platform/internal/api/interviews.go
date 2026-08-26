package api

import (
	"context"
	"encoding/json"
	"errors"
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
	// GetPractice answers one practice session as its owner sees it.
	// Somebody else's session fails with ErrSessionMissing, exactly like
	// none: existence is not answered across owners.
	GetPractice(ctx context.Context, userID, sessionID string) (InterviewSession, error)
	// StartPractice starts a ready session and mints its room grant. Each
	// refusal is a *StartRefusedError with its own stable code.
	StartPractice(ctx context.Context, userID, sessionID string) (StartedInterview, error)
	// IngestEvents accepts one control event batch for the current epoch.
	// A superseded epoch refuses whole with a *StartRefusedError carrying
	// EPOCH_STALE or NO_ATTEMPT.
	IngestEvents(ctx context.Context, userID, sessionID string, epoch int, events []ControlEventIn) (ControlAck, error)
	// ReplayEvents answers the durable timeline after a cursor.
	ReplayEvents(ctx context.Context, userID, sessionID string, afterEpoch, afterSequence int) ([]ControlEventOut, error)
	// Transcript answers the assembled read model, provenance included.
	Transcript(ctx context.Context, userID, sessionID string) (TranscriptView, error)
}

// TranscriptView mirrors the contract at the port.
type TranscriptView struct {
	Segments          []TranscriptSegmentView
	OrphanCorrections []int
}

// TranscriptSegmentView is one segment with its provenance links.
type TranscriptSegmentView struct {
	Epoch               int
	Sequence            int
	Type                string
	Speaker             string
	Text                string
	StartMs             int
	EndMs               int
	Confidence          float64
	Words               []TranscriptWordView
	Superseded          bool
	CorrectedBySequence int
	Supersedes          int
}

// TranscriptWordView is one word on the room clock.
type TranscriptWordView struct {
	Word       string
	StartMs    int
	EndMs      int
	Confidence float64
}

// ControlEventIn is one envelope from the browser.
type ControlEventIn struct {
	EventID    string
	Sequence   int
	Type       string
	Payload    json.RawMessage
	OccurredAt time.Time
}

// ControlEventOut is one stored event, replayed.
type ControlEventOut struct {
	EventID    string
	Epoch      int
	Sequence   int
	Type       string
	Payload    json.RawMessage
	OccurredAt time.Time
}

// ControlAck mirrors the contract's acknowledgment at the port.
type ControlAck struct {
	Epoch    int
	Accepted int
	Missing  [][2]int
	Outcomes []ControlOutcome
}

// ControlOutcome is one event's verdict.
type ControlOutcome struct {
	EventID string
	Status  string
	Reason  string
}

// StartRefusedError is one of start's distinct refusals, carried with the
// stable code the response repeats.
type StartRefusedError struct {
	Code    string
	Message string
}

func (e *StartRefusedError) Error() string { return "api: start refused: " + e.Code }

// StartedInterview is the start command's answer at the port.
type StartedInterview struct {
	Session  InterviewSession
	Realtime RoomGrantView
}

// RoomGrantView mirrors the contract's RoomGrant.
type RoomGrantView struct {
	URL       string
	Room      string
	Token     string
	ExpiresAt time.Time
}

// ErrSessionMissing covers absence and somebody else's session alike.
var ErrSessionMissing = errors.New("api: no such session")

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
	FailureCode         string
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

	body, err := interviewSessionBody(created)
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}
	return prepeetapi.CreateInterview201JSONResponse{
		Body:    body,
		Headers: prepeetapi.CreateInterview201ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// interviewSessionBody encodes one session for the wire.
func interviewSessionBody(session InterviewSession) (prepeetapi.InterviewSession, error) {
	id, err := uuid.Parse(session.ID)
	if err != nil {
		return prepeetapi.InterviewSession{}, err
	}
	body := prepeetapi.InterviewSession{
		ID:    id,
		Mode:  prepeetapi.InterviewSessionMode(session.Mode),
		State: session.State,
		Config: prepeetapi.InterviewConfig{
			Discipline: session.Config.Discipline,
			Role:       session.Config.Role,
			Shape:      session.Config.Shape,
			Minutes:    session.Config.Minutes,
			Persona:    session.Config.Persona,
		},
		RecordingPreference: prepeetapi.InterviewSessionRecordingPreference(session.RecordingPreference),
		ConsentVersion:      session.ConsentVersion,
		CreatedAt:           session.CreatedAt,
	}
	if session.FailureCode != "" {
		code := session.FailureCode
		body.FailureCode = &code
	}
	return body, nil
}

// GetInterview answers one session for the prepare screen.
func (i *interviews) GetInterview(ctx context.Context, request prepeetapi.GetInterviewRequestObject) (prepeetapi.GetInterviewResponseObject, error) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		return i.authentication.rejectedSession(ctx), nil
	}
	principal, err := i.authentication.identity.Lookup(ctx, presented)
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}

	session, err := i.flows.GetPractice(ctx, principal.UserID, request.SessionID.String())
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}
	body, err := interviewSessionBody(session)
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}
	return prepeetapi.GetInterview200JSONResponse{
		Body:    body,
		Headers: prepeetapi.GetInterview200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// StartInterview starts a ready session and answers with its room grant.
func (i *interviews) StartInterview(ctx context.Context, request prepeetapi.StartInterviewRequestObject) (prepeetapi.StartInterviewResponseObject, error) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		return i.authentication.rejectedSession(ctx), nil
	}
	principal, err := i.authentication.identity.Lookup(ctx, presented)
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}

	started, err := i.flows.StartPractice(ctx, principal.UserID, request.SessionID.String())
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}
	session, err := interviewSessionBody(started.Session)
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}
	return prepeetapi.StartInterview200JSONResponse{
		Body: prepeetapi.StartedInterview{
			Session: session,
			Realtime: prepeetapi.RoomGrant{
				URL:       started.Realtime.URL,
				Room:      started.Realtime.Room,
				Token:     started.Realtime.Token,
				ExpiresAt: started.Realtime.ExpiresAt,
			},
		},
		Headers: prepeetapi.StartInterview200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// IngestControlEvents accepts a control event batch.
func (i *interviews) IngestControlEvents(ctx context.Context, request prepeetapi.IngestControlEventsRequestObject) (prepeetapi.IngestControlEventsResponseObject, error) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		return i.authentication.rejectedSession(ctx), nil
	}
	principal, err := i.authentication.identity.Lookup(ctx, presented)
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}

	events := make([]ControlEventIn, 0, len(request.Body.Events))
	for _, event := range request.Body.Events {
		var payload json.RawMessage
		if event.Payload != nil {
			encoded, err := json.Marshal(event.Payload)
			if err != nil {
				return i.authentication.failed(ctx, err), nil
			}
			payload = encoded
		}
		events = append(events, ControlEventIn{
			EventID: event.EventID.String(), Sequence: event.Sequence,
			Type: event.Type, Payload: payload, OccurredAt: event.OccurredAt,
		})
	}

	ack, err := i.flows.IngestEvents(ctx, principal.UserID, request.SessionID.String(),
		request.Body.ConnectionEpoch, events)
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}

	body := prepeetapi.ControlAcknowledgment{
		ConnectionEpoch:  ack.Epoch,
		AcceptedSequence: ack.Accepted,
		Missing: make([]struct {
			From int `json:"from"`
			To   int `json:"to"`
		}, 0, len(ack.Missing)),
	}
	for _, gap := range ack.Missing {
		body.Missing = append(body.Missing, struct {
			From int `json:"from"`
			To   int `json:"to"`
		}{From: gap[0], To: gap[1]})
	}
	for _, outcome := range ack.Outcomes {
		id, err := uuid.Parse(outcome.EventID)
		if err != nil {
			return i.authentication.failed(ctx, err), nil
		}
		encoded := struct {
			EventID uuid.UUID                                      `json:"event_id"`
			Reason  *string                                        `json:"reason,omitempty"`
			Status  prepeetapi.ControlAcknowledgmentOutcomesStatus `json:"status"`
		}{EventID: id, Status: prepeetapi.ControlAcknowledgmentOutcomesStatus(outcome.Status)}
		if outcome.Reason != "" {
			reason := outcome.Reason
			encoded.Reason = &reason
		}
		body.Outcomes = append(body.Outcomes, encoded)
	}
	return prepeetapi.IngestControlEvents200JSONResponse{
		Body:    body,
		Headers: prepeetapi.IngestControlEvents200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// ReplayControlEvents answers the timeline after a cursor.
func (i *interviews) ReplayControlEvents(ctx context.Context, request prepeetapi.ReplayControlEventsRequestObject) (prepeetapi.ReplayControlEventsResponseObject, error) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		return i.authentication.rejectedSession(ctx), nil
	}
	principal, err := i.authentication.identity.Lookup(ctx, presented)
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}

	afterEpoch, afterSequence := 0, 0
	if request.Params.AfterEpoch != nil {
		afterEpoch = *request.Params.AfterEpoch
	}
	if request.Params.AfterSequence != nil {
		afterSequence = *request.Params.AfterSequence
	}
	replayed, err := i.flows.ReplayEvents(ctx, principal.UserID, request.SessionID.String(), afterEpoch, afterSequence)
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}

	body := prepeetapi.ControlEventList{}
	for _, event := range replayed {
		id, err := uuid.Parse(event.EventID)
		if err != nil {
			return i.authentication.failed(ctx, err), nil
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return i.authentication.failed(ctx, err), nil
		}
		body.Events = append(body.Events, struct {
			ConnectionEpoch int                    `json:"connection_epoch"`
			EventID         uuid.UUID              `json:"event_id"`
			OccurredAt      time.Time              `json:"occurred_at"`
			Payload         map[string]interface{} `json:"payload"`
			Sequence        int                    `json:"sequence"`
			Type            string                 `json:"type"`
		}{
			ConnectionEpoch: event.Epoch, EventID: id, OccurredAt: event.OccurredAt,
			Payload: payload, Sequence: event.Sequence, Type: event.Type,
		})
	}
	if body.Events == nil {
		body.Events = []struct {
			ConnectionEpoch int                    `json:"connection_epoch"`
			EventID         uuid.UUID              `json:"event_id"`
			OccurredAt      time.Time              `json:"occurred_at"`
			Payload         map[string]interface{} `json:"payload"`
			Sequence        int                    `json:"sequence"`
			Type            string                 `json:"type"`
		}{}
	}
	return prepeetapi.ReplayControlEvents200JSONResponse{
		Body:    body,
		Headers: prepeetapi.ReplayControlEvents200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// GetTranscript answers the assembled transcript.
func (i *interviews) GetTranscript(ctx context.Context, request prepeetapi.GetTranscriptRequestObject) (prepeetapi.GetTranscriptResponseObject, error) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		return i.authentication.rejectedSession(ctx), nil
	}
	principal, err := i.authentication.identity.Lookup(ctx, presented)
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}

	transcript, err := i.flows.Transcript(ctx, principal.UserID, request.SessionID.String())
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}

	body := prepeetapi.TranscriptView{
		Segments:          make([]prepeetapi.TranscriptSegment, 0, len(transcript.Segments)),
		OrphanCorrections: transcript.OrphanCorrections,
	}
	if body.OrphanCorrections == nil {
		body.OrphanCorrections = []int{}
	}
	for _, segment := range transcript.Segments {
		encoded := prepeetapi.TranscriptSegment{
			Epoch: segment.Epoch, Sequence: segment.Sequence,
			Type:    prepeetapi.TranscriptSegmentType(segment.Type),
			Speaker: prepeetapi.TranscriptSegmentSpeaker(segment.Speaker),
			Text:    segment.Text, StartMs: segment.StartMs, EndMs: segment.EndMs,
			Confidence: float32(segment.Confidence), Superseded: segment.Superseded,
		}
		if segment.CorrectedBySequence != 0 {
			linked := segment.CorrectedBySequence
			encoded.CorrectedBySequence = &linked
		}
		if segment.Supersedes != 0 {
			linked := segment.Supersedes
			encoded.Supersedes = &linked
		}
		if len(segment.Words) > 0 {
			transcriptWords := make([]struct {
				Confidence float32 `json:"confidence"`
				EndMs      int     `json:"end_ms"`
				StartMs    int     `json:"start_ms"`
				W          string  `json:"w"`
			}, 0, len(segment.Words))
			for _, word := range segment.Words {
				transcriptWords = append(transcriptWords, struct {
					Confidence float32 `json:"confidence"`
					EndMs      int     `json:"end_ms"`
					StartMs    int     `json:"start_ms"`
					W          string  `json:"w"`
				}{Confidence: float32(word.Confidence), EndMs: word.EndMs, StartMs: word.StartMs, W: word.Word})
			}
			encoded.Words = &transcriptWords
		}
		body.Segments = append(body.Segments, encoded)
	}
	return prepeetapi.GetTranscript200JSONResponse{
		Body:    body,
		Headers: prepeetapi.GetTranscript200ResponseHeaders{CacheControl: NoStore},
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
	_ prepeetapi.CreateInterviewResponseObject     = failure{}
	_ prepeetapi.GetPracticeConsentResponseObject  = failure{}
	_ prepeetapi.GetInterviewResponseObject        = failure{}
	_ prepeetapi.StartInterviewResponseObject      = failure{}
	_ prepeetapi.IngestControlEventsResponseObject = failure{}
	_ prepeetapi.ReplayControlEventsResponseObject = failure{}
	_ prepeetapi.GetTranscriptResponseObject       = failure{}
)

func (f failure) VisitCreateInterviewResponse(w http.ResponseWriter) error     { return f.write(w) }
func (f failure) VisitGetPracticeConsentResponse(w http.ResponseWriter) error  { return f.write(w) }
func (f failure) VisitGetInterviewResponse(w http.ResponseWriter) error        { return f.write(w) }
func (f failure) VisitStartInterviewResponse(w http.ResponseWriter) error      { return f.write(w) }
func (f failure) VisitIngestControlEventsResponse(w http.ResponseWriter) error { return f.write(w) }
func (f failure) VisitReplayControlEventsResponse(w http.ResponseWriter) error { return f.write(w) }
func (f failure) VisitGetTranscriptResponse(w http.ResponseWriter) error       { return f.write(w) }
