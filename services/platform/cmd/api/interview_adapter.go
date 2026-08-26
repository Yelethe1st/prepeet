package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/internal/billing"
	"github.com/Yelethe1st/prepeet/services/platform/internal/catalog"
	"github.com/Yelethe1st/prepeet/services/platform/internal/content"
	"github.com/Yelethe1st/prepeet/services/platform/internal/evaluation"
	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
	"github.com/Yelethe1st/prepeet/services/platform/platform/objectstore"
	"github.com/Yelethe1st/prepeet/services/platform/platform/realtime"
)

// interviewAdapter presents session creation as the port the API declared.
// It is the enforcement point CAT-03 left open: the one place that sees both
// the catalogue and the interview context, so the selection is validated
// against the former before the latter ever hears about it.
type interviewAdapter struct {
	catalogue *catalog.Service
	sessions  *interview.Store
	registry  *content.Store
	starter   *interview.Starter
	events    *interview.Events
	completer *interview.Completer
	results   *evaluation.Store
	documents *objectstore.S3Store
}

// Review derives the coaching for the owner's evaluated session. The
// derivation is pure and gated: coaching-1 over the stored evidence and
// the sealed input, held to the fact-preservation gate before serving. A
// gate refusal or a missing input is a stated absence with the evaluation
// intact, exactly as PRC-02's third box demands - never a failed request.
func (a interviewAdapter) Review(ctx context.Context, userID, sessionID string) (api.ReviewView, error) {
	if _, err := a.sessions.Get(ctx, sessionID, "practice", userID, ""); err != nil {
		if errors.Is(err, interview.ErrNotFound) {
			return api.ReviewView{}, api.ErrSessionMissing
		}
		return api.ReviewView{}, err
	}
	ref := evaluation.SessionRef{SessionID: sessionID, Mode: "practice", CandidateID: userID}
	if _, err := a.results.ResultOf(ctx, ref); err != nil {
		if errors.Is(err, evaluation.ErrNoResult) {
			return api.ReviewView{}, api.ErrResultNotReady
		}
		return api.ReviewView{}, err
	}

	unavailable := func(note string) api.ReviewView {
		return api.ReviewView{
			SessionID: sessionID, CoachingVersion: evaluation.CoachingVersion,
			CoachingAvailable: false, Note: note,
			Answers: []api.AnswerCoachingView{},
		}
	}
	intact := "Coaching could not be derived for this session. Your evaluation is complete and unaffected."

	key, err := objectstore.SealedInputKey("practice", "", userID, sessionID)
	if err != nil {
		return unavailable(intact), nil
	}
	body, err := a.documents.Fetch(ctx, key)
	if err != nil {
		return unavailable(intact), nil
	}
	sealed, err := evaluation.DecodeSealedInput(body)
	if err != nil {
		return unavailable(intact), nil
	}
	spans, err := a.results.List(ctx, ref)
	if err != nil {
		return api.ReviewView{}, err
	}

	review := evaluation.Coach(sealed, spans)
	if err := evaluation.ValidateCoaching(sealed, review); err != nil {
		// The gate refused its own floor: a bug worth logging loudly, but
		// never one that touches the stored evaluation.
		return unavailable(intact), nil
	}

	view := api.ReviewView{
		SessionID: review.SessionID, CoachingVersion: review.CoachingVersion,
		CoachingAvailable: true, Answers: make([]api.AnswerCoachingView, 0, len(review.Answers)),
	}
	points := func(from []evaluation.CoachingPoint) []api.CoachingPointView {
		out := make([]api.CoachingPointView, 0, len(from))
		for _, point := range from {
			out = append(out, api.CoachingPointView{Statement: point.Statement, Quote: point.Quote})
		}
		return out
	}
	for _, answer := range review.Answers {
		encoded := api.AnswerCoachingView{
			Sequence:  answer.Sequence,
			Strengths: points(answer.Strengths),
			Gaps:      points(answer.Gaps),
			Rewrite:   make([]api.RewritePartView, 0, len(answer.Rewrite)),
		}
		for _, part := range answer.Rewrite {
			encoded.Rewrite = append(encoded.Rewrite, api.RewritePartView{Kind: part.Kind, Text: part.Text})
		}
		view.Answers = append(view.Answers, encoded)
	}
	return view, nil
}

// Results answers the owner's stored evaluation. The session is confirmed
// theirs FIRST, so "not yours" and "not ready" never share an answer: a
// stranger learns nothing, the owner learns the honest state.
func (a interviewAdapter) Results(ctx context.Context, userID, sessionID string) (api.EvaluationResultView, error) {
	if _, err := a.sessions.Get(ctx, sessionID, "practice", userID, ""); err != nil {
		if errors.Is(err, interview.ErrNotFound) {
			return api.EvaluationResultView{}, api.ErrSessionMissing
		}
		return api.EvaluationResultView{}, err
	}

	result, err := a.results.ResultOf(ctx, evaluation.SessionRef{
		SessionID: sessionID, Mode: "practice", CandidateID: userID,
	})
	if errors.Is(err, evaluation.ErrNoResult) {
		return api.EvaluationResultView{}, api.ErrResultNotReady
	}
	if err != nil {
		return api.EvaluationResultView{}, err
	}

	ref := evaluation.SessionRef{SessionID: sessionID, Mode: "practice", CandidateID: userID}
	pairs, err := a.results.Contradictions(ctx, ref)
	if err != nil {
		return api.EvaluationResultView{}, err
	}
	spans, err := a.results.List(ctx, ref)
	if err != nil {
		return api.EvaluationResultView{}, err
	}

	view := api.EvaluationResultView{
		SessionID: result.SessionID,
		Rubric: api.RubricPinView{
			Reference: result.RubricReference,
			Version:   result.RubricVersion,
			Digest:    result.RubricDigest,
		},
		AggregationVersion:  result.AggregationVersion,
		ExtractionVersion:   result.ExtractionVersion,
		ModelVersion:        result.ModelVersion,
		PolicyVersion:       result.PolicyVersion,
		CoverageReached:     result.Aggregation.Coverage.Reached,
		CoverageNotReached:  result.Aggregation.Coverage.NotReached,
		CoveredCompetencies: result.Aggregation.CoveredCompetencies,
		TotalCompetencies:   result.Aggregation.TotalCompetencies,
		ResultDigest:        result.ResultDigest,
		Warnings:            result.Warnings,
		CreatedAt:           result.CreatedAt,
	}
	for _, span := range spans {
		view.Evidence = append(view.Evidence, api.EvidenceSpanView{
			ID: span.ID, CompetencyID: span.CompetencyID, Kind: span.Kind,
			Quote: span.Quote, SegmentSequence: span.SegmentSequence,
			StartMs: span.StartMs, EndMs: span.EndMs,
		})
	}
	for _, pair := range pairs {
		view.Contradictions = append(view.Contradictions, api.ContradictionView{
			Topic: pair.Topic,
			SideA: api.ContradictionSideView{
				SegmentSequence: pair.SideA.SegmentSequence, Quote: pair.SideA.Quote,
				StartMs: pair.SideA.StartMs, EndMs: pair.SideA.EndMs,
			},
			SideB: api.ContradictionSideView{
				SegmentSequence: pair.SideB.SegmentSequence, Quote: pair.SideB.Quote,
				StartMs: pair.SideB.StartMs, EndMs: pair.SideB.EndMs,
			},
		})
	}
	for _, competency := range result.Aggregation.Competencies {
		view.Competencies = append(view.Competencies, api.CompetencyResultView{
			CompetencyID:  competency.CompetencyID,
			Status:        competency.Status,
			Confidence:    competency.Confidence,
			Band:          competency.Band,
			EvidenceIDs:   competency.EvidenceIDs,
			EvidenceCount: competency.EvidenceCount,
			Supporting:    competency.Supporting,
			Contradictory: competency.Contradictory,
			Unverified:    competency.Unverified,
			Gaps:          competency.Gaps,
			ReasonCodes:   competency.ReasonCodes,
		})
	}
	return view, nil
}

// CompleteInterview seals the owner's practice session, translating each
// refusal onto the wire's stable codes.
func (a interviewAdapter) CompleteInterview(ctx context.Context, userID, sessionID string, epoch, finalSequence int) (api.CompletionReceiptView, error) {
	receipt, err := a.completer.Complete(ctx, sessionID, "practice", userID, "", epoch, finalSequence)
	switch {
	case errors.Is(err, interview.ErrNotFound):
		return api.CompletionReceiptView{}, api.ErrSessionMissing
	case errors.Is(err, interview.ErrCompleteNotRunning):
		return api.CompletionReceiptView{}, &api.StartRefusedError{Code: "SESSION_NOT_RUNNING",
			Message: "Only a running session can complete."}
	case errors.Is(err, interview.ErrSealConflict):
		return api.CompletionReceiptView{}, &api.StartRefusedError{Code: "SEAL_CONFLICT",
			Message: "This session already sealed at a different cursor."}
	case errors.Is(err, interview.ErrEpochStale):
		return api.CompletionReceiptView{}, &api.StartRefusedError{Code: "EPOCH_STALE",
			Message: "This connection was superseded by a newer one."}
	case err != nil:
		return api.CompletionReceiptView{}, err
	}

	view := api.CompletionReceiptView{
		SessionID: receipt.SessionID, State: string(receipt.State),
		SealedEpoch: receipt.SealedEpoch, SealedSequence: receipt.SealedSequence,
		TranscriptDigest: receipt.TranscriptDigest, BundleDigest: receipt.BundleDigest,
		MediaStatus: receipt.MediaStatus, Warnings: receipt.Warnings,
		SealedAt: receipt.SealedAt,
	}
	for _, gap := range receipt.Gaps {
		view.Gaps = append(view.Gaps, [2]int{gap.From, gap.To})
	}
	return view, nil
}

// IngestEvents accepts one control batch under the owner's practice scope.
func (a interviewAdapter) IngestEvents(ctx context.Context, userID, sessionID string, epoch int, events []api.ControlEventIn) (api.ControlAck, error) {
	batch := make([]interview.ControlEvent, 0, len(events))
	for _, event := range events {
		batch = append(batch, interview.ControlEvent{
			EventID: event.EventID, Epoch: epoch, Sequence: event.Sequence,
			Type: event.Type, Payload: event.Payload, OccurredAt: event.OccurredAt,
		})
	}
	ack, err := a.events.Ingest(ctx, sessionID, "practice", userID, "", epoch, batch)
	switch {
	case errors.Is(err, interview.ErrNotFound):
		return api.ControlAck{}, api.ErrSessionMissing
	case errors.Is(err, interview.ErrEpochStale):
		return api.ControlAck{}, &api.StartRefusedError{Code: "EPOCH_STALE",
			Message: "This connection was superseded by a newer one. Resume to continue; the newer connection owns the session."}
	case errors.Is(err, interview.ErrNoAttempt):
		return api.ControlAck{}, &api.StartRefusedError{Code: "NO_ATTEMPT",
			Message: "The session has no active connection attempt. Start it first."}
	case err != nil:
		return api.ControlAck{}, err
	}

	out := api.ControlAck{Epoch: ack.Epoch, Accepted: ack.Accepted}
	for _, gap := range ack.Missing {
		out.Missing = append(out.Missing, [2]int{gap.From, gap.To})
	}
	for _, outcome := range ack.Outcomes {
		out.Outcomes = append(out.Outcomes, api.ControlOutcome{
			EventID: outcome.EventID, Status: outcome.Status, Reason: outcome.Reason,
		})
	}
	return out, nil
}

// Transcript answers the assembled read model under the owner's scope.
func (a interviewAdapter) Transcript(ctx context.Context, userID, sessionID string) (api.TranscriptView, error) {
	transcript, err := a.events.AssembleTranscript(ctx, sessionID, "practice", userID, "")
	if errors.Is(err, interview.ErrNotFound) {
		return api.TranscriptView{}, api.ErrSessionMissing
	}
	if err != nil {
		return api.TranscriptView{}, err
	}
	view := api.TranscriptView{OrphanCorrections: transcript.OrphanCorrections}
	for _, segment := range transcript.Segments {
		encoded := api.TranscriptSegmentView{
			Epoch: segment.Epoch, Sequence: segment.Sequence, Type: segment.Type,
			Speaker: segment.Speaker, Text: segment.Text,
			StartMs: segment.StartMs, EndMs: segment.EndMs,
			Confidence: segment.Confidence, Superseded: segment.Superseded,
			CorrectedBySequence: segment.CorrectedBySequence, Supersedes: segment.Supersedes,
		}
		for _, word := range segment.Words {
			encoded.Words = append(encoded.Words, api.TranscriptWordView{
				Word: word.Word, StartMs: word.StartMs, EndMs: word.EndMs, Confidence: word.Confidence,
			})
		}
		view.Segments = append(view.Segments, encoded)
	}
	return view, nil
}

// ReplayEvents answers the durable timeline after a cursor.
func (a interviewAdapter) ReplayEvents(ctx context.Context, userID, sessionID string, afterEpoch, afterSequence int) ([]api.ControlEventOut, error) {
	replayed, err := a.events.Replay(ctx, sessionID, "practice", userID, "", afterEpoch, afterSequence)
	if errors.Is(err, interview.ErrNotFound) {
		return nil, api.ErrSessionMissing
	}
	if err != nil {
		return nil, err
	}
	out := make([]api.ControlEventOut, 0, len(replayed))
	for _, event := range replayed {
		out = append(out, api.ControlEventOut{
			EventID: event.EventID, Epoch: event.Epoch, Sequence: event.Sequence,
			Type: event.Type, Payload: event.Payload, OccurredAt: event.OccurredAt,
		})
	}
	return out, nil
}

// ledgerPort narrows billing to the port interview declares, translating
// the sentinels so interview never imports billing.
type ledgerPort struct {
	ledger *billing.Ledger
}

func (l ledgerPort) ReserveStart(ctx context.Context, tenantID, sessionID, mode string) error {
	err := l.ledger.ReserveStart(ctx, tenantID, sessionID, mode)
	switch {
	case errors.Is(err, billing.ErrQuotaExhausted):
		return interview.ErrStartQuotaExhausted
	case errors.Is(err, billing.ErrAlreadyMetered):
		return interview.ErrLedgerAlreadyMetered
	}
	return err
}

// grantsPort narrows the realtime signer to the port interview declares.
type grantsPort struct {
	grants *realtime.Grants
}

func (g grantsPort) MintJoin(room, identity string, ttl time.Duration) (interview.RoomGrant, error) {
	grant, err := g.grants.MintJoin(realtime.JoinRequest{Room: room, Identity: identity, TTL: ttl})
	if err != nil {
		return interview.RoomGrant{}, err
	}
	return interview.RoomGrant{
		URL: grant.URL, Room: grant.Room, Token: grant.Token, ExpiresAt: grant.ExpiresAt,
	}, nil
}

// StartPractice runs the start command for the owner's practice session and
// translates each distinct refusal onto the wire's stable codes.
func (a interviewAdapter) StartPractice(ctx context.Context, userID, sessionID string) (api.StartedInterview, error) {
	started, err := a.starter.Start(ctx, sessionID, "practice", userID, "")
	switch {
	case errors.Is(err, interview.ErrNotFound):
		return api.StartedInterview{
			Timing: api.TimingPolicyView{
				PolicyVersion:         started.Timing.Version,
				ReconnectGraceSeconds: started.Timing.ReconnectGraceSeconds,
				MaxOverrunSeconds:     started.Timing.MaxOverrunSeconds,
			}}, api.ErrSessionMissing
	case errors.Is(err, interview.ErrStartExpired):
		return api.StartedInterview{
				Timing: api.TimingPolicyView{
					PolicyVersion:         started.Timing.Version,
					ReconnectGraceSeconds: started.Timing.ReconnectGraceSeconds,
					MaxOverrunSeconds:     started.Timing.MaxOverrunSeconds,
				}}, &api.StartRefusedError{Code: "SESSION_EXPIRED",
				Message: "This session has expired. Set up a fresh interview; nothing you configured is lost."}
	case errors.Is(err, interview.ErrStartAlreadyStarted):
		return api.StartedInterview{
				Timing: api.TimingPolicyView{
					PolicyVersion:         started.Timing.Version,
					ReconnectGraceSeconds: started.Timing.ReconnectGraceSeconds,
					MaxOverrunSeconds:     started.Timing.MaxOverrunSeconds,
				}}, &api.StartRefusedError{Code: "SESSION_ALREADY_STARTED",
				Message: "This session has already started."}
	case errors.Is(err, interview.ErrStartNotReady):
		return api.StartedInterview{
				Timing: api.TimingPolicyView{
					PolicyVersion:         started.Timing.Version,
					ReconnectGraceSeconds: started.Timing.ReconnectGraceSeconds,
					MaxOverrunSeconds:     started.Timing.MaxOverrunSeconds,
				}}, &api.StartRefusedError{Code: "SESSION_NOT_READY",
				Message: "This session is not ready to start yet."}
	case errors.Is(err, interview.ErrStartQuotaExhausted):
		return api.StartedInterview{
				Timing: api.TimingPolicyView{
					PolicyVersion:         started.Timing.Version,
					ReconnectGraceSeconds: started.Timing.ReconnectGraceSeconds,
					MaxOverrunSeconds:     started.Timing.MaxOverrunSeconds,
				}}, &api.StartRefusedError{Code: "QUOTA_EXHAUSTED",
				Message: "This workspace is at capacity right now. The hiring team has been told; nothing you did caused this."}
	case err != nil:
		return api.StartedInterview{
			Timing: api.TimingPolicyView{
				PolicyVersion:         started.Timing.Version,
				ReconnectGraceSeconds: started.Timing.ReconnectGraceSeconds,
				MaxOverrunSeconds:     started.Timing.MaxOverrunSeconds,
			}}, err
	}

	session, err := a.GetPractice(ctx, userID, sessionID)
	if err != nil {
		return api.StartedInterview{
			Timing: api.TimingPolicyView{
				PolicyVersion:         started.Timing.Version,
				ReconnectGraceSeconds: started.Timing.ReconnectGraceSeconds,
				MaxOverrunSeconds:     started.Timing.MaxOverrunSeconds,
			}}, err
	}
	return api.StartedInterview{
		Timing: api.TimingPolicyView{
			PolicyVersion:         started.Timing.Version,
			ReconnectGraceSeconds: started.Timing.ReconnectGraceSeconds,
			MaxOverrunSeconds:     started.Timing.MaxOverrunSeconds,
		},
		Session: session,
		Realtime: api.RoomGrantView{
			URL:       started.Grant.URL,
			Room:      started.Grant.Room,
			Token:     started.Grant.Token,
			ExpiresAt: started.Grant.ExpiresAt,
		},
	}, nil
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

	view := api.InterviewSession{
		ID: session.ID, Mode: session.Mode, State: string(session.State),
		Config:              selection,
		RecordingPreference: session.RecordingPreference,
		ConsentVersion:      session.ConsentVersion,
		FailureCode:         session.FailureCode,
		ConnectionEpoch:     session.ConnectionEpoch,
		AcceptedSequence:    session.AcceptedSequence,
		CreatedAt:           session.CreatedAt,
	}
	// The durable receipt rides the session once sealed, so the complete
	// screen survives navigation: the same GET answers it forever.
	switch session.State {
	case interview.StateFinalizing, interview.StateEvaluating,
		interview.StateReviewReady, interview.StateEvaluationFailed, interview.StateArchived:
		receipt, err := a.completer.SealOf(ctx, sessionID, "practice", userID, "")
		if err == nil {
			view.Seal = &api.SealView{
				SealedAt:    receipt.SealedAt,
				MediaStatus: receipt.MediaStatus,
				Warnings:    receipt.Warnings,
			}
		}
	}
	return view, nil
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
