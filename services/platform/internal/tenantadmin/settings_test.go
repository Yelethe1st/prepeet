package tenantadmin_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/tenantadmin"
)

// The settings document's own rules, which hold before anything reaches the
// database. Validation lives in the domain rather than in CHECK constraints
// because a refusal an administrator can read is worth more than a constraint
// violation, and because the ceiling on a screening's length is a product
// decision rather than a storage one.

// valid returns a document every field of which passes, so each test can
// break exactly one thing and know which rule fired.
func valid() tenantadmin.Settings {
	settings := tenantadmin.DefaultSettings()
	settings.Organisation.LegalName = "Northwind Health System Ltd"
	settings.Organisation.DisplayName = "Northwind Health"
	settings.Organisation.PrimaryContact = "talent@northwind.example"
	settings.Organisation.DataProtectionContact = "dpo@northwind.example"
	return settings
}

func TestADocumentWithEveryFieldSetIsAccepted(t *testing.T) {
	t.Parallel()
	if err := valid().Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

// A workspace with no display name has nothing to put on a candidate's
// invitation, which is the one place these settings are seen by somebody who
// did not choose them.
func TestADisplayNameIsRequired(t *testing.T) {
	t.Parallel()
	settings := valid()
	settings.Organisation.DisplayName = "   "
	if err := settings.Validate(); !errors.Is(err, tenantadmin.ErrSettingsInvalid) {
		t.Fatalf("Validate() = %v, want ErrSettingsInvalid", err)
	}
}

// The default length and the ceiling are two halves of one rule: a default a
// recruiter is not allowed to keep is a form that refuses its own initial
// state.
func TestTheDefaultScreeningLengthMustFitUnderTheCeiling(t *testing.T) {
	t.Parallel()
	settings := valid()
	settings.Defaults.ScreeningMinutes = 90
	settings.Defaults.MaximumMinutes = 60
	if err := settings.Validate(); !errors.Is(err, tenantadmin.ErrSettingsInvalid) {
		t.Fatalf("Validate() = %v, want ErrSettingsInvalid", err)
	}
}

func TestAScreeningLengthMustBePositive(t *testing.T) {
	t.Parallel()
	settings := valid()
	settings.Defaults.ScreeningMinutes = 0
	if err := settings.Validate(); !errors.Is(err, tenantadmin.ErrSettingsInvalid) {
		t.Fatalf("Validate() = %v, want ErrSettingsInvalid", err)
	}
}

// A reminder that fires after the deadline is a reminder about nothing.
func TestAReminderMustFallBeforeTheDeadline(t *testing.T) {
	t.Parallel()
	settings := valid()
	settings.Defaults.DeadlineDays = 3
	settings.Defaults.ReminderDays = 5
	if err := settings.Validate(); !errors.Is(err, tenantadmin.ErrSettingsInvalid) {
		t.Fatalf("Validate() = %v, want ErrSettingsInvalid", err)
	}
}

func TestTheDigestCadenceMustBeOneWeOffer(t *testing.T) {
	t.Parallel()
	settings := valid()
	settings.Notifications.DigestCadence = "hourly"
	if err := settings.Validate(); !errors.Is(err, tenantadmin.ErrSettingsInvalid) {
		t.Fatalf("Validate() = %v, want ErrSettingsInvalid", err)
	}
}

// The refusal has to say which field, because a settings page with six
// sections and one message is a page an administrator has to guess at.
func TestARefusalNamesTheFieldThatFailed(t *testing.T) {
	t.Parallel()
	settings := valid()
	settings.Organisation.DisplayName = ""
	err := settings.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want a refusal")
	}
	var invalid *tenantadmin.SettingsError
	if !errors.As(err, &invalid) {
		t.Fatalf("Validate() = %T, want *SettingsError", err)
	}
	if invalid.Field != "organisation.display_name" {
		t.Errorf("Field = %q, want organisation.display_name", invalid.Field)
	}
}

// The starting document must itself be valid apart from the identity fields
// nobody can guess, or a new workspace opens on a form that refuses to save.
func TestTheDefaultDocumentIsValidOnceTheOrganisationIsNamed(t *testing.T) {
	t.Parallel()
	settings := tenantadmin.DefaultSettings()
	settings.Organisation.LegalName = "Orbital Labs"
	settings.Organisation.DisplayName = "Orbital"
	if err := settings.Validate(); err != nil {
		t.Fatalf("Validate() on the default document = %v, want nil", err)
	}
}

// What changed is the question the audit trail answers, so the difference
// between two documents is computed rather than described by the caller.
func TestChangedFieldsNamesEveryFieldThatMoved(t *testing.T) {
	t.Parallel()
	before := valid()
	after := before
	after.Organisation.DisplayName = "Northwind"
	after.Defaults.ScreeningMinutes = before.Defaults.ScreeningMinutes + 5

	changed := tenantadmin.ChangedFields(before, after)
	want := map[string]bool{"organisation.display_name": true, "defaults.screening_minutes": true}
	if len(changed) != len(want) {
		t.Fatalf("ChangedFields() = %v, want the two fields that moved", changed)
	}
	for _, field := range changed {
		if !want[field] {
			t.Errorf("ChangedFields() reported %q, which did not move", field)
		}
	}
}

func TestChangedFieldsIsEmptyWhenNothingMoved(t *testing.T) {
	t.Parallel()
	if changed := tenantadmin.ChangedFields(valid(), valid()); len(changed) != 0 {
		t.Errorf("ChangedFields() = %v, want nothing", changed)
	}
}

// The remaining validation rules, one test each, so a failure names the rule
// rather than "the document".
func TestALegalNameIsRequired(t *testing.T) {
	t.Parallel()
	settings := valid()
	settings.Organisation.LegalName = ""
	assertField(t, settings.Validate(), "organisation.legal_name")
}

func TestAnInvitationHasToExpire(t *testing.T) {
	t.Parallel()
	settings := valid()
	settings.Defaults.DeadlineDays = 0
	assertField(t, settings.Validate(), "defaults.deadline_days")
}

// The ceiling on the ceiling. A workspace may shorten its maximum and may not
// lengthen it past what the platform will run.
func TestAWorkspaceCannotRaiseTheMaximumPastThePlatformsOwn(t *testing.T) {
	t.Parallel()
	settings := valid()
	settings.Defaults.MaximumMinutes = 480
	assertField(t, settings.Validate(), "defaults.maximum_minutes")
}

// The refusal has to read as a sentence somebody can act on, not as a code.
func TestASettingsRefusalReadsAsAReason(t *testing.T) {
	t.Parallel()
	settings := valid()
	settings.Notifications.DigestCadence = ""
	err := settings.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want a refusal")
	}
	if !strings.Contains(err.Error(), "notifications.digest_cadence") {
		t.Errorf("the refusal does not name the field: %s", err)
	}
	if !strings.Contains(err.Error(), "cadence we send on") {
		t.Errorf("the refusal does not say why: %s", err)
	}
}

// assertField checks that a refusal is a field refusal naming one field.
func assertField(t *testing.T, err error, field string) {
	t.Helper()
	var invalid *tenantadmin.SettingsError
	if !errors.As(err, &invalid) {
		t.Fatalf("Validate() = %v, want a *SettingsError", err)
	}
	if invalid.Field != field {
		t.Errorf("Field = %q, want %q", invalid.Field, field)
	}
}

// The default schedule has to be internally coherent, or the first review a
// workspace is ever offered is one it cannot finish in time.
func TestTheDefaultReviewScheduleIsCoherent(t *testing.T) {
	t.Parallel()
	schedule := tenantadmin.DefaultReviewSchedule()
	if schedule.EveryDays <= 0 || schedule.DormantAfterDays <= 0 {
		t.Fatalf("DefaultReviewSchedule() = %+v, want positive periods", schedule)
	}
	if schedule.CompleteWithinDays >= schedule.EveryDays {
		t.Errorf("a review has %d days to finish on a %d-day cadence, so the next one is due "+
			"before this one has to be done", schedule.CompleteWithinDays, schedule.EveryDays)
	}
}
