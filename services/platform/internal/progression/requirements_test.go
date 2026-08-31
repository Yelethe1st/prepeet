package progression_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/internal/progression"
)

// PRG-06's pure half: what a candidate may ask of a session, what the
// system refuses to be asked, and how an answer is reported.
//
// The two failures this file is written to catch are opposites. One is a
// requirement that quietly becomes an inference about who somebody is. The
// other is a session that never gave a fair chance to show something and
// reports it as a failure anyway.

func TestAnObservableRequestResolvesToVersionedCriteria(t *testing.T) {
	t.Parallel()
	resolution := progression.Resolve("I want to give a clear greeting and a concise introduction")
	if resolution.Outcome != progression.ResolutionAccepted {
		t.Fatalf("outcome = %q (%s), want accepted", resolution.Outcome, resolution.Explanation)
	}
	if len(resolution.Criteria) < 2 {
		t.Fatalf("criteria = %+v, want one for the greeting and one for the introduction",
			resolution.Criteria)
	}
	for _, criterion := range resolution.Criteria {
		if criterion.ID == "" || criterion.Statement == "" {
			t.Errorf("a criterion the candidate cannot read: %+v", criterion)
		}
	}
}

func TestTheCandidatesOwnWordsAreKeptBesideTheCriteria(t *testing.T) {
	t.Parallel()
	// The resolution is the system's reading of a request, and a candidate
	// has to be able to see whether it read them right.
	requirement, err := progression.NewRequirement("req-1", "I want to close the interview clearly")
	if err != nil {
		t.Fatalf("new requirement: %v", err)
	}
	if requirement.Intent != "I want to close the interview clearly" {
		t.Errorf("intent = %q", requirement.Intent)
	}
	if requirement.Version != 1 {
		t.Errorf("version = %d, want 1", requirement.Version)
	}
}

func TestAnUnobservableRequestAsksForClarificationRatherThanInventingCriteria(t *testing.T) {
	t.Parallel()
	resolution := progression.Resolve("make me better")
	if resolution.Outcome != progression.ResolutionUnclear {
		t.Fatalf("outcome = %q, want unclear", resolution.Outcome)
	}
	if len(resolution.Criteria) != 0 {
		t.Errorf("criteria were invented for a request nobody could observe: %+v", resolution.Criteria)
	}
	if resolution.Explanation == "" {
		t.Error("a request was refused with no explanation the candidate can act on")
	}
}

func TestARequestForAProhibitedInferenceIsRefused(t *testing.T) {
	t.Parallel()
	for _, request := range []string{
		"tell me if my accent sounds professional",
		"assess my personality",
		"rate how likeable I seem",
		"say whether I sound anxious",
	} {
		t.Run(request, func(t *testing.T) {
			t.Parallel()
			resolution := progression.Resolve(request)
			if resolution.Outcome == progression.ResolutionAccepted {
				t.Fatalf("accepted a prohibited inference: %+v", resolution.Criteria)
			}
			if resolution.Prohibited == "" {
				t.Error("the refusal does not name what it will not infer")
			}
		})
	}
}

func TestAProhibitedRequestWithAnObservableIntentIsReframedNotSilentlyAccepted(t *testing.T) {
	t.Parallel()
	// "Sound more confident" is a real thing somebody wants and an
	// inference the system must not make. The reframing offers the
	// observable behaviour instead, and says plainly that confidence
	// itself is not being assessed.
	resolution := progression.Resolve("I want to sound more confident")
	if resolution.Outcome != progression.ResolutionReframed {
		t.Fatalf("outcome = %q, want reframed", resolution.Outcome)
	}
	if len(resolution.Criteria) == 0 {
		t.Fatal("a reframing with no criteria is a refusal wearing a friendlier word")
	}
	if !strings.Contains(strings.ToLower(resolution.Explanation), "confidence") {
		t.Errorf("the reframing does not say what it declined to assess: %q", resolution.Explanation)
	}
	for _, criterion := range resolution.Criteria {
		if strings.Contains(strings.ToLower(criterion.Statement), "confiden") {
			t.Errorf("the reframed criterion still asks about confidence: %q", criterion.Statement)
		}
	}
}

func TestAProtectedCharacteristicIsRefusedAndNeverReframed(t *testing.T) {
	t.Parallel()
	// Reframing implies the request was reasonable and only badly worded.
	// For an accent it was not, so there is nothing to offer instead.
	resolution := progression.Resolve("tell me if my accent sounds professional")
	if resolution.Outcome != progression.ResolutionRefused {
		t.Fatalf("outcome = %q, want refused", resolution.Outcome)
	}
	if len(resolution.Criteria) != 0 {
		t.Errorf("a protected characteristic was reframed into criteria: %+v", resolution.Criteria)
	}
}

func TestARequirementMovesThroughItsLifecycleAndRetiringIsFinal(t *testing.T) {
	t.Parallel()
	requirement, err := progression.NewRequirement("req-2", "give a clear greeting")
	if err != nil {
		t.Fatalf("new requirement: %v", err)
	}
	for _, status := range []string{
		progression.RequirementActive, progression.RequirementPaused,
		progression.RequirementActive, progression.RequirementRetired,
	} {
		if err := requirement.MoveTo(status); err != nil {
			t.Fatalf("moving to %q: %v", status, err)
		}
	}
	if err := requirement.MoveTo(progression.RequirementActive); err == nil {
		t.Fatal("a retired requirement came back, so retiring is not a decision")
	}
}

func TestEditingARequirementRaisesItsVersionAndLeavesTheOldCriteriaReadable(t *testing.T) {
	t.Parallel()
	// A criterion version is what an outcome is reported against, so
	// editing in place would rewrite the meaning of every result already
	// given. An edit is a new version.
	requirement, err := progression.NewRequirement("req-3", "give a clear greeting")
	if err != nil {
		t.Fatalf("new requirement: %v", err)
	}
	first := requirement.Criteria
	edited, err := requirement.Revise("give a clear greeting and ask a question at the close")
	if err != nil {
		t.Fatalf("revise: %v", err)
	}
	if edited.Version != 2 {
		t.Fatalf("version = %d, want 2", edited.Version)
	}
	if len(requirement.Criteria) != len(first) {
		t.Error("revising changed the criteria of the version already in use")
	}
	if len(edited.Criteria) <= len(first) {
		t.Errorf("the revision added nothing: %+v", edited.Criteria)
	}
}

func TestARevisionThatBecomesProhibitedIsRefused(t *testing.T) {
	t.Parallel()
	requirement, _ := progression.NewRequirement("req-4", "give a clear greeting")
	_, err := requirement.Revise("tell me whether my accent sounds professional")
	if !errors.Is(err, progression.ErrProhibitedInference) {
		t.Fatalf("err = %v, want ErrProhibitedInference", err)
	}
}

func TestAReframedRequirementCarriesTheReframingItWasGiven(t *testing.T) {
	t.Parallel()
	// A reframing the candidate cannot see is a substitution. The
	// requirement has to say what was asked for, what is being looked for
	// instead, and that the original was declined.
	requirement, err := progression.NewRequirement("req-5", "I want to sound more confident")
	if err != nil {
		t.Fatalf("new requirement: %v", err)
	}
	if requirement.Prohibited == "" {
		t.Error("the requirement does not name the inference that was declined")
	}
	if requirement.Reframing == "" {
		t.Fatal("the requirement was reframed and does not say so")
	}
	if requirement.Intent != "I want to sound more confident" {
		t.Errorf("the candidate's own words were rewritten: %q", requirement.Intent)
	}
}

// ------------------------------------------------------------- outcomes

func requirementOf(t *testing.T, intent string) progression.PersonalRequirement {
	t.Helper()
	requirement, err := progression.NewRequirement("req-outcome", intent)
	if err != nil {
		t.Fatalf("new requirement: %v", err)
	}
	return requirement
}

func TestEveryCriterionDemonstratedIsAchieved(t *testing.T) {
	t.Parallel()
	requirement := requirementOf(t, "give a clear greeting and a concise introduction")
	findings := make([]progression.CriterionFinding, 0, len(requirement.Criteria))
	for _, criterion := range requirement.Criteria {
		findings = append(findings, progression.CriterionFinding{
			CriterionID: criterion.ID, Demonstrated: true,
			Evidence: []string{"span-1"},
		})
	}
	outcome := progression.Judge(requirement, findings, true, reference)
	if outcome.Outcome != progression.RequirementAchieved {
		t.Fatalf("outcome = %q, want achieved", outcome.Outcome)
	}
	if outcome.CriterionVersion != requirement.Version {
		t.Errorf("criterion version = %d, want %d", outcome.CriterionVersion, requirement.Version)
	}
	if len(outcome.Evidence) == 0 {
		t.Error("an achieved outcome cites no evidence")
	}
}

func TestSomeCriteriaDemonstratedIsPartialAndSaysWhichWereMissing(t *testing.T) {
	t.Parallel()
	requirement := requirementOf(t, "give a clear greeting and a concise introduction")
	findings := []progression.CriterionFinding{{
		CriterionID: requirement.Criteria[0].ID, Demonstrated: true, Evidence: []string{"span-1"},
	}}
	for _, criterion := range requirement.Criteria[1:] {
		findings = append(findings, progression.CriterionFinding{CriterionID: criterion.ID})
	}
	outcome := progression.Judge(requirement, findings, true, reference)
	if outcome.Outcome != progression.RequirementPartiallyAchieved {
		t.Fatalf("outcome = %q, want partially_achieved", outcome.Outcome)
	}
	if len(outcome.Demonstrated) != 1 || len(outcome.Missing) == 0 {
		t.Fatalf("demonstrated %v, missing %v", outcome.Demonstrated, outcome.Missing)
	}
	if len(outcome.NextActions) == 0 {
		t.Error("a partial outcome offers nothing to do about it")
	}
}

func TestAnAchievementWithNoEvidenceIsNotAnAchievement(t *testing.T) {
	t.Parallel()
	// The claim this product exists not to make. A finding that says the
	// candidate did something and cannot point at where is not a finding.
	requirement := requirementOf(t, "give a clear greeting")
	outcome := progression.Judge(requirement, []progression.CriterionFinding{{
		CriterionID: requirement.Criteria[0].ID, Demonstrated: true,
	}}, true, reference)
	if outcome.Outcome != progression.RequirementNotDemonstrated {
		t.Fatalf("outcome = %q, want not_demonstrated", outcome.Outcome)
	}
	if len(outcome.Demonstrated) != 0 {
		t.Errorf("an unevidenced claim was counted as demonstrated: %v", outcome.Demonstrated)
	}
}

func TestNothingDemonstratedWithAFairChanceIsNotDemonstrated(t *testing.T) {
	t.Parallel()
	requirement := requirementOf(t, "give a clear greeting")
	findings := []progression.CriterionFinding{{CriterionID: requirement.Criteria[0].ID}}
	outcome := progression.Judge(requirement, findings, true, reference)
	if outcome.Outcome != progression.RequirementNotDemonstrated {
		t.Fatalf("outcome = %q, want not_demonstrated", outcome.Outcome)
	}
}

func TestNoFairOpportunityIsNotAssessableAndNeverAFailure(t *testing.T) {
	t.Parallel()
	// The box this ticket turns on. A session that never invited a closing
	// statement cannot report that the candidate failed to give one.
	requirement := requirementOf(t, "close the interview clearly")
	outcome := progression.Judge(requirement, nil, false, reference)
	if outcome.Outcome != progression.RequirementNotAssessable {
		t.Fatalf("outcome = %q, want not_assessable", outcome.Outcome)
	}
	// The reason has to be this one. Asserting only that some reason was
	// given let the whole branch be deleted and still pass, because the
	// no-findings branch below answers not assessable too, for a different
	// and less honest reason.
	if outcome.Reason != progression.NoFairOpportunity {
		t.Errorf("reason = %q, want no_fair_opportunity", outcome.Reason)
	}
	if len(outcome.Missing) != 0 {
		t.Errorf("a session with no fair chance listed %v as missing", outcome.Missing)
	}
	if outcome.Score() != nil {
		t.Error("a not-assessable outcome produced a number")
	}
}

func TestAnAssessableOutcomeStillOffersNoNumberForTheUnassessed(t *testing.T) {
	t.Parallel()
	// Score exists for the metrics, and returns nothing rather than zero
	// for anything not assessed, so no caller can accidentally average
	// silence into a trend.
	requirement := requirementOf(t, "give a clear greeting")
	achieved := progression.Judge(requirement, []progression.CriterionFinding{{
		CriterionID: requirement.Criteria[0].ID, Demonstrated: true, Evidence: []string{"s"},
	}}, true, reference)
	if achieved.Score() == nil || *achieved.Score() != 1 {
		t.Fatalf("an achieved outcome scores %v, want 1", achieved.Score())
	}
}

// -------------------------------------------------------------- metrics

func outcomeIn(role, shape string, version int, result string) progression.RequirementOutcome {
	return progression.RequirementOutcome{
		RequirementID: "req", CriterionVersion: version, Outcome: result,
		RoleID: role, ShapeID: shape, ObservedAt: reference,
		Demonstrated: []string{"c1"},
	}
}

func TestAMetricStatesItsDefinitionEvidenceSufficiencyVersionAndBasis(t *testing.T) {
	t.Parallel()
	metrics := progression.Measure([]progression.RequirementOutcome{
		outcomeIn("backend-engineer", "mixed", 1, progression.RequirementAchieved),
		outcomeIn("backend-engineer", "mixed", 1, progression.RequirementAchieved),
		outcomeIn("backend-engineer", "mixed", 1, progression.RequirementNotDemonstrated),
	})
	if len(metrics) != 1 {
		t.Fatalf("metrics = %d, want one series", len(metrics))
	}
	metric := metrics[0]
	if metric.Definition == "" {
		t.Error("a number with no definition")
	}
	if metric.ComparisonBasis == "" {
		t.Error("a number with no comparison basis")
	}
	if metric.CriterionVersion != 1 {
		t.Errorf("criterion version = %d", metric.CriterionVersion)
	}
	if metric.Assessed != 3 {
		t.Errorf("assessed = %d, want 3", metric.Assessed)
	}
	if metric.Sufficiency != progression.EvidenceSufficient {
		t.Errorf("sufficiency = %q", metric.Sufficiency)
	}
}

func TestIncompatibleSessionsStaySeparateSeries(t *testing.T) {
	t.Parallel()
	// Two roles, two shapes and two criterion versions produce separate
	// series rather than one flattering average.
	metrics := progression.Measure([]progression.RequirementOutcome{
		outcomeIn("backend-engineer", "mixed", 1, progression.RequirementAchieved),
		outcomeIn("engineering-manager", "mixed", 1, progression.RequirementAchieved),
		outcomeIn("backend-engineer", "behavioural", 1, progression.RequirementAchieved),
		outcomeIn("backend-engineer", "mixed", 2, progression.RequirementAchieved),
	})
	if len(metrics) != 4 {
		t.Fatalf("metrics = %d, want 4 separate series: %+v", len(metrics), metrics)
	}
}

func TestNotAssessableOutcomesAreCountedApartAndNeverAsFailures(t *testing.T) {
	t.Parallel()
	metrics := progression.Measure([]progression.RequirementOutcome{
		outcomeIn("backend-engineer", "mixed", 1, progression.RequirementAchieved),
		outcomeIn("backend-engineer", "mixed", 1, progression.RequirementNotAssessable),
		outcomeIn("backend-engineer", "mixed", 1, progression.RequirementNotAssessable),
	})
	metric := metrics[0]
	if metric.Assessed != 1 {
		t.Fatalf("assessed = %d, want 1: the unassessable are not results", metric.Assessed)
	}
	if metric.NotAssessable != 2 {
		t.Errorf("not assessable = %d, want 2", metric.NotAssessable)
	}
	if metric.Achieved != 1 {
		t.Errorf("achieved = %d, want 1", metric.Achieved)
	}
	if metric.Sufficiency != progression.EvidenceInsufficient {
		t.Errorf("sufficiency = %q, want insufficient on one assessed session", metric.Sufficiency)
	}
}

func TestOneSessionIsNeverEnoughToCallAMetricSufficient(t *testing.T) {
	t.Parallel()
	metrics := progression.Measure([]progression.RequirementOutcome{
		outcomeIn("backend-engineer", "mixed", 1, progression.RequirementAchieved),
	})
	if metrics[0].Sufficiency != progression.EvidenceInsufficient {
		t.Fatalf("sufficiency = %q, want insufficient", metrics[0].Sufficiency)
	}
}

// ---------------------------------------------------------- self-report

func TestConfidenceIsCandidateSelfReportAndCarriesItsPhase(t *testing.T) {
	t.Parallel()
	report, err := progression.NewSelfReport("session-1", progression.SelfReportBefore, 4,
		time.Now().UTC())
	if err != nil {
		t.Fatalf("self report: %v", err)
	}
	if report.Phase != progression.SelfReportBefore || report.Rating != 4 {
		t.Fatalf("report = %+v", report)
	}
}

func TestASelfReportOutsideTheScaleIsRefused(t *testing.T) {
	t.Parallel()
	for _, rating := range []int{0, 6, -1} {
		if _, err := progression.NewSelfReport("session-1", progression.SelfReportAfter,
			rating, time.Now()); err == nil {
			t.Errorf("rating %d was accepted", rating)
		}
	}
}
