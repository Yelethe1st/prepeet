package progression

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Personal interview requirements: PRG-06's pure half.
//
// A candidate says what they want a session to test, in their own words.
// Three things have to be true of whatever happens next, and each is held
// by a shape here rather than by care at the call site.
//
// The system must not agree to infer something it has no business
// inferring. Resolve refuses a protected characteristic outright and
// reframes a request whose observable intent is recoverable, saying so;
// there is no path from an intent to a criterion that does not go through
// it, and no way to spell a criterion about a personality.
//
// A result must be reported against the exact criteria that were pinned.
// Criteria are immutable per version and an edit is a new version, so an
// outcome already given keeps meaning what it meant.
//
// And a session that never gave a fair chance to demonstrate something
// reports not assessable, never a zero. Score returns nothing for every
// outcome that is not a real reading, so a metric cannot average silence
// into a decline.

// ErrProhibitedInference refuses a requirement that asks the system to
// infer personality, emotion, accent, confidence or another trait it must
// not judge.
var ErrProhibitedInference = errors.New("progression: this cannot be observed and will not be inferred")

// ErrRequirementIncoherent refuses a requirement that cannot be tracked.
var ErrRequirementIncoherent = errors.New("progression: the requirement is incoherent")

// A personal requirement's lifecycle. Draft exists because resolution can
// need a conversation: a request the system could not read is a draft the
// candidate can rewrite, not a rejection they have to start over from.
const (
	RequirementDraft   = "draft"
	RequirementActive  = "active"
	RequirementPaused  = "paused"
	RequirementRetired = "retired"
)

// What resolving a candidate's request came to.
//
// Four outcomes rather than accept-or-reject, because the three refusals
// are different conversations: one cannot be observed at all, one must not
// be inferred and has no honest substitute, and one has an observable
// behaviour underneath it that the candidate probably meant.
const (
	ResolutionAccepted = "accepted"
	ResolutionReframed = "reframed"
	ResolutionRefused  = "refused"
	ResolutionUnclear  = "unclear"
)

// What a session can say about a requirement.
//
// NotAssessable is the one the ticket exists to protect. It means the
// session never created a fair opportunity, which is a fact about the
// session and not about the candidate, and it must never be converted into
// a low result.
const (
	RequirementAchieved          = "achieved"
	RequirementPartiallyAchieved = "partially_achieved"
	RequirementNotDemonstrated   = "not_demonstrated"
	RequirementNotAssessable     = "not_assessable"
)

// Why a requirement could not be assessed.
const (
	NoFairOpportunity = "no_fair_opportunity"
	NoEvidenceOffered = "no_evidence_offered"
)

// Whether a metric rests on enough sessions to mean anything.
const (
	EvidenceSufficient   = "sufficient"
	EvidenceInsufficient = "insufficient"
)

// sufficientSessions is the point at which a personal-requirement metric
// stops being an anecdote.
//
// Three, and deliberately not two: two sessions produce a line, and a line
// through two points is the shape most easily mistaken for a trend. Below
// this the metric is still shown, with its sufficiency stated, because
// hiding it would leave the candidate with nothing at exactly the moment
// they are most interested.
const sufficientSessions = 3

// Criterion is one observable thing a session can look for.
//
// Statement is the candidate's copy and Observable is what an evaluator
// looks for. Both are stored: a criterion the candidate cannot read is one
// they cannot disagree with, and the ticket requires them to be visible.
type Criterion struct {
	ID         string `json:"id"`
	Statement  string `json:"statement"`
	Observable string `json:"observable"`
}

// Resolution is what the system made of a candidate's request.
type Resolution struct {
	Outcome  string
	Criteria []Criterion

	// Explanation is what the candidate reads. It is required for every
	// outcome that is not a plain acceptance, because a refusal nobody can
	// act on is indistinguishable from a bug.
	Explanation string

	// Prohibited names the inference that was declined, when one was.
	Prohibited string
}

// PersonalRequirement is one thing a candidate asked a session to test.
type PersonalRequirement struct {
	ID     string
	Intent string
	Status string

	// Version rises with every revision and never falls. It is what an
	// outcome is reported against, so a result given in March still means
	// what it meant after an edit in June.
	Version  int
	Criteria []Criterion

	// Reframing and Prohibited are set when the request asked for something
	// the system will not infer and an observable behaviour was offered
	// instead. Carried on the requirement rather than shown once at
	// creation, because a reframing the candidate cannot go back and read
	// is a substitution: they would believe the session is looking for what
	// they asked for.
	Reframing  string
	Prohibited string

	CreatedAt time.Time
}

// criterionTemplate is one observable behaviour the product knows how to
// look for, and the words that ask for it.
//
// A catalogue rather than free interpretation. The alternative is a model
// deciding what a sentence meant, which would make the pinned criteria
// unreproducible and put an unreviewable judgement between a candidate and
// what they get measured on. An intent this catalogue cannot read comes
// back as a request for clarification, which is honest, rather than as
// invented criteria, which is not.
type criterionTemplate struct {
	id         string
	statement  string
	observable string
	cues       []string
}

var criterionCatalogue = []criterionTemplate{
	{
		id:         "greeting",
		statement:  "Open with a greeting that acknowledges the interviewer.",
		observable: "The first turn contains a greeting addressed to the interviewer.",
		cues:       []string{"greet", "greeting", "hello", "open the interview", "introduce myself to"},
	},
	{
		id:         "introduction",
		statement:  "Give a short introduction that names your role and focus.",
		observable: "An early turn states the candidate's current role and area of work in under a minute.",
		cues:       []string{"introduction", "introduce", "about myself", "elevator", "summary of me"},
	},
	{
		id:         "answer-structure",
		statement:  "Lead each answer with the point, then the supporting detail.",
		observable: "Answers state a conclusion before the detail that supports it.",
		cues:       []string{"structure", "structured", "star", "headline", "get to the point", "rambl", "waffle", "hedg"},
	},
	{
		id:         "trade-offs",
		statement:  "Name the trade-offs behind a technical choice.",
		observable: "A technical answer names at least one alternative and why it was not chosen.",
		cues:       []string{"trade-off", "tradeoff", "trade off", "technical decision", "why i chose", "alternatives"},
	},
	{
		id:         "evidence",
		statement:  "Support claims with a concrete example.",
		observable: "A claim about past work is followed by a specific example with a result.",
		cues:       []string{"example", "evidence", "concrete", "specific", "back up"},
	},
	{
		id:         "questions",
		statement:  "Ask the interviewer at least one relevant question.",
		observable: "The candidate asks a question about the role, team or work.",
		cues:       []string{"question", "ask them", "ask the interviewer", "curious"},
	},
	{
		id:         "closing",
		statement:  "Close with a short summary and a clear next step.",
		observable: "The final turn summarises and states interest or a next step.",
		cues:       []string{"clos", "wrap up", "end the interview", "finish", "sign off"},
	},
	{
		id:         "conciseness",
		statement:  "Keep answers to the length the question asked for.",
		observable: "Answer length stays within the range the pinned policy considers concise.",
		cues:       []string{"concise", "brief", "shorter", "too long", "conciseness", "succinct"},
	},
}

// prohibition is one inference the system will not make, and what to do
// about a request for it.
//
// reframeTo names an observable criterion when the request has a
// recoverable intent underneath it, and is empty when it does not. That
// emptiness is the important half: reframing a request about somebody's
// accent would imply the request was reasonable and merely badly worded,
// and it was not.
type prohibition struct {
	name      string
	cues      []string
	reframeTo string
	because   string
}

var prohibitions = []prohibition{
	{
		name: "accent or speech background",
		cues: []string{"accent", "sound native", "sound american", "sound british",
			"neutral accent", "how i pronounce", "my english sounds"},
		because: "How somebody speaks is not something this system judges, and there is " +
			"no version of the question it can answer instead.",
	},
	{
		name: "personality",
		cues: []string{"personality", "what kind of person", "my character", "am i likeable",
			"likeable", "likable", "charisma", "charismatic"},
		because: "Personality is not observable from an interview answer, and inferring one " +
			"from a recording is not something this system will do.",
	},
	{
		name: "emotional state",
		cues: []string{"anxious", "nervous", "nervousness", "stressed", "calm", "my mood",
			"emotion", "how i feel", "sound relaxed"},
		reframeTo: "answer-structure",
		because: "Emotional state is not inferred from voice or delivery. What can be " +
			"observed is the shape of an answer, so that is what this requirement asks for.",
	},
	{
		name: "confidence",
		cues: []string{"confident", "confidence", "self-assured", "sound sure of myself",
			"assertive", "sound authoritative"},
		reframeTo: "answer-structure",
		because: "Confidence is not inferred from how somebody sounds. It is collected only " +
			"as your own optional self-rating. What can be observed is whether an answer " +
			"leads with its point, so that is what this requirement asks for.",
	},
	{
		name:      "enthusiasm",
		cues:      []string{"enthusiastic", "enthusiasm", "passionate", "passion", "excited", "energy level"},
		reframeTo: "questions",
		because: "Enthusiasm is a feeling, not a behaviour a transcript records. What can be " +
			"observed is whether you asked about the role and the work, so that is what this " +
			"requirement asks for.",
	},
	{
		name: "appearance, age, health or another protected characteristic",
		cues: []string{"how i look", "my age", "look older", "look younger", "attractive",
			"my disability", "my accent is foreign", "sound foreign", "mental health", "neurodiver"},
		because: "This is a characteristic the system must never judge, and no interview " +
			"result will ever be based on one.",
	},
}

// Resolve turns a candidate's request into criteria, a reframing, or a
// refusal that says why.
//
// Prohibitions are checked before the catalogue and always. A request that
// contains both an observable behaviour and a prohibited inference is not
// half accepted: the prohibited half decides the outcome, because a
// requirement that quietly assessed the acceptable part while appearing to
// answer the whole request would be the worst of the available answers.
func Resolve(intent string) Resolution {
	normalised := strings.ToLower(strings.TrimSpace(intent))
	if normalised == "" {
		return Resolution{
			Outcome:     ResolutionUnclear,
			Explanation: "Tell us what you would like this session to look for.",
		}
	}

	for _, rule := range prohibitions {
		if !mentions(normalised, rule.cues) {
			continue
		}
		resolution := Resolution{Prohibited: rule.name, Explanation: rule.because}
		if rule.reframeTo == "" {
			resolution.Outcome = ResolutionRefused
			return resolution
		}
		template, found := templateByID(rule.reframeTo)
		if !found {
			resolution.Outcome = ResolutionRefused
			return resolution
		}
		resolution.Outcome = ResolutionReframed
		resolution.Criteria = []Criterion{criterionOf(template)}
		return resolution
	}

	matched := make([]Criterion, 0, len(criterionCatalogue))
	for _, template := range criterionCatalogue {
		if mentions(normalised, template.cues) {
			matched = append(matched, criterionOf(template))
		}
	}
	if len(matched) == 0 {
		return Resolution{
			Outcome: ResolutionUnclear,
			Explanation: "We could not turn that into something a session can observe. " +
				"Try naming a moment or a behaviour, such as the greeting, the introduction, " +
				"how an answer is structured, the trade-offs behind a decision, the questions " +
				"you ask, or how you close.",
		}
	}
	return Resolution{Outcome: ResolutionAccepted, Criteria: matched}
}

// mentions reports whether any cue appears in the request.
func mentions(normalised string, cues []string) bool {
	for _, cue := range cues {
		if strings.Contains(normalised, cue) {
			return true
		}
	}
	return false
}

func templateByID(id string) (criterionTemplate, bool) {
	for _, template := range criterionCatalogue {
		if template.id == id {
			return template, true
		}
	}
	return criterionTemplate{}, false
}

func criterionOf(template criterionTemplate) Criterion {
	return Criterion{ID: template.id, Statement: template.statement, Observable: template.observable}
}

// NewRequirement resolves a candidate's request into version 1 of a
// requirement, or refuses it.
//
// A refused or unclear request produces no requirement at all rather than
// an empty one, because a requirement with no criteria would be selectable
// for a session it could never answer.
func NewRequirement(id, intent string) (PersonalRequirement, error) {
	resolution := Resolve(intent)
	switch resolution.Outcome {
	case ResolutionRefused:
		return PersonalRequirement{}, fmt.Errorf("%w: %s (%s)",
			ErrProhibitedInference, resolution.Explanation, resolution.Prohibited)
	case ResolutionUnclear:
		return PersonalRequirement{}, fmt.Errorf("%w: %s",
			ErrRequirementIncoherent, resolution.Explanation)
	}
	requirement := PersonalRequirement{
		ID: id, Intent: intent, Status: RequirementDraft,
		Version: 1, Criteria: resolution.Criteria,
		Prohibited: resolution.Prohibited,
	}
	if resolution.Outcome == ResolutionReframed {
		requirement.Reframing = resolution.Explanation
	}
	return requirement, nil
}

// MoveTo changes a requirement's lifecycle state.
//
// Retiring is final, for the same reason retiring a goal is: a state
// somebody chose to leave and can silently re-enter is not a decision, and
// the outcomes already recorded against a retired requirement should not
// start accruing again without the candidate saying so afresh.
func (r *PersonalRequirement) MoveTo(status string) error {
	switch status {
	case RequirementDraft, RequirementActive, RequirementPaused, RequirementRetired:
	default:
		return fmt.Errorf("%w: %q is not a requirement lifecycle state",
			ErrRequirementIncoherent, status)
	}
	if r.Status == RequirementRetired && status != RequirementRetired {
		return fmt.Errorf("%w: a retired requirement stays retired: write a new one",
			ErrRequirementIncoherent)
	}
	r.Status = status
	return nil
}

// Revise produces the next version of a requirement from a new intent.
//
// A new value rather than a mutation, and the receiver is left alone.
// Criteria are what outcomes are reported against, so editing them in
// place would rewrite the meaning of every result already given; the old
// version stays readable and the new one starts being pinned from now on.
func (r PersonalRequirement) Revise(intent string) (PersonalRequirement, error) {
	revised, err := NewRequirement(r.ID, intent)
	if err != nil {
		return PersonalRequirement{}, err
	}
	revised.Version = r.Version + 1
	revised.Status = r.Status
	revised.CreatedAt = r.CreatedAt
	return revised, nil
}

// CriterionFinding is what one session found about one criterion.
//
// Demonstrated is a fact about the transcript, and Evidence names where.
// A finding with no evidence and Demonstrated true is refused by Judge
// rather than trusted, because an unevidenced achievement is the exact
// claim this product exists not to make.
type CriterionFinding struct {
	CriterionID  string
	Demonstrated bool
	Evidence     []string
}

// RequirementOutcome is one session's answer about one requirement.
type RequirementOutcome struct {
	RequirementID    string
	CriterionVersion int
	SessionID        string

	// The comparison basis. Two sessions for different roles or in
	// different shapes are not two readings of one thing, and carrying the
	// basis on the row is what lets Measure keep them apart without
	// anybody remembering to.
	RoleID  string
	ShapeID string

	Outcome string

	// Reason is set exactly when the outcome is not assessable.
	Reason string

	Demonstrated []string
	Missing      []string
	Evidence     []string
	NextActions  []string

	ObservedAt time.Time
}

// Score is the outcome as a number, or nothing.
//
// Nothing for not assessable, deliberately and returned as a nil pointer
// rather than a zero, so that a caller computing an average has to decide
// what to do about the absence instead of silently averaging a session
// that never asked the question into a decline.
func (o RequirementOutcome) Score() *float64 {
	var value float64
	switch o.Outcome {
	case RequirementAchieved:
		value = 1
	case RequirementPartiallyAchieved:
		value = 0.5
	case RequirementNotDemonstrated:
		value = 0
	default:
		return nil
	}
	return &value
}

// Judge answers what one session says about one requirement.
//
// opportunity is the session's own statement about whether it ever created
// a fair chance to demonstrate this. It is a separate argument rather than
// something inferred from the findings because "we asked and they did not"
// and "we never asked" produce identical findings and opposite outcomes,
// and a function that had to guess between them would eventually guess
// wrong in the direction that hurts a candidate.
func Judge(requirement PersonalRequirement, findings []CriterionFinding,
	opportunity bool, at time.Time) RequirementOutcome {

	outcome := RequirementOutcome{
		RequirementID:    requirement.ID,
		CriterionVersion: requirement.Version,
		ObservedAt:       at,
	}
	if !opportunity {
		outcome.Outcome = RequirementNotAssessable
		outcome.Reason = NoFairOpportunity
		outcome.NextActions = []string{
			"This session never reached a point where " + strings.ToLower(requirement.Intent) +
				" would have fitted. Try a shape or a duration that makes room for it.",
		}
		return outcome
	}
	if len(findings) == 0 {
		outcome.Outcome = RequirementNotAssessable
		outcome.Reason = NoEvidenceOffered
		return outcome
	}

	byCriterion := make(map[string]CriterionFinding, len(findings))
	for _, finding := range findings {
		byCriterion[finding.CriterionID] = finding
	}
	for _, criterion := range requirement.Criteria {
		finding, found := byCriterion[criterion.ID]
		if found && finding.Demonstrated && len(finding.Evidence) > 0 {
			outcome.Demonstrated = append(outcome.Demonstrated, criterion.ID)
			outcome.Evidence = append(outcome.Evidence, finding.Evidence...)
			continue
		}
		outcome.Missing = append(outcome.Missing, criterion.ID)
		outcome.NextActions = append(outcome.NextActions, nextAction(criterion))
	}

	switch {
	case len(outcome.Missing) == 0:
		outcome.Outcome = RequirementAchieved
	case len(outcome.Demonstrated) == 0:
		outcome.Outcome = RequirementNotDemonstrated
	default:
		outcome.Outcome = RequirementPartiallyAchieved
	}
	// Two actions at most. A list of six is a list nobody acts on, and the
	// product requirement is one or two concrete things.
	if len(outcome.NextActions) > 2 {
		outcome.NextActions = outcome.NextActions[:2]
	}
	return outcome
}

// nextAction turns a missing criterion into something to do about it.
func nextAction(criterion Criterion) string {
	return "Next time: " + strings.TrimSuffix(criterion.Statement, ".") + "."
}

// Metric is one behaviourally anchored measure over comparable sessions.
//
// Every field the ticket asks a number to carry is here rather than left
// to a screen: what it means, how many sessions it rests on, whether that
// is enough, which criterion version, and what makes those sessions
// comparable. A Metric that could be rendered without its basis would
// eventually be rendered without it.
type Metric struct {
	RequirementID    string
	CriterionVersion int

	Definition      string
	ComparisonBasis string

	// Assessed counts the sessions that produced a real reading.
	// NotAssessable counts the rest and is reported beside them, never
	// folded in: a session that could not ask is not a session that went
	// badly.
	Assessed      int
	NotAssessable int

	Achieved         int
	PartiallyReached int
	NotDemonstrated  int

	Sufficiency string
	Rate        float64

	FirstObserved time.Time
	LastObserved  time.Time
	Evidence      Evidence
}

// Measure turns outcomes into one metric per comparable set.
//
// The key is requirement, criterion version, role and shape together, and
// every part of it earns its place: a different requirement is a different
// subject, a different criterion version measures a different thing, and a
// different role or shape is a different situation. Incompatible sessions
// come back as separate series rather than as one number, which is the
// whole of "must not average incomparable formats" made structural.
func Measure(outcomes []RequirementOutcome) []Metric {
	type key struct {
		requirement string
		version     int
		role        string
		shape       string
	}
	grouped := make(map[key]*Metric)
	order := make([]key, 0, len(outcomes))

	for _, outcome := range outcomes {
		k := key{outcome.RequirementID, outcome.CriterionVersion, outcome.RoleID, outcome.ShapeID}
		metric, seen := grouped[k]
		if !seen {
			metric = &Metric{
				RequirementID:    outcome.RequirementID,
				CriterionVersion: outcome.CriterionVersion,
				Definition: "The share of comparable sessions in which every criterion of " +
					"this requirement was demonstrated, counting a partial result as half.",
				ComparisonBasis: fmt.Sprintf(
					"role %s, shape %s, criterion version %d", outcome.RoleID, outcome.ShapeID,
					outcome.CriterionVersion),
			}
			grouped[k] = metric
			order = append(order, k)
		}
		if outcome.ObservedAt.After(metric.LastObserved) {
			metric.LastObserved = outcome.ObservedAt
		}
		if metric.FirstObserved.IsZero() || outcome.ObservedAt.Before(metric.FirstObserved) {
			metric.FirstObserved = outcome.ObservedAt
		}
		switch outcome.Outcome {
		case RequirementAchieved:
			metric.Achieved++
		case RequirementPartiallyAchieved:
			metric.PartiallyReached++
		case RequirementNotDemonstrated:
			metric.NotDemonstrated++
		default:
			metric.NotAssessable++
			continue
		}
		metric.Assessed++
	}

	metrics := make([]Metric, 0, len(order))
	for _, k := range order {
		metric := grouped[k]
		if metric.Assessed > 0 {
			metric.Rate = (float64(metric.Achieved) + 0.5*float64(metric.PartiallyReached)) /
				float64(metric.Assessed)
		}
		metric.Sufficiency = EvidenceInsufficient
		if metric.Assessed >= sufficientSessions {
			metric.Sufficiency = EvidenceSufficient
		}
		metric.Evidence = Freshness(metric.LastObserved, metric.LastObserved)
		metrics = append(metrics, *metric)
	}
	sort.SliceStable(metrics, func(i, j int) bool {
		if metrics[i].RequirementID != metrics[j].RequirementID {
			return metrics[i].RequirementID < metrics[j].RequirementID
		}
		return metrics[i].ComparisonBasis < metrics[j].ComparisonBasis
	})
	return metrics
}

// When a candidate rated themselves.
const (
	SelfReportBefore = "before"
	SelfReportAfter  = "after"
)

// SelfReport is a candidate's own confidence rating.
//
// A distinct type, stored in a distinct table, and never an Observation.
// Confidence is collected only as a self-rating and is never inferred from
// delivery or media, and the way to keep that true under maintenance is to
// give it no shared shape with anything the system judges: nothing here
// carries a rubric, a band or an evidence reference, so it cannot be
// mistaken for a reading or joined into one.
type SelfReport struct {
	SessionID  string
	Phase      string
	Rating     int
	ReportedAt time.Time
}

// NewSelfReport records one self-rating on a one-to-five scale.
func NewSelfReport(sessionID, phase string, rating int, at time.Time) (SelfReport, error) {
	if sessionID == "" {
		return SelfReport{}, fmt.Errorf("%w: a self-rating belongs to a session", ErrRequirementIncoherent)
	}
	if phase != SelfReportBefore && phase != SelfReportAfter {
		return SelfReport{}, fmt.Errorf("%w: %q is neither before nor after", ErrRequirementIncoherent, phase)
	}
	if rating < 1 || rating > 5 {
		return SelfReport{}, fmt.Errorf("%w: %d is off the one-to-five scale",
			ErrRequirementIncoherent, rating)
	}
	return SelfReport{SessionID: sessionID, Phase: phase, Rating: rating, ReportedAt: at}, nil
}
