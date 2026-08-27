package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
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
	// CompleteInterview seals at the final cursor, idempotently to the
	// receipt. Refusals arrive as *StartRefusedError with their codes.
	CompleteInterview(ctx context.Context, userID, sessionID string, epoch, finalSequence int) (CompletionReceiptView, error)
	// Results answers the stored evaluation. ErrResultNotReady while
	// evaluation has not landed; ErrSessionMissing when the session is
	// not the caller's to see.
	Results(ctx context.Context, userID, sessionID string) (EvaluationResultView, error)
	// Brief assembles what the interviewer needs, scoped as the candidate
	// the agent heard. ErrSessionMissing for anything it may not see.
	Brief(ctx context.Context, sessionID, candidateID, mode string) (BriefView, error)
	// IngestServiceEvents lands the agent's events for a session under the
	// candidate the agent heard, epoch and sequences assigned server-side.
	IngestServiceEvents(ctx context.Context, sessionID, candidateID, mode string, events []ControlEventIn) (ControlAck, error)
	// MySessions answers every session the caller owns, newest first,
	// every lifecycle state included.
	MySessions(ctx context.Context, userID string) ([]InterviewSession, error)
	// Review answers the derived coaching. Same refusals as Results; a
	// coaching failure is NOT an error here - it arrives as a view with
	// CoachingAvailable false, because the evaluation stands either way.
	Review(ctx context.Context, userID, sessionID string) (ReviewView, error)
}

// ReviewView mirrors the contract at the port.
type ReviewView struct {
	SessionID         string
	CoachingVersion   string
	CoachingAvailable bool
	Note              string
	Answers           []AnswerCoachingView
}

// AnswerCoachingView is one answer's coaching.
type AnswerCoachingView struct {
	Sequence  int
	Strengths []CoachingPointView
	Gaps      []CoachingPointView
	Rewrite   []RewritePartView
}

// CoachingPointView is one statement about one exact quote.
type CoachingPointView struct {
	Statement string
	Quote     string
}

// RewritePartView is a candidate quote or a bracketed question.
type RewritePartView struct {
	Kind string
	Text string
}

// ErrResultNotReady says evaluation has not landed yet: the session is
// real and the caller's own, so this is a state, never a 404.
var ErrResultNotReady = errors.New("api: the evaluation result is not ready")

// EvaluationResultView mirrors the contract at the port.
type EvaluationResultView struct {
	SessionID           string
	Rubric              RubricPinView
	AggregationVersion  string
	ExtractionVersion   string
	ModelVersion        string
	PolicyVersion       string
	Competencies        []CompetencyResultView
	Evidence            []EvidenceSpanView
	Contradictions      []ContradictionView
	Delivery            DeliveryView
	CoverageReached     []string
	CoverageNotReached  []string
	CoveredCompetencies int
	TotalCompetencies   int
	ResultDigest        string
	Warnings            []string
	CreatedAt           time.Time
}

// RubricPinView is the rubric the session composed with, echoed so a
// result is always read against exactly what judged it.
type RubricPinView struct {
	Reference string
	Version   string
	Digest    string
}

// FramingUnverified ships beside every unverified count. Server-supplied
// so no consumer can drop or reword it: unverified does not mean untrue.
const FramingUnverified = "Unverified means nobody checked this claim during the session. " +
	"It does not mean the claim is untrue."

// FramingContradictions ships beside every contradiction pair.
const FramingContradictions = "These statements appear to conflict. " +
	"Treat this as something to ask about, not as a conclusion about the person."

// FramingConfidence ships beside every confidence label (ADR-0015).
const FramingConfidence = "Confidence describes how much verifiable evidence this session produced " +
	"for each competency. It is not a prediction of performance in any role."

// DeliveryView is the delivery analysis's assessability, from its own
// workflow. Status pending while it has not landed.
type DeliveryView struct {
	Status   string
	Warnings []string
	Note     string
}

// FramingDeliveryNotAssessable ships with every not-assessable delivery:
// a statement about the recording or transcript, never about the person.
const FramingDeliveryNotAssessable = "Delivery was not assessable for this session. That is a statement " +
	"about the recording or the transcript, not about you: it is not a low result, and it has not " +
	"affected any score."

// EvidenceSpanView is one stored span: the exact sentence behind a score.
type EvidenceSpanView struct {
	ID              string
	CompetencyID    string
	Kind            string
	Quote           string
	SegmentSequence int
	StartMs         int
	EndMs           int
}

// ContradictionView pairs two statements that appear to conflict, both
// sides quoted with timestamps. A clarification prompt, never a judgment.
type ContradictionView struct {
	Topic []string
	SideA ContradictionSideView
	SideB ContradictionSideView
}

// ContradictionSideView is one quoted side on the room clock.
type ContradictionSideView struct {
	SegmentSequence int
	Quote           string
	StartMs         int
	EndMs           int
}

// CompetencyResultView is one competency's outcome. Band is empty
// whenever Status is unassessed: unknown is a state, never a low score.
type CompetencyResultView struct {
	CompetencyID  string
	Status        string
	Confidence    string
	Band          string
	EvidenceCount int
	Supporting    int
	Contradictory int
	Unverified    int
	Gaps          int
	EvidenceIDs   []string
	ReasonCodes   []string
}

// CompletionReceiptView mirrors the contract at the port.
type CompletionReceiptView struct {
	SessionID        string
	State            string
	SealedEpoch      int
	SealedSequence   int
	Gaps             [][2]int
	TranscriptDigest string
	BundleDigest     string
	MediaStatus      string
	Warnings         []string
	SealedAt         time.Time
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
	Timing   TimingPolicyView
}

// TimingPolicyView is the versioned timing rules stamped at start. The
// client renders grace countdowns from these; it compiles in none.
type TimingPolicyView struct {
	PolicyVersion         int
	ReconnectGraceSeconds int
	MaxOverrunSeconds     int
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
	// ConnectionEpoch and AcceptedSequence are the durable timeline's
	// cursor: what completion seals at. Zero before any attempt.
	ConnectionEpoch  int
	AcceptedSequence int
	// Seal is the durable completion receipt, nil until sealed.
	Seal      *SealView
	CreatedAt time.Time
}

// SealView is the receipt's durable summary.
type SealView struct {
	SealedAt    time.Time
	MediaStatus string
	Warnings    []string
}

// interviews handles the /interviews operations.
type interviews struct {
	authentication *authentication
	flows          Interviews
	// agentToken is the service credential the internal operations check,
	// in constant time. Empty means no internal surface exists.
	agentToken string
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
	if session.ConnectionEpoch > 0 {
		body.Cursor = &struct {
			AcceptedSequence int `json:"accepted_sequence"`
			ConnectionEpoch  int `json:"connection_epoch"`
		}{AcceptedSequence: session.AcceptedSequence, ConnectionEpoch: session.ConnectionEpoch}
	}
	if session.Seal != nil {
		warnings := session.Seal.Warnings
		if warnings == nil {
			warnings = []string{}
		}
		body.Seal = &struct {
			MediaStatus prepeetapi.InterviewSessionSealMediaStatus `json:"media_status"`
			SealedAt    time.Time                                  `json:"sealed_at"`
			Warnings    []string                                   `json:"warnings"`
		}{
			MediaStatus: prepeetapi.InterviewSessionSealMediaStatus(session.Seal.MediaStatus),
			SealedAt:    session.Seal.SealedAt,
			Warnings:    warnings,
		}
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
			Timing: prepeetapi.TimingPolicyView{
				PolicyVersion:         started.Timing.PolicyVersion,
				ReconnectGraceSeconds: started.Timing.ReconnectGraceSeconds,
				MaxOverrunSeconds:     started.Timing.MaxOverrunSeconds,
			},
		},
		Headers: prepeetapi.StartInterview200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// BriefView mirrors the contract at the port.
type BriefView struct {
	SessionID    string
	Minutes      int
	PersonaName  string
	PersonaStyle string
	PersonaAbout string
	RoleTitle    string
	Competencies []string
	Plan         json.RawMessage
}

// GetInterviewBrief answers the agent's brief under the service token.
func (i *interviews) GetInterviewBrief(ctx context.Context, request prepeetapi.GetInterviewBriefRequestObject) (prepeetapi.GetInterviewBriefResponseObject, error) {
	presented := bearerFromContext(ctx)
	if i.agentToken == "" || presented == "" ||
		subtle.ConstantTimeCompare([]byte(presented), []byte(i.agentToken)) != 1 {
		return i.authentication.rejectedSession(ctx), nil
	}

	brief, err := i.flows.Brief(ctx, request.SessionID.String(),
		request.Params.CandidateID.String(), string(request.Params.Mode))
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}
	sessionID, err := uuid.Parse(brief.SessionID)
	if err != nil {
		return nil, fmt.Errorf("api: the brief's session id is not a uuid: %w", err)
	}
	var plan map[string]any
	if len(brief.Plan) > 0 {
		if err := json.Unmarshal(brief.Plan, &plan); err != nil {
			return nil, fmt.Errorf("api: the pinned plan is not an object: %w", err)
		}
	}
	if plan == nil {
		plan = map[string]any{}
	}
	competencies := brief.Competencies
	if competencies == nil {
		competencies = []string{}
	}
	body := prepeetapi.InterviewBrief{SessionID: sessionID, Minutes: brief.Minutes, Plan: plan}
	body.Persona.Name = brief.PersonaName
	body.Persona.Style = brief.PersonaStyle
	body.Persona.Description = brief.PersonaAbout
	body.Role.Title = brief.RoleTitle
	body.Role.Competencies = competencies
	return prepeetapi.GetInterviewBrief200JSONResponse{
		Body:    body,
		Headers: prepeetapi.GetInterviewBrief200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// ackBody encodes an acknowledgment for the wire, shared by both ingest
// paths so the agent and the browser read the same shape.
func ackBody(ack ControlAck) (prepeetapi.ControlAcknowledgment, error) {
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
			return prepeetapi.ControlAcknowledgment{}, err
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
	return body, nil
}

// IngestServiceEvents lands the voice agent's events (ADR-0019). The
// credential is the deployment's service token, compared in constant
// time; a missing or wrong token, or a deployment with none configured,
// answers the same 401 so the surface reveals nothing about itself.
func (i *interviews) IngestServiceEvents(ctx context.Context, request prepeetapi.IngestServiceEventsRequestObject) (prepeetapi.IngestServiceEventsResponseObject, error) {
	presented := bearerFromContext(ctx)
	if i.agentToken == "" || presented == "" ||
		subtle.ConstantTimeCompare([]byte(presented), []byte(i.agentToken)) != 1 {
		return i.authentication.rejectedSession(ctx), nil
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
			EventID: event.EventID.String(), Type: event.Type,
			Payload: payload, OccurredAt: event.OccurredAt,
		})
	}

	ack, err := i.flows.IngestServiceEvents(ctx, request.SessionID.String(),
		request.Body.CandidateID.String(), string(request.Body.Mode), events)
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}
	body, err := ackBody(ack)
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}
	return prepeetapi.IngestServiceEvents200JSONResponse{
		Body:    body,
		Headers: prepeetapi.IngestServiceEvents200ResponseHeaders{CacheControl: NoStore},
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

	body, err := ackBody(ack)
	if err != nil {
		return i.authentication.failed(ctx, err), nil
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

// CompleteInterview seals the session.
func (i *interviews) CompleteInterview(ctx context.Context, request prepeetapi.CompleteInterviewRequestObject) (prepeetapi.CompleteInterviewResponseObject, error) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		return i.authentication.rejectedSession(ctx), nil
	}
	principal, err := i.authentication.identity.Lookup(ctx, presented)
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}

	receipt, err := i.flows.CompleteInterview(ctx, principal.UserID, request.SessionID.String(),
		request.Body.ConnectionEpoch, request.Body.FinalSequence)
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}

	sessionID, err := uuid.Parse(receipt.SessionID)
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}
	body := prepeetapi.CompletionReceipt{
		SessionID: sessionID, State: receipt.State,
		SealedEpoch: receipt.SealedEpoch, SealedSequence: receipt.SealedSequence,
		TranscriptDigest: receipt.TranscriptDigest,
		MediaStatus:      prepeetapi.CompletionReceiptMediaStatus(receipt.MediaStatus),
		Warnings:         receipt.Warnings,
		SealedAt:         receipt.SealedAt,
		Gaps: make([]struct {
			From int `json:"from"`
			To   int `json:"to"`
		}, 0, len(receipt.Gaps)),
	}
	if body.Warnings == nil {
		body.Warnings = []string{}
	}
	if receipt.BundleDigest != "" {
		digest := receipt.BundleDigest
		body.BundleDigest = &digest
	}
	for _, gap := range receipt.Gaps {
		body.Gaps = append(body.Gaps, struct {
			From int `json:"from"`
			To   int `json:"to"`
		}{From: gap[0], To: gap[1]})
	}
	return prepeetapi.CompleteInterview200JSONResponse{
		Body:    body,
		Headers: prepeetapi.CompleteInterview200ResponseHeaders{CacheControl: NoStore},
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

// GetResults answers the stored evaluation with sufficiency and coverage.
func (i *interviews) GetResults(ctx context.Context, request prepeetapi.GetResultsRequestObject) (prepeetapi.GetResultsResponseObject, error) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		return i.authentication.rejectedSession(ctx), nil
	}
	principal, err := i.authentication.identity.Lookup(ctx, presented)
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}

	result, err := i.flows.Results(ctx, principal.UserID, request.SessionID.String())
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}

	sessionID, err := uuid.Parse(result.SessionID)
	if err != nil {
		return nil, fmt.Errorf("api: the result's session id is not a uuid: %w", err)
	}
	body := prepeetapi.EvaluationResultView{
		SessionID:           sessionID,
		AggregationVersion:  result.AggregationVersion,
		ExtractionVersion:   result.ExtractionVersion,
		ModelVersion:        result.ModelVersion,
		PolicyVersion:       result.PolicyVersion,
		Competencies:        make([]prepeetapi.CompetencyResultView, 0, len(result.Competencies)),
		CoveredCompetencies: result.CoveredCompetencies,
		TotalCompetencies:   result.TotalCompetencies,
		ResultDigest:        result.ResultDigest,
		Warnings:            result.Warnings,
		CreatedAt:           result.CreatedAt,
	}
	body.Rubric.Reference = result.Rubric.Reference
	body.Rubric.Version = result.Rubric.Version
	body.Rubric.Digest = result.Rubric.Digest
	body.Coverage.Reached = result.CoverageReached
	body.Coverage.NotReached = result.CoverageNotReached
	if body.Coverage.Reached == nil {
		body.Coverage.Reached = []string{}
	}
	if body.Coverage.NotReached == nil {
		body.Coverage.NotReached = []string{}
	}
	if body.Warnings == nil {
		body.Warnings = []string{}
	}
	body.Evidence = make([]prepeetapi.EvidenceSpanView, 0, len(result.Evidence))
	for _, span := range result.Evidence {
		body.Evidence = append(body.Evidence, prepeetapi.EvidenceSpanView{
			ID: span.ID, CompetencyID: span.CompetencyID,
			Kind:  prepeetapi.EvidenceSpanViewKind(span.Kind),
			Quote: span.Quote, SegmentSequence: span.SegmentSequence,
			StartMs: span.StartMs, EndMs: span.EndMs,
		})
	}
	body.Contradictions = make([]prepeetapi.ContradictionView, 0, len(result.Contradictions))
	for _, pair := range result.Contradictions {
		encoded := prepeetapi.ContradictionView{Topic: pair.Topic}
		if encoded.Topic == nil {
			encoded.Topic = []string{}
		}
		encoded.SideA = prepeetapi.ContradictionSideView{
			SegmentSequence: pair.SideA.SegmentSequence, Quote: pair.SideA.Quote,
			StartMs: pair.SideA.StartMs, EndMs: pair.SideA.EndMs,
		}
		encoded.SideB = prepeetapi.ContradictionSideView{
			SegmentSequence: pair.SideB.SegmentSequence, Quote: pair.SideB.Quote,
			StartMs: pair.SideB.StartMs, EndMs: pair.SideB.EndMs,
		}
		body.Contradictions = append(body.Contradictions, encoded)
	}
	// The framing ships from the server so no consumer can drop or reword
	// it: unverified does not mean untrue, and a contradiction is a
	// question, not a conclusion.
	body.Framing.Unverified = FramingUnverified
	body.Framing.Contradictions = FramingContradictions
	body.Framing.Confidence = FramingConfidence
	body.Delivery.Status = prepeetapi.EvaluationResultViewDeliveryStatus(result.Delivery.Status)
	if body.Delivery.Status == "" {
		body.Delivery.Status = "pending"
	}
	body.Delivery.Warnings = result.Delivery.Warnings
	if body.Delivery.Warnings == nil {
		body.Delivery.Warnings = []string{}
	}
	body.Delivery.Note = result.Delivery.Note
	if body.Delivery.Status == "not_assessable" {
		body.Delivery.Note = FramingDeliveryNotAssessable
	}
	for _, competency := range result.Competencies {
		encoded := prepeetapi.CompetencyResultView{
			CompetencyID:  competency.CompetencyID,
			Status:        prepeetapi.CompetencyResultViewStatus(competency.Status),
			Confidence:    prepeetapi.CompetencyResultViewConfidence(competency.Confidence),
			EvidenceIds:   competency.EvidenceIDs,
			EvidenceCount: competency.EvidenceCount,
			Supporting:    competency.Supporting,
			Contradictory: competency.Contradictory,
			Unverified:    competency.Unverified,
			Gaps:          competency.Gaps,
			ReasonCodes:   competency.ReasonCodes,
		}
		if encoded.ReasonCodes == nil {
			encoded.ReasonCodes = []string{}
		}
		if encoded.EvidenceIds == nil {
			encoded.EvidenceIds = []string{}
		}
		if competency.Band != "" {
			band := competency.Band
			encoded.Band = &band
		}
		body.Competencies = append(body.Competencies, encoded)
	}
	return prepeetapi.GetResults200JSONResponse{
		Body:    body,
		Headers: prepeetapi.GetResults200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// GetReview answers the derived coaching review.
func (i *interviews) GetReview(ctx context.Context, request prepeetapi.GetReviewRequestObject) (prepeetapi.GetReviewResponseObject, error) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		return i.authentication.rejectedSession(ctx), nil
	}
	principal, err := i.authentication.identity.Lookup(ctx, presented)
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}

	review, err := i.flows.Review(ctx, principal.UserID, request.SessionID.String())
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}
	sessionID, err := uuid.Parse(review.SessionID)
	if err != nil {
		return nil, fmt.Errorf("api: the review's session id is not a uuid: %w", err)
	}

	body := prepeetapi.ReviewView{
		SessionID:         sessionID,
		CoachingVersion:   review.CoachingVersion,
		CoachingAvailable: review.CoachingAvailable,
		Answers:           make([]prepeetapi.AnswerCoachingView, 0, len(review.Answers)),
	}
	if review.Note != "" {
		note := review.Note
		body.Note = &note
	}
	points := func(from []CoachingPointView) []prepeetapi.CoachingPointView {
		out := make([]prepeetapi.CoachingPointView, 0, len(from))
		for _, point := range from {
			out = append(out, prepeetapi.CoachingPointView{Statement: point.Statement, Quote: point.Quote})
		}
		return out
	}
	for _, answer := range review.Answers {
		encoded := prepeetapi.AnswerCoachingView{
			Sequence:  answer.Sequence,
			Strengths: points(answer.Strengths),
			Gaps:      points(answer.Gaps),
		}
		for _, part := range answer.Rewrite {
			encoded.Rewrite = append(encoded.Rewrite, struct {
				Kind prepeetapi.AnswerCoachingViewRewriteKind `json:"kind"`
				Text string                                   `json:"text"`
			}{Kind: prepeetapi.AnswerCoachingViewRewriteKind(part.Kind), Text: part.Text})
		}
		if encoded.Rewrite == nil {
			encoded.Rewrite = []struct {
				Kind prepeetapi.AnswerCoachingViewRewriteKind `json:"kind"`
				Text string                                   `json:"text"`
			}{}
		}
		body.Answers = append(body.Answers, encoded)
	}
	return prepeetapi.GetReview200JSONResponse{
		Body:    body,
		Headers: prepeetapi.GetReview200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// ListMySessions answers the caller's whole session history.
func (i *interviews) ListMySessions(ctx context.Context, _ prepeetapi.ListMySessionsRequestObject) (prepeetapi.ListMySessionsResponseObject, error) {
	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		return i.authentication.rejectedSession(ctx), nil
	}
	principal, err := i.authentication.identity.Lookup(ctx, presented)
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}

	sessions, err := i.flows.MySessions(ctx, principal.UserID)
	if err != nil {
		return i.authentication.failed(ctx, err), nil
	}
	encoded := make([]prepeetapi.InterviewSession, 0, len(sessions))
	for _, session := range sessions {
		body, err := interviewSessionBody(session)
		if err != nil {
			return nil, err
		}
		encoded = append(encoded, body)
	}
	return prepeetapi.ListMySessions200JSONResponse{
		Body: struct {
			Sessions []prepeetapi.InterviewSession `json:"sessions"`
		}{Sessions: encoded},
		Headers: prepeetapi.ListMySessions200ResponseHeaders{CacheControl: NoStore},
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
	_ prepeetapi.GetResultsResponseObject          = failure{}
	_ prepeetapi.GetReviewResponseObject           = failure{}
	_ prepeetapi.ListMySessionsResponseObject      = failure{}
	_ prepeetapi.IngestServiceEventsResponseObject = failure{}
	_ prepeetapi.GetInterviewBriefResponseObject   = failure{}
	_ prepeetapi.CompleteInterviewResponseObject   = failure{}
)

func (f failure) VisitCreateInterviewResponse(w http.ResponseWriter) error     { return f.write(w) }
func (f failure) VisitGetPracticeConsentResponse(w http.ResponseWriter) error  { return f.write(w) }
func (f failure) VisitGetInterviewResponse(w http.ResponseWriter) error        { return f.write(w) }
func (f failure) VisitStartInterviewResponse(w http.ResponseWriter) error      { return f.write(w) }
func (f failure) VisitIngestControlEventsResponse(w http.ResponseWriter) error { return f.write(w) }
func (f failure) VisitReplayControlEventsResponse(w http.ResponseWriter) error { return f.write(w) }
func (f failure) VisitGetTranscriptResponse(w http.ResponseWriter) error       { return f.write(w) }
func (f failure) VisitGetResultsResponse(w http.ResponseWriter) error          { return f.write(w) }
func (f failure) VisitGetReviewResponse(w http.ResponseWriter) error           { return f.write(w) }
func (f failure) VisitListMySessionsResponse(w http.ResponseWriter) error      { return f.write(w) }
func (f failure) VisitIngestServiceEventsResponse(w http.ResponseWriter) error { return f.write(w) }
func (f failure) VisitGetInterviewBriefResponse(w http.ResponseWriter) error   { return f.write(w) }
func (f failure) VisitCompleteInterviewResponse(w http.ResponseWriter) error   { return f.write(w) }
