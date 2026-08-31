package progression_test

import (
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/internal/progression"
)

// Evidence freshness. An observation from six months ago is not the same
// claim as one from last week, and everything that reads progression has
// to be able to say which it is holding.

func TestEvidenceIsFreshAgingOrStaleByHowOldItIs(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	for _, testCase := range []struct {
		name string
		days int
		want string
	}{
		{"today", 0, progression.EvidenceFresh},
		{"a month back", 29, progression.EvidenceFresh},
		{"the day freshness runs out", 30, progression.EvidenceAging},
		{"nearly a quarter", 89, progression.EvidenceAging},
		{"the day it goes stale", 90, progression.EvidenceStale},
		{"six months", 180, progression.EvidenceStale},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got := progression.Freshness(now.AddDate(0, 0, -testCase.days), now)
			if got.Standing != testCase.want {
				t.Errorf("standing = %q, want %q", got.Standing, testCase.want)
			}
			if got.AgeDays != testCase.days {
				t.Errorf("age = %d days, want %d", got.AgeDays, testCase.days)
			}
		})
	}
}

func TestAbsentEvidenceIsAbsentRatherThanInfinitelyStale(t *testing.T) {
	t.Parallel()
	// A competency never observed has no age at all. Calling it stale would
	// be the same collapse this context refuses everywhere else: silence
	// dressed up as a measurement.
	got := progression.Freshness(time.Time{}, time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
	if got.Standing != progression.EvidenceNone {
		t.Fatalf("standing = %q, want none", got.Standing)
	}
	if got.AgeDays != 0 {
		t.Errorf("absent evidence reported an age of %d days", got.AgeDays)
	}
}

func TestEvidenceDatedInTheFutureIsNotTreatedAsFresherThanNow(t *testing.T) {
	t.Parallel()
	// Clock skew between the worker that observed and the reader that asks
	// should not manufacture negative ages that a screen would render as a
	// date in the future.
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	got := progression.Freshness(now.AddDate(0, 0, 3), now)
	if got.Standing != progression.EvidenceFresh || got.AgeDays != 0 {
		t.Fatalf("future evidence = %+v, want fresh at zero days", got)
	}
}

func TestAGoalCarriesTheFreshnessOfWhatResolvedIt(t *testing.T) {
	t.Parallel()
	// PRG-04's requirement reaching PRG-03's screen: a goal showing a band
	// from four months ago has to say the reading is four months old.
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	progress := progression.TrackGoal(goalAt("systems-design", "strong"),
		[]progression.Observation{
			bandAt("a", "systems-design", "solid", "1.0.0", now.AddDate(0, 0, -120)),
		}, nil, now)

	if progress.Evidence.Standing != progression.EvidenceStale {
		t.Fatalf("standing = %q, want stale", progress.Evidence.Standing)
	}
	if progress.Evidence.AgeDays != 120 {
		t.Errorf("age = %d, want 120", progress.Evidence.AgeDays)
	}
}
