package recruiting

// Accommodations: SCR-06's request and fulfilment path.
//
// Three facts, three append-only records: what was requested, what was
// granted and by whom, and which session it was actually applied to. The
// state a candidate sees is derived from those rows rather than stored, so
// it can never disagree with them.
//
// Two constraints from screen-mode.md and responsible-hiring.md are the
// architecture here rather than features of it.
//
// The first: an accommodation must never reach evaluation as a signal. That
// is not enforced by a flag evaluation promises not to read; it is enforced
// by evaluation being unable to read any of this at all. These tables live in
// the recruiting schema, internal/architecture's ownership gate refuses any
// other module a query that names it, and ADR-0010 leaves no hand-written SQL
// to walk around that. Captions, push-to-talk and extra time change how a
// session is conducted, and the conduct reaches the interview runner through
// the composition root; nothing on the scoring path can see that any of it
// happened.
//
// The second: the request is for a named adjustment, never a diagnosis. The
// request record deliberately has no free-text field, because a "reason" box
// on an accommodation form is where a medical condition gets asked for
// whether or not anybody meant to ask. A candidate names what they need from
// screen-mode.md's list and owes nobody an explanation of why.

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Adjustment is one named change to how a session is conducted. The
// vocabulary is screen-mode.md's "Accommodation and incidents" list,
// verbatim, and growing it starts in that document rather than here.
type Adjustment string

const (
	// AdjustmentCaptions shows live captions of the interviewer's speech.
	AdjustmentCaptions Adjustment = "captions"
	// AdjustmentPushToTalk replaces open-microphone voice detection with an
	// explicit control the candidate holds.
	AdjustmentPushToTalk Adjustment = "push_to_talk"
	// AdjustmentExtraTime extends thinking time. It changes the session's
	// clock, never its scoring: response latency is excluded from evaluation
	// for every candidate, so there is nothing for extra time to distort.
	AdjustmentExtraTime Adjustment = "extra_time"
	// AdjustmentAlternativePath is the assessment path for when voice is not
	// accessible at all, rather than an adjustment to a voice session.
	AdjustmentAlternativePath Adjustment = "alternative_path"
)

// Adjustments returns the vocabulary in a stable order.
func Adjustments() []Adjustment {
	return []Adjustment{
		AdjustmentCaptions, AdjustmentPushToTalk,
		AdjustmentExtraTime, AdjustmentAlternativePath,
	}
}

// SessionPhase is where the candidate's session is, said in this context's
// vocabulary. The interview context owns the real lifecycle; the composition
// root translates its state into one of these, because recruiting may not
// import interview (ADR-0005) and needs only this much of the answer.
type SessionPhase string

const (
	// PhaseNoSession means no session exists yet, which is the earliest and
	// commonest moment to ask: an invitation has been accepted and nothing
	// has been composed.
	PhaseNoSession SessionPhase = "no_session"
	// PhasePreparation covers a session that exists but has not begun:
	// composed, ready, connecting. An adjustment recorded now still shapes
	// the whole interview.
	PhasePreparation SessionPhase = "preparation"
	// PhaseUnderway means the interview is running. A need surfacing now is
	// SCR-08's incident path, which puts a human in the loop, rather than a
	// request that quietly reconfigures a session mid-answer.
	PhaseUnderway SessionPhase = "underway"
	// PhaseComplete means the interview already happened, and what its
	// conduct was is now a fact rather than a choice.
	PhaseComplete SessionPhase = "complete"
)

// Errors a caller distinguishes because each means a different next step.
var (
	// ErrUnknownAdjustment means the request named something outside
	// screen-mode.md's vocabulary. The screen offers the list; free text
	// never reaches here.
	ErrUnknownAdjustment = errors.New("recruiting: not a named adjustment")
	// ErrRequestTooLate means the interview is underway or over. The route
	// that remains is the incident path, and the message says so because the
	// candidate needs the door that is open, not just the one that closed.
	ErrRequestTooLate = errors.New("recruiting: the interview has begun; an accommodation need now goes to the incident path")
	// ErrUnknownPhase means the caller did not say where the session is.
	// Refused rather than read as "no session yet", because a caller that
	// forgot to ask would otherwise admit requests mid-interview, which is
	// the one window the phase rule exists to close.
	ErrUnknownPhase = errors.New("recruiting: the session phase was not stated")
	// ErrNoDecider means a decision arrived without a named human. A
	// fulfilment path with no accountable person is the thing
	// responsible-hiring.md's human-decision rule forbids.
	ErrNoDecider = errors.New("recruiting: an accommodation decision requires a named human")
	// ErrNotGranted means a fulfilment was attempted for a request nobody
	// granted, or whose grant was later withdrawn.
	ErrNotGranted = errors.New("recruiting: the accommodation is not granted")
)

// AccommodationRequest is a candidate asking for one named adjustment on one
// campaign. Deliberately nothing else: no reason, no condition, no note. The
// unit test asserting these exact fields is the design constraint written
// down, so a field for a diagnosis cannot appear without a reviewed diff.
type AccommodationRequest struct {
	ID          string
	TenantID    string
	CampaignID  string
	CandidateID string
	Adjustment  Adjustment
	RequestedAt time.Time
}

// AccommodationRequestInput is what a caller supplies to make one. Phase is
// required, not defaulted: the caller must have asked where the session is.
type AccommodationRequestInput struct {
	TenantID    string
	CampaignID  string
	CandidateID string
	Adjustment  Adjustment
	Phase       SessionPhase
}

// AccommodationDecision is one human's answer to one request. Append-only in
// the store: a change of mind is a later row, and the latest row per request
// is the standing answer, exactly as consent decisions work.
type AccommodationDecision struct {
	RequestID string
	Granted   bool
	DecidedBy string
	DecidedAt time.Time
}

// Fulfilment records that a granted adjustment was actually applied to a
// named session. It is the difference between an accommodation policy and an
// accommodated interview, which is what "exercised, not merely promised"
// asks for.
type Fulfilment struct {
	RequestID   string
	SessionID   string
	FulfilledAt time.Time
}

// RequestState is what a candidate sees of their request.
type RequestState string

const (
	// RequestStateRequested means nobody has answered yet.
	RequestStateRequested RequestState = "requested"
	// RequestStateGranted means the standing decision is a grant.
	RequestStateGranted RequestState = "granted"
	// RequestStateDeclined means the standing decision is a decline.
	RequestStateDeclined RequestState = "declined"
)

// AccommodationView is one request as the candidate sees it: the request,
// its derived state, and who answered when somebody has. The decider is
// shown because an answer from nobody is not an answer a person can follow
// up on.
type AccommodationView struct {
	Request   AccommodationRequest
	State     RequestState
	DecidedBy string
	DecidedAt *time.Time
}

// validAdjustments is the vocabulary as a set, for the refusal.
var validAdjustments = map[Adjustment]bool{
	AdjustmentCaptions: true, AdjustmentPushToTalk: true,
	AdjustmentExtraTime: true, AdjustmentAlternativePath: true,
}

// NewAccommodationRequest builds the record of a candidate asking.
//
// The phase rule is the first criterion's boundary: before a session and
// during its preparation the request is admitted, and once the interview is
// underway the need is real but the route is SCR-08's incident path, where a
// human decides what happens to a session already in flight.
func NewAccommodationRequest(input AccommodationRequestInput) (AccommodationRequest, error) {
	switch input.Phase {
	case PhaseNoSession, PhasePreparation:
		// The window the criterion names.
	case PhaseUnderway, PhaseComplete:
		return AccommodationRequest{}, fmt.Errorf("%w: the session is %s", ErrRequestTooLate, input.Phase)
	default:
		return AccommodationRequest{}, fmt.Errorf("%w: %q", ErrUnknownPhase, input.Phase)
	}

	if !validAdjustments[input.Adjustment] {
		return AccommodationRequest{}, fmt.Errorf("%w: %q", ErrUnknownAdjustment, input.Adjustment)
	}
	for name, value := range map[string]string{
		"tenant": input.TenantID, "campaign": input.CampaignID, "candidate": input.CandidateID,
	} {
		if strings.TrimSpace(value) == "" {
			return AccommodationRequest{}, fmt.Errorf("recruiting: a request must name its %s", name)
		}
	}

	return AccommodationRequest{
		TenantID:    input.TenantID,
		CampaignID:  input.CampaignID,
		CandidateID: input.CandidateID,
		Adjustment:  input.Adjustment,
		RequestedAt: time.Now().UTC(),
	}, nil
}

// NewAccommodationDecision builds one human's answer.
//
// A decline is as valid a decision as a grant; what is refused is a decision
// from nobody, for the reason the jurisdiction determination refuses an
// unnamed approver: a record with no accountable person evidences nothing.
func NewAccommodationDecision(requestID string, granted bool, decidedBy string) (AccommodationDecision, error) {
	if strings.TrimSpace(decidedBy) == "" {
		return AccommodationDecision{}, ErrNoDecider
	}
	if strings.TrimSpace(requestID) == "" {
		return AccommodationDecision{}, errors.New("recruiting: a decision must name its request")
	}
	return AccommodationDecision{
		RequestID: requestID,
		Granted:   granted,
		DecidedBy: decidedBy,
		DecidedAt: time.Now().UTC(),
	}, nil
}

// NewFulfilment records a granted adjustment being applied to a session.
//
// standing is the latest decision for the request, nil when nobody has
// answered. Both the nil and the declined case refuse for the same reason: a
// fulfilment is the execution of a grant, and executing a grant that does
// not stand would let the fulfilment path make the decision the human was
// supposed to. The store enforces the same rule again in the database, so a
// future caller that skips this function meets the same refusal there.
func NewFulfilment(requestID, sessionID string, standing *AccommodationDecision) (Fulfilment, error) {
	if standing == nil || !standing.Granted {
		return Fulfilment{}, fmt.Errorf("%w: request %s", ErrNotGranted, requestID)
	}
	if strings.TrimSpace(sessionID) == "" {
		return Fulfilment{}, errors.New("recruiting: a fulfilment must name the session it was applied to")
	}
	return Fulfilment{
		RequestID:   requestID,
		SessionID:   sessionID,
		FulfilledAt: time.Now().UTC(),
	}, nil
}

// StateOf derives what the candidate sees from the standing decision.
func StateOf(standing *AccommodationDecision) RequestState {
	switch {
	case standing == nil:
		return RequestStateRequested
	case standing.Granted:
		return RequestStateGranted
	default:
		return RequestStateDeclined
	}
}
