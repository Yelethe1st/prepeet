package evaluation_test

import (
	"errors"
	"os"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/evaluation"
)

// The model policy: which stages exist, which the result cannot do
// without, and what each may spend (ADR-0019). Parsed and cohered here so
// an incoherent policy never publishes and never reaches a session.

func policyFixture(t *testing.T) evaluation.Policy {
	t.Helper()
	policy, err := evaluation.ParsePolicy([]byte(`{"stages":[
		{"id":"evidence","required":true,"budget_units":100},
		{"id":"aggregation","required":true,"budget_units":20},
		{"id":"articulation","required":false,"budget_units":60}
	]}`))
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return policy
}

func TestAStageAnswersItsBudgetAndWhetherItIsRequired(t *testing.T) {
	policy := policyFixture(t)

	required, budget, known := policy.Stage("aggregation")
	if !known || !required || budget != 20 {
		t.Fatalf("aggregation = required %v budget %d known %v", required, budget, known)
	}
	optional, spend, known := policy.Stage("articulation")
	if !known || optional || spend != 60 {
		t.Fatalf("articulation = required %v budget %d", optional, spend)
	}

	// A stage the policy does not name is unknown rather than silently
	// unlimited: a stage nobody budgeted for must not spend by default.
	if _, _, known := policy.Stage("invented"); known {
		t.Fatal("an unnamed stage was treated as budgeted")
	}
}

func TestAnIncoherentPolicyIsRefusedAtParse(t *testing.T) {
	cases := map[string]string{
		"no stages at all":                 `{"stages":[]}`,
		"a stage with no name":             `{"stages":[{"id":"","required":true,"budget_units":10}]}`,
		"a negative budget":                `{"stages":[{"id":"evidence","required":true,"budget_units":-1}]}`,
		"a required stage that cannot run": `{"stages":[{"id":"evidence","required":true,"budget_units":0}]}`,
		"the same stage twice":             `{"stages":[{"id":"evidence","required":true,"budget_units":10},{"id":"evidence","required":false,"budget_units":5}]}`,
		"not a document at all":            `{"stages":`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := evaluation.ParsePolicy([]byte(body)); !errors.Is(err, evaluation.ErrPolicyIncoherent) {
				t.Fatalf("%s = %v, want ErrPolicyIncoherent", name, err)
			}
		})
	}
}

func TestTheShippedPolicyParses(t *testing.T) {
	// The artifact the registry publishes must be one the platform can
	// actually run against.
	body, err := os.ReadFile("../../../intelligence/artifacts/policy/practice-default@1.0.0.json")
	if err != nil {
		t.Fatalf("reading the artifact: %v", err)
	}
	var envelope struct {
		Type string `json:"type"`
		Body []byte `json:"-"`
	}
	if err := unmarshalEnvelope(body, &envelope.Type); err != nil {
		t.Fatalf("envelope: %v", err)
	}
	if envelope.Type != "model_policy" {
		t.Fatalf("artifact type = %q", envelope.Type)
	}
	policy, err := evaluation.ParsePolicy(shippedPolicyBody(t, body))
	if err != nil {
		t.Fatalf("the shipped policy does not parse: %v", err)
	}
	// Every stage the pipeline runs must be budgeted, or the pipeline
	// would meet an unknown stage at the moment it needed a number.
	for _, stage := range []string{"evidence", "aggregation", "articulation", "coaching"} {
		if _, _, known := policy.Stage(stage); !known {
			t.Fatalf("the shipped policy does not budget %q", stage)
		}
	}
}
