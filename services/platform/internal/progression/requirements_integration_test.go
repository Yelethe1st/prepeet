//go:build integration

package progression_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/internal/progression"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// PRG-06 against real PostgreSQL. What is worth attacking here is not the
// happy path but the three refusals: an employer reaching a private
// requirement, an outcome being edited after the fact, and a session that
// asked nothing being recorded as a session the candidate failed.

const requirementCandidateID = "00000000-0000-7000-8000-0000000000e1"
const requirementOtherID = "00000000-0000-7000-8000-0000000000e2"

func requirementOwner(candidate string) progression.Owner {
	return progression.Owner{Mode: "practice", CandidateID: candidate}
}

// storedRequirement writes one active requirement for a candidate.
func storedRequirement(t *testing.T, candidate, intent string) progression.PersonalRequirement {
	t.Helper()
	requirement, err := progression.NewRequirement(id.New().String(), intent)
	if err != nil {
		t.Fatalf("new requirement: %v", err)
	}
	if err := requirement.MoveTo(progression.RequirementActive); err != nil {
		t.Fatalf("activate: %v", err)
	}
	if err := progression.NewStore(pool).CreateRequirement(
		context.Background(), requirementOwner(candidate), requirement); err != nil {
		t.Fatalf("create requirement: %v", err)
	}
	return requirement
}

func TestARequirementIsReadBackWithTheCriteriaOfTheVersionInUse(t *testing.T) {
	ctx := context.Background()
	requirement := storedRequirement(t, requirementCandidateID,
		"I want to give a clear greeting and a concise introduction")

	stored, err := progression.NewStore(pool).Requirements(ctx, requirementOwner(requirementCandidateID))
	if err != nil {
		t.Fatalf("requirements: %v", err)
	}
	for _, found := range stored {
		if found.ID != requirement.ID {
			continue
		}
		if found.Version != 1 {
			t.Errorf("version = %d, want 1", found.Version)
		}
		if len(found.Criteria) != len(requirement.Criteria) {
			t.Fatalf("criteria = %d, want %d", len(found.Criteria), len(requirement.Criteria))
		}
		if found.Criteria[0].Statement == "" {
			t.Error("a criterion came back with nothing the candidate could read")
		}
		if found.Intent != requirement.Intent {
			t.Errorf("intent = %q, want the candidate's own words", found.Intent)
		}
		return
	}
	t.Fatal("the requirement just created is missing")
}

func TestARevisionLeavesTheEarlierVersionsCriteriaReadable(t *testing.T) {
	ctx := context.Background()
	store := progression.NewStore(pool)
	owner := requirementOwner(requirementCandidateID)
	requirement := storedRequirement(t, requirementCandidateID, "I want to give a clear greeting")

	revised, err := requirement.Revise("give a clear greeting and ask a question at the close")
	if err != nil {
		t.Fatalf("revise: %v", err)
	}
	if err := store.ReviseRequirement(ctx, owner, revised); err != nil {
		t.Fatalf("store revision: %v", err)
	}

	history, err := store.Export(ctx, owner)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	versions := history.Criteria[requirement.ID]
	if len(versions[1]) != len(requirement.Criteria) {
		t.Fatalf("version 1 criteria = %d, want %d: an edit rewrote the past",
			len(versions[1]), len(requirement.Criteria))
	}
	if len(versions[2]) != len(revised.Criteria) {
		t.Fatalf("version 2 criteria = %d, want %d", len(versions[2]), len(revised.Criteria))
	}
}

func TestRaisingThenLoweringARequirementVersionIsRefused(t *testing.T) {
	ctx := context.Background()
	store := progression.NewStore(pool)
	owner := requirementOwner(requirementCandidateID)
	requirement := storedRequirement(t, requirementCandidateID, "give a clear greeting")
	revised, err := requirement.Revise("give a clear greeting and a concise introduction")
	if err != nil {
		t.Fatalf("revise: %v", err)
	}
	if err := store.ReviseRequirement(ctx, owner, revised); err != nil {
		t.Fatalf("store revision: %v", err)
	}

	_, err = scopedTo(t, owner,
		`UPDATE progression.personal_requirements SET version = 1 WHERE id = $1`, requirement.ID)
	if err == nil {
		t.Fatal("a requirement version fell, so an existing outcome now names criteria it never had")
	}
	if !strings.Contains(err.Error(), "cannot fall") {
		t.Errorf("the refusal does not name the rule: %v", err)
	}
}

func TestACriterionCannotBeEditedInPlace(t *testing.T) {
	requirement := storedRequirement(t, requirementCandidateID, "ask the interviewer a question")
	_, err := scopedTo(t, requirementOwner(requirementCandidateID),
		`UPDATE progression.requirement_criteria SET statement = 'something else'
		 WHERE requirement_id = $1`, requirement.ID)
	if err == nil {
		t.Fatal("a criterion was edited, so every outcome against it now means something else")
	}
	if !strings.Contains(err.Error(), "immutable per version") {
		t.Errorf("the refusal does not name the rule: %v", err)
	}
}

func TestASessionWithNoFairOpportunityIsStoredAsNotAssessableAndBlamesNobody(t *testing.T) {
	ctx := context.Background()
	store := progression.NewStore(pool)
	owner := requirementOwner(requirementCandidateID)
	requirement := storedRequirement(t, requirementCandidateID, "close the interview clearly")

	outcome := progression.Judge(requirement, nil, false, time.Now().UTC())
	outcome.SessionID = id.New().String()
	outcome.RoleID, outcome.ShapeID = "backend-engineer", "mixed"
	if err := store.RecordOutcome(ctx, owner, outcome); err != nil {
		t.Fatalf("record: %v", err)
	}

	stored, err := store.Outcomes(ctx, owner)
	if err != nil {
		t.Fatalf("outcomes: %v", err)
	}
	for _, found := range stored {
		if found.SessionID != outcome.SessionID {
			continue
		}
		if found.Outcome != progression.RequirementNotAssessable {
			t.Fatalf("outcome = %q", found.Outcome)
		}
		if len(found.Missing) != 0 || len(found.Demonstrated) != 0 {
			t.Errorf("a not-assessable outcome blames somebody: missing %v demonstrated %v",
				found.Missing, found.Demonstrated)
		}
		if found.Reason == "" {
			t.Error("not assessable with no reason")
		}
		return
	}
	t.Fatal("the outcome just recorded is missing")
}

func TestTheSchemaRefusesANotAssessableOutcomeThatBlamesSomebody(t *testing.T) {
	// The Go refuses this too, so the attack goes straight at the table:
	// the constraint is what a future projection written by somebody else
	// will be held to.
	requirement := storedRequirement(t, requirementCandidateID, "give a clear greeting")
	_, err := scopedTo(t, requirementOwner(requirementCandidateID),
		`INSERT INTO progression.requirement_outcomes
		     (id, requirement_id, candidate_id, criterion_version, session_id,
		      outcome, reason, demonstrated, missing, observed_at)
		 VALUES ($1, $2, $3, 1, $4, 'not_assessable', 'no_fair_opportunity',
		         '{}', '{greeting}', now())`,
		id.New().String(), requirement.ID, requirementCandidateID, id.New().String())
	if err == nil {
		t.Fatal("a session that never asked recorded the candidate as having missed something")
	}
	if !strings.Contains(err.Error(), "not_assessable_blames_nobody") {
		t.Errorf("the refusal came from somewhere else: %v", err)
	}
}

func TestTheSchemaRefusesAnAssessedOutcomeWearingAReason(t *testing.T) {
	requirement := storedRequirement(t, requirementCandidateID, "ask the interviewer a question")
	_, err := scopedTo(t, requirementOwner(requirementCandidateID),
		`INSERT INTO progression.requirement_outcomes
		     (id, requirement_id, candidate_id, criterion_version, session_id,
		      outcome, reason, demonstrated, missing, observed_at)
		 VALUES ($1, $2, $3, 1, $4, 'achieved', 'no_fair_opportunity',
		         '{questions}', '{}', now())`,
		id.New().String(), requirement.ID, requirementCandidateID, id.New().String())
	if err == nil {
		t.Fatal("an achieved outcome carries an unassessable reason")
	}
	if !strings.Contains(err.Error(), "not_assessable_states_why") {
		t.Errorf("the refusal came from somewhere else: %v", err)
	}
}

func TestARecordedOutcomeCannotBeEdited(t *testing.T) {
	ctx := context.Background()
	store := progression.NewStore(pool)
	owner := requirementOwner(requirementCandidateID)
	requirement := storedRequirement(t, requirementCandidateID, "support claims with a concrete example")

	outcome := progression.Judge(requirement, []progression.CriterionFinding{{
		CriterionID: requirement.Criteria[0].ID, Demonstrated: true, Evidence: []string{"span-1"},
	}}, true, time.Now().UTC())
	outcome.SessionID = id.New().String()
	if err := store.RecordOutcome(ctx, owner, outcome); err != nil {
		t.Fatalf("record: %v", err)
	}

	_, err := scopedTo(t, owner,
		`UPDATE progression.requirement_outcomes SET outcome = 'not_demonstrated'
		 WHERE session_id = $1`, outcome.SessionID)
	if err == nil {
		t.Fatal("a recorded outcome was edited")
	}
	if !strings.Contains(err.Error(), "cannot be edited") {
		t.Errorf("the refusal does not name the rule: %v", err)
	}
}

func TestNoEmployerAuthorityReachesAPersonalRequirement(t *testing.T) {
	// The attack sets a tenant AND the owner's own user id, because setting
	// the tenant alone would be refused by the candidate clause and would
	// leave the tenant clause untested.
	ctx := context.Background()
	requirement := storedRequirement(t, requirementCandidateID, "name the trade-offs behind a choice")
	const employerTenantID = "00000000-0000-7000-8000-0000000000a9"

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx,
		`SELECT set_config('app.user_id', $1, true), set_config('app.tenant_id', $2, true)`,
		requirementCandidateID, employerTenantID); err != nil {
		t.Fatalf("scoping: %v", err)
	}

	for _, table := range []string{
		"progression.personal_requirements",
		"progression.requirement_criteria",
	} {
		var visible int
		column := "id"
		if table == "progression.requirement_criteria" {
			column = "requirement_id"
		}
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM `+table+` WHERE `+column+` = $1`, requirement.ID).Scan(&visible); err != nil {
			t.Fatalf("counting %s: %v", table, err)
		}
		if visible != 0 {
			t.Errorf("a request carrying a tenant read %d rows of %s", visible, table)
		}
	}

	// With the tenant cleared the same read finds the rows, so the refusals
	// above were the tenant clause and not the rows being absent.
	if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', '', true)`); err != nil {
		t.Fatalf("clearing the tenant: %v", err)
	}
	var visible int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM progression.personal_requirements WHERE id = $1`,
		requirement.ID).Scan(&visible); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if visible != 1 {
		t.Fatal("the requirement is invisible to its own owner too, so the attack proved nothing")
	}
}

func TestOneCandidatesRequirementsAndOutcomesAreInvisibleToAnother(t *testing.T) {
	ctx := context.Background()
	store := progression.NewStore(pool)
	owner := requirementOwner(requirementCandidateID)
	requirement := storedRequirement(t, requirementCandidateID, "keep answers concise")

	outcome := progression.Judge(requirement, []progression.CriterionFinding{{
		CriterionID: requirement.Criteria[0].ID, Demonstrated: true, Evidence: []string{"span-9"},
	}}, true, time.Now().UTC())
	outcome.SessionID = id.New().String()
	if err := store.RecordOutcome(ctx, owner, outcome); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := store.RecordSelfReport(ctx, owner, progression.SelfReport{
		SessionID: outcome.SessionID, Phase: progression.SelfReportBefore,
		Rating: 3, ReportedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("self report: %v", err)
	}

	// Every attack is aimed at a row known to exist under the first
	// candidate. An unscoped attack matches nothing whether the policies
	// work or not.
	other := requirementOwner(requirementOtherID)
	for _, attack := range []struct {
		name      string
		statement string
		argument  string
	}{
		{"the requirement", `DELETE FROM progression.personal_requirements WHERE id = $1`, requirement.ID},
		{"its criteria", `DELETE FROM progression.requirement_criteria WHERE requirement_id = $1`, requirement.ID},
		{"its outcomes", `DELETE FROM progression.requirement_outcomes WHERE session_id = $1`, outcome.SessionID},
		{"the self-report", `DELETE FROM progression.confidence_self_reports WHERE session_id = $1`, outcome.SessionID},
	} {
		matched, err := scopedTo(t, other, attack.statement, attack.argument)
		if err != nil {
			t.Fatalf("the attack on %s failed for the wrong reason: %v", attack.name, err)
		}
		if matched != 0 {
			t.Errorf("another candidate deleted %d rows of %s", matched, attack.name)
		}
	}

	// And the store, asked as the other candidate, answers nothing.
	history, err := store.Export(ctx, other)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	for _, found := range history.Requirements {
		if found.ID == requirement.ID {
			t.Fatal("another candidate can read this requirement")
		}
	}
	for _, found := range history.Outcomes {
		if found.SessionID == outcome.SessionID {
			t.Fatal("another candidate can read this outcome")
		}
	}
	for _, found := range history.SelfReports {
		if found.SessionID == outcome.SessionID {
			t.Fatal("another candidate can read this self-report")
		}
	}
}

func TestACandidateCanErasetheirOwnRequirementAndEverythingRecordedAgainstIt(t *testing.T) {
	ctx := context.Background()
	store := progression.NewStore(pool)
	owner := requirementOwner(requirementCandidateID)
	requirement := storedRequirement(t, requirementCandidateID, "open with a greeting")

	outcome := progression.Judge(requirement, []progression.CriterionFinding{{
		CriterionID: requirement.Criteria[0].ID, Demonstrated: true, Evidence: []string{"span-2"},
	}}, true, time.Now().UTC())
	outcome.SessionID = id.New().String()
	if err := store.RecordOutcome(ctx, owner, outcome); err != nil {
		t.Fatalf("record: %v", err)
	}

	if err := store.EraseRequirement(ctx, owner, requirement.ID); err != nil {
		t.Fatalf("erase: %v", err)
	}
	history, err := store.Export(ctx, owner)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	for _, found := range history.Requirements {
		if found.ID == requirement.ID {
			t.Fatal("the requirement survived erasure")
		}
	}
	for _, found := range history.Outcomes {
		if found.RequirementID == requirement.ID {
			t.Fatal("an outcome survived the erasure of the thing it was about")
		}
	}
	if history.Criteria[requirement.ID] != nil {
		t.Fatal("the criteria survived erasure")
	}
}

func TestPersonalisationDefaultsToOnAndCanBeTurnedOffWithoutDeletingAnything(t *testing.T) {
	ctx := context.Background()
	store := progression.NewStore(pool)
	owner := requirementOwner(requirementOtherID)
	storedRequirement(t, requirementOtherID, "ask the interviewer a question")

	history, err := store.Export(ctx, owner)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !history.PersonalisationEnabled {
		t.Fatal("personalisation is off for a candidate who never expressed a preference")
	}

	if err := store.SetPersonalisation(ctx, owner, false); err != nil {
		t.Fatalf("set personalisation: %v", err)
	}
	after, err := store.Export(ctx, owner)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if after.PersonalisationEnabled {
		t.Fatal("personalisation is still on after being turned off")
	}
	// Turning it off is not a deletion: stopping the use of history and
	// erasing it are two different things a candidate might want.
	if len(after.Requirements) == 0 {
		t.Fatal("turning personalisation off deleted the history")
	}
}

func TestAConfidenceSelfReportIsStoredApartFromEveryEvaluatedReading(t *testing.T) {
	ctx := context.Background()
	store := progression.NewStore(pool)
	owner := requirementOwner(requirementCandidateID)
	sessionID := id.New().String()

	for _, phase := range []string{progression.SelfReportBefore, progression.SelfReportAfter} {
		report, err := progression.NewSelfReport(sessionID, phase, 4, time.Now().UTC())
		if err != nil {
			t.Fatalf("self report: %v", err)
		}
		if err := store.RecordSelfReport(ctx, owner, report); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	// The structural claim: the self-report table carries no column that
	// could tie a rating to a rubric, a band or a piece of evidence, so
	// nothing here can become an evaluated reading by being joined.
	rows, err := pool.Query(ctx,
		`SELECT column_name FROM information_schema.columns
		 WHERE table_schema = 'progression' AND table_name = 'confidence_self_reports'`)
	if err != nil {
		t.Fatalf("columns: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatalf("scan: %v", err)
		}
		for _, forbidden := range []string{"rubric", "band", "criterion", "evidence", "observation"} {
			if strings.Contains(column, forbidden) {
				t.Errorf("confidence_self_reports carries %q, which lets a self-rating "+
					"be joined to an evaluated reading", column)
			}
		}
	}
}
