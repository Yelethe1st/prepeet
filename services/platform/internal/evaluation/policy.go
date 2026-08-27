package evaluation

// The model policy: EVL-07's half of ADR-0019.
//
// A policy names the stages the pipeline may run, says which the result
// cannot do without, and gives each a budget in the cost units the RPC
// contract already reports. It is a pinned artifact like the rubric, so
// what a session was allowed to spend is answerable forever from the
// session itself rather than from whatever is configured today.
//
// The rule the budget serves is ADR-0019's: exhaustion never degrades a
// required result silently. A required stage's budget is checked when it
// is set, not when it runs out - a required stage that could not run is a
// publication bug, refused at parse. An optional stage that cannot afford
// to run is omitted, and the omission is recorded and shown in words.

import (
	"encoding/json"
	"errors"
	"fmt"
)

// PolicyReference is the platform default every practice session pins.
const PolicyReference = "policy/practice-default"

// ErrPolicyIncoherent refuses a policy that cannot govern a pipeline.
var ErrPolicyIncoherent = errors.New("evaluation: the model policy document is incoherent")

// Policy is one parsed, coherent model policy.
type Policy struct {
	Stages []StagePolicy `json:"stages"`
}

// StagePolicy is one stage's standing.
type StagePolicy struct {
	ID string `json:"id"`
	// Required stages produce the result; optional stages add to it.
	Required bool `json:"required"`
	// BudgetUnits is what this stage may spend, in the cost units the
	// capability contract reports on every response.
	BudgetUnits int `json:"budget_units"`
}

// ParsePolicy decodes and coheres a policy document. The loader runs this
// as the artifact's validating step, so an incoherent policy never
// publishes.
func ParsePolicy(body []byte) (Policy, error) {
	var policy Policy
	if err := json.Unmarshal(body, &policy); err != nil {
		return Policy{}, fmt.Errorf("%w: %v", ErrPolicyIncoherent, err)
	}
	if len(policy.Stages) == 0 {
		return Policy{}, fmt.Errorf("%w: a policy with no stages governs nothing", ErrPolicyIncoherent)
	}
	seen := map[string]bool{}
	for _, stage := range policy.Stages {
		switch {
		case stage.ID == "":
			return Policy{}, fmt.Errorf("%w: a stage without a name", ErrPolicyIncoherent)
		case seen[stage.ID]:
			return Policy{}, fmt.Errorf("%w: stage %q appears twice", ErrPolicyIncoherent, stage.ID)
		case stage.BudgetUnits < 0:
			return Policy{}, fmt.Errorf("%w: stage %q has a negative budget", ErrPolicyIncoherent, stage.ID)
		case stage.Required && stage.BudgetUnits == 0:
			// A required stage with nothing to spend could never run, and
			// the result it is required for could never be produced.
			return Policy{}, fmt.Errorf(
				"%w: stage %q is required and budgeted nothing", ErrPolicyIncoherent, stage.ID)
		}
		seen[stage.ID] = true
	}
	return policy, nil
}

// Stage answers one stage's standing: whether it is required, what it may
// spend, and whether the policy names it at all. An unnamed stage is
// unknown rather than unlimited - a stage nobody budgeted for must not
// spend by default.
func (p Policy) Stage(id string) (required bool, budgetUnits int, known bool) {
	for _, stage := range p.Stages {
		if stage.ID == id {
			return stage.Required, stage.BudgetUnits, true
		}
	}
	return false, 0, false
}

// PolicyPin is the policy exactly as a session's bundle pinned it.
type PolicyPin struct {
	Reference string
	Version   string
	Digest    string
	Body      json.RawMessage
}
