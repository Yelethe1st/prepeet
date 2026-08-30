package recruiting

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Errors a caller distinguishes because each means a different next step.
var (
	// ErrConsentRefused means a required purpose was declined. The candidate
	// answered; the answer was no.
	ErrConsentRefused = errors.New("recruiting: a required consent was refused")
	// ErrConsentMissing means a required purpose was never answered.
	// Deliberately distinct from refusal: an unanswered question is a screen
	// the candidate has not finished, and treating it as either a yes or a no
	// would be inventing an answer they did not give.
	ErrConsentMissing = errors.New("recruiting: a required consent was not answered")
	// ErrPurposeCannotBeRequired means a campaign tried to make optional
	// processing a condition of the interview.
	ErrPurposeCannotBeRequired = errors.New("recruiting: this purpose can never be required")
	// ErrDisclosureIncomplete means the candidate would not be told something
	// they must be told.
	ErrDisclosureIncomplete = errors.New("recruiting: the disclosure does not cover every required area")
	// ErrNotADigest means a digest column was handed something that is not one.
	ErrNotADigest = errors.New("recruiting: a digest is required, not a reference")
)

// neverRequired are purposes that may be offered and may never be a condition
// of taking an interview.
//
// responsible-hiring.md puts it as consent being "not bundled with optional
// model improvement", and bundling is exactly what marking it required would
// be, whatever the screen looked like. Keeping the list here rather than in a
// review checklist means a campaign cannot do it by passing the wrong flag.
var neverRequired = map[string]bool{
	"model_improvement": true,
}

// requiredDisclosureAreas is what a candidate must be told before screening.
//
// The list is screen-mode.md's and responsible-hiring.md's, which agree: the
// employer, why, that AI is involved, what is recorded, who sees it, how long
// it is kept, how to exercise their rights, what accommodations exist, what
// they will be told of the result, and that a human owns the decision.
//
// It is a list here rather than prose in a template because a disclosure that
// silently omits one is not a shorter disclosure, it is an incomplete one, and
// the omission should stop it before a candidate sees it rather than being
// noticed in a review.
var requiredDisclosureAreas = []string{
	"employer",
	"purpose",
	"ai_involvement",
	"recording",
	"access",
	"retention",
	"rights_route",
	"accommodations",
	"result_disclosure",
	"human_decision_ownership",
}

// RequiredDisclosureAreas returns the areas a disclosure must cover.
func RequiredDisclosureAreas() []string {
	out := make([]string, len(requiredDisclosureAreas))
	copy(out, requiredDisclosureAreas)
	return out
}

// Purpose is one kind of processing a campaign asks about.
type Purpose struct {
	Name string
	// Required says whether the interview can happen without it.
	Required bool
}

// ConsentDecision is one candidate's answer about one purpose.
type ConsentDecision struct {
	Purpose  string
	Required bool
	Granted  bool
}

// Acceptance is a candidate agreeing to a specific disclosure version.
type Acceptance struct {
	TenantID          string
	CampaignID        string
	CandidateID       string
	DisclosureDigest  string
	DisclosureVersion string
	AcceptedAt        time.Time
}

// AcceptanceRequest is what a caller supplies to record one.
type AcceptanceRequest struct {
	TenantID          string
	CampaignID        string
	CandidateID       string
	DisclosureDigest  string
	DisclosureVersion string
}

// digestPattern is content.DigestOf's output shape.
var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// ValidatePurposes refuses a set of purposes a campaign may not ask for.
func ValidatePurposes(purposes []Purpose) error {
	for _, purpose := range purposes {
		if purpose.Required && neverRequired[purpose.Name] {
			return fmt.Errorf("%w: %s", ErrPurposeCannotBeRequired, purpose.Name)
		}
	}
	return nil
}

// ValidateDisclosure refuses a disclosure that leaves a required area uncovered.
func ValidateDisclosure(sections map[string]string) error {
	for _, area := range requiredDisclosureAreas {
		// Blank counts as absent. A key present with nothing behind it passes a
		// "has every key" check and tells the candidate nothing, which is how
		// this guard would most plausibly be defeated by accident.
		if strings.TrimSpace(sections[area]) == "" {
			return fmt.Errorf("%w: %s", ErrDisclosureIncomplete, area)
		}
	}
	return nil
}

// MayProceed reports whether the candidate's decisions permit the interview.
//
// The rule is only about required purposes. An optional purpose may be granted,
// refused, or never answered, and the interview is unaffected either way: that
// is what unbundling means, and stating it as a function rather than as a
// convention is what stops a later caller from tightening it by accident.
func MayProceed(purposes []Purpose, decisions []ConsentDecision) error {
	answered := make(map[string]ConsentDecision, len(decisions))
	for _, decision := range decisions {
		answered[decision.Purpose] = decision
	}

	for _, purpose := range purposes {
		if !purpose.Required {
			continue
		}
		decision, found := answered[purpose.Name]
		if !found {
			// Absence is its own outcome. Reading it as agreement is the
			// failure this separation exists to prevent.
			return fmt.Errorf("%w: %s", ErrConsentMissing, purpose.Name)
		}
		if !decision.Granted {
			return fmt.Errorf("%w: %s", ErrConsentRefused, purpose.Name)
		}
	}
	return nil
}

// NewAcceptance builds the record of a candidate accepting a disclosure.
//
// It refuses an acceptance that cannot say what was accepted, because storing
// one would leave behind a record that looks like consent and evidences
// nothing.
func NewAcceptance(request AcceptanceRequest) (Acceptance, error) {
	if strings.TrimSpace(request.DisclosureVersion) == "" {
		return Acceptance{}, errors.New("recruiting: an acceptance must name the disclosure version")
	}
	if !digestPattern.MatchString(request.DisclosureDigest) {
		return Acceptance{}, fmt.Errorf("%w: %q", ErrNotADigest, request.DisclosureDigest)
	}
	for name, value := range map[string]string{
		"tenant": request.TenantID, "campaign": request.CampaignID, "candidate": request.CandidateID,
	} {
		if strings.TrimSpace(value) == "" {
			return Acceptance{}, fmt.Errorf("recruiting: an acceptance must name its %s", name)
		}
	}

	return Acceptance{
		TenantID:          request.TenantID,
		CampaignID:        request.CampaignID,
		CandidateID:       request.CandidateID,
		DisclosureDigest:  request.DisclosureDigest,
		DisclosureVersion: request.DisclosureVersion,
		AcceptedAt:        time.Now().UTC(),
	}, nil
}
