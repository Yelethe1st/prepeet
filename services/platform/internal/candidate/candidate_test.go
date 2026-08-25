package candidate

import (
	"errors"
	"strings"
	"testing"
)

// The pure half: what a profile may say, and how its entries are tidied.

func TestTheEmptyProfileIsValid(t *testing.T) {
	t.Parallel()
	// PRO-01's third criterion at its root: the zero value passes validation,
	// because a partial profile - including a completely absent one - is
	// usable and nothing hides behind completeness.
	if err := validate(Profile{}); err != nil {
		t.Fatalf("the empty profile was refused: %v", err)
	}
}

func TestTheBoundsRefuseWhatTheyName(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		profile Profile
		want    error
	}{
		"career context over 4000": {
			Profile{CareerContext: strings.Repeat("x", 4001)}, ErrCareerContextTooLong},
		"twenty-one disciplines": {
			Profile{Disciplines: make([]string, 21)}, ErrTooManyEntries},
		"an eighty-one character role": {
			Profile{TargetRoles: []string{strings.Repeat("r", 81)}}, ErrEntryTooLong},
		"an invented pressure": {
			Profile{DefaultPressure: "extreme"}, ErrPressureUnknown},
		"a nine minute default": {
			Profile{DefaultDurationMinutes: 9}, ErrDurationOutOfRange},
		"a ninety-one minute default": {
			Profile{DefaultDurationMinutes: 91}, ErrDurationOutOfRange},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if err := validate(tc.profile); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestTheEdgesOfEveryBoundAreInside(t *testing.T) {
	t.Parallel()
	edge := Profile{
		CareerContext:          strings.Repeat("x", 4000),
		Disciplines:            make([]string, 20),
		TargetRoles:            []string{strings.Repeat("r", 80)},
		DefaultPressure:        "high",
		DefaultDurationMinutes: 90,
	}
	if err := validate(edge); err != nil {
		t.Fatalf("a profile exactly on every bound was refused: %v", err)
	}
}

func TestNormaliseTidiesWithoutInterpreting(t *testing.T) {
	t.Parallel()
	tidied := normalise(Profile{
		Disciplines:   []string{"  Go ", "", "  ", "React"},
		TargetRoles:   []string{" Staff Engineer "},
		Seniority:     "  senior  ",
		CareerContext: "  ten years in payments  ",
	})

	if len(tidied.Disciplines) != 2 || tidied.Disciplines[0] != "Go" || tidied.Disciplines[1] != "React" {
		t.Fatalf("disciplines = %v", tidied.Disciplines)
	}
	if tidied.TargetRoles[0] != "Staff Engineer" {
		t.Fatalf("roles = %v", tidied.TargetRoles)
	}
	if tidied.Seniority != "senior" || tidied.CareerContext != "ten years in payments" {
		t.Fatalf("trimming missed: %q %q", tidied.Seniority, tidied.CareerContext)
	}
}
