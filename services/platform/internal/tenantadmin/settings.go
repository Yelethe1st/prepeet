// Package tenantadmin owns the administration of one employer workspace:
// its settings and branding, the periodic review of who can reach candidate
// evidence, and the rubric library.
//
// What this package deliberately does not own is versioning. The rubric
// library is a surface over the artifact registry, reached through a port,
// because content.artifacts already stores versioned, digest-identified,
// published artifacts with rollback and a rubric is one of its types. Two
// answers to "what is version 3 of this rubric" is one too many.
//
// Implements TEN-01, TEN-03 and TEN-04.
package tenantadmin

import (
	"errors"
	"fmt"
	"strings"
)

// A workspace's configuration, as one document rather than a bag of keys.
//
// One document with one version, because the two things the ticket asks for
// both need a version to hang on: an audit trail that can show what a value
// was before, and defaults that a campaign can pin so a later change never
// reaches back into it. A key/value table can express neither without
// inventing a version per key, which is the same version with more places to
// get it wrong.
//
// The shape follows the tenant settings screen in /screens, section by
// section, so the surface and the storage do not need a translation table.

// Settings is the whole configuration document for one workspace.
type Settings struct {
	Organisation        Organisation        `json:"organisation"`
	Defaults            CampaignDefaults    `json:"defaults"`
	CandidateExperience CandidateExperience `json:"candidate_experience"`
	Notifications       Notifications       `json:"notifications"`
}

// Organisation is who the workspace says it is, to candidates and to us.
type Organisation struct {
	// LegalName is the entity that answers for the hiring decision; it is
	// not necessarily what a candidate sees.
	LegalName string `json:"legal_name"`
	// DisplayName is what a candidate sees on an invitation, which is why
	// it is the one field that cannot be left empty.
	DisplayName string `json:"display_name"`
	// PrimaryContact and DataProtectionContact are addresses a candidate or
	// a regulator writes to. Kept apart because the person who answers a
	// subject access request is rarely the person running the campaign.
	PrimaryContact        string `json:"primary_contact"`
	DataProtectionContact string `json:"data_protection_contact"`
	Industry              string `json:"industry"`
	// BrandColour is the workspace's accent, as a hex triple. Branding is a
	// value here rather than an uploaded asset because an asset needs the
	// object store's scoped upload path, which is PLT-05's, not this one's.
	BrandColour string `json:"brand_colour"`
}

// CampaignDefaults is what a new campaign starts from.
//
// Starts from, and nothing more: these are copied into a campaign when it is
// created and never consulted again, which is the whole of "defaults apply to
// new campaigns only". A campaign that read them live would silently change
// length or deadline under a candidate who had already been invited.
type CampaignDefaults struct {
	ScreeningMinutes int `json:"screening_minutes"`
	// MaximumMinutes is the ceiling a recruiter may set, not a default.
	MaximumMinutes int `json:"maximum_minutes"`
	DeadlineDays   int `json:"deadline_days"`
	// ReminderDays is how long after the invitation the reminder goes out.
	ReminderDays int `json:"reminder_days"`
}

// CandidateExperience is the copy a candidate reads, which is the part of
// this document seen by somebody who did not choose it.
type CandidateExperience struct {
	InvitationSubject string `json:"invitation_subject"`
	InvitationOpening string `json:"invitation_opening"`
	// EvidenceDisclosure is what candidates are told the employer receives.
	// The binding disclosure is SCR-02's versioned artifact; this is the
	// workspace's own wording around it and never a substitute for it.
	EvidenceDisclosure string `json:"evidence_disclosure"`
	// AccommodationsStatement is the adjustments offer every candidate sees.
	AccommodationsStatement string `json:"accommodations_statement"`
}

// Notifications is who inside the workspace hears about what.
type Notifications struct {
	OnScreeningSubmitted bool `json:"on_screening_submitted"`
	OnLowConfidence      bool `json:"on_low_confidence"`
	OnAppealRaised       bool `json:"on_appeal_raised"`
	OnQuotaThreshold     bool `json:"on_quota_threshold"`
	// DigestCadence batches the above rather than sending each one.
	DigestCadence string `json:"digest_cadence"`
}

// The cadences offered. A set rather than free text, because a cadence is
// read by a scheduler and "when I feel like it" has no schedule.
var digestCadences = map[string]bool{"immediate": true, "daily": true, "weekly": true}

// maximumScreeningMinutes is the ceiling on the ceiling.
//
// A workspace may set its own maximum below this and not above it: a
// three-hour screening is a burden on a candidate that no employer's
// preference should be able to impose, and the platform's cost model in
// ADR-0014 is built on sessions that end.
const maximumScreeningMinutes = 120

// ErrSettingsInvalid is the class every settings refusal belongs to, so a
// caller can branch on "the administrator typed something we cannot store"
// without enumerating fields.
var ErrSettingsInvalid = errors.New("tenantadmin: SETTINGS_INVALID: the settings document is not valid")

// SettingsError names the field that failed.
//
// The field is carried rather than only described, because a settings screen
// has six sections and one unattributed message makes an administrator guess
// which of them refused.
type SettingsError struct {
	Field  string
	Reason string
}

func (e *SettingsError) Error() string {
	return fmt.Sprintf("tenantadmin: SETTINGS_INVALID: %s: %s", e.Field, e.Reason)
}

// Unwrap makes every field refusal answer errors.Is(err, ErrSettingsInvalid).
func (e *SettingsError) Unwrap() error { return ErrSettingsInvalid }

// DefaultSettings is what an unconfigured workspace reads.
//
// A workspace that has never saved anything must still render its settings
// screen, so absence answers with this rather than with an error. Every value
// here is one a workspace could keep unchanged: the document is valid as soon
// as the organisation is named, which is the only thing nobody can guess.
func DefaultSettings() Settings {
	return Settings{
		Defaults: CampaignDefaults{
			ScreeningMinutes: 30,
			MaximumMinutes:   60,
			DeadlineDays:     7,
			ReminderDays:     3,
		},
		CandidateExperience: CandidateExperience{
			InvitationSubject: "You have been invited to a screening interview",
			InvitationOpening: "Thank you for your interest. This screening is a short " +
				"conversation with an AI interviewer, and a person reviews the result.",
			EvidenceDisclosure: "The employer receives your recorded answers, a transcript, " +
				"and an evaluation against the role's published rubric.",
			AccommodationsStatement: "If you need an adjustment to take part, tell us before " +
				"you start and we will arrange it.",
		},
		Notifications: Notifications{
			OnScreeningSubmitted: true,
			OnLowConfidence:      true,
			OnAppealRaised:       true,
			OnQuotaThreshold:     true,
			DigestCadence:        "daily",
		},
	}
}

// Validate reports the first rule the document breaks.
//
// First rather than all: the surface saves a whole document at once, and a
// list of failures is only useful to a form that can attribute each one, which
// is a promise this port cannot keep for callers it does not know about.
func (s Settings) Validate() error {
	if strings.TrimSpace(s.Organisation.DisplayName) == "" {
		return &SettingsError{Field: "organisation.display_name",
			Reason: "a candidate's invitation has to name the employer"}
	}
	if strings.TrimSpace(s.Organisation.LegalName) == "" {
		return &SettingsError{Field: "organisation.legal_name",
			Reason: "the entity answering for the hiring decision has to be named"}
	}
	if s.Defaults.ScreeningMinutes <= 0 {
		return &SettingsError{Field: "defaults.screening_minutes",
			Reason: "a screening has to have a length"}
	}
	if s.Defaults.MaximumMinutes > maximumScreeningMinutes {
		return &SettingsError{Field: "defaults.maximum_minutes",
			Reason: fmt.Sprintf("a screening may not run longer than %d minutes", maximumScreeningMinutes)}
	}
	if s.Defaults.ScreeningMinutes > s.Defaults.MaximumMinutes {
		return &SettingsError{Field: "defaults.screening_minutes",
			Reason: "the default length has to fit under the maximum a recruiter may set"}
	}
	if s.Defaults.DeadlineDays <= 0 {
		return &SettingsError{Field: "defaults.deadline_days",
			Reason: "an invitation has to expire"}
	}
	if s.Defaults.ReminderDays <= 0 || s.Defaults.ReminderDays >= s.Defaults.DeadlineDays {
		return &SettingsError{Field: "defaults.reminder_days",
			Reason: "a reminder has to fall before the deadline it is reminding about"}
	}
	if !digestCadences[s.Notifications.DigestCadence] {
		return &SettingsError{Field: "notifications.digest_cadence",
			Reason: "that is not a cadence we send on"}
	}
	return nil
}

// ChangedFields names every field that differs between two documents.
//
// Computed rather than described by the caller, because the audit trail's
// worth depends on it being right, and a caller that has to say what it
// changed is a caller that can forget to.
func ChangedFields(before, after Settings) []string {
	var changed []string
	compare := func(field string, same bool) {
		if !same {
			changed = append(changed, field)
		}
	}

	compare("organisation.legal_name", before.Organisation.LegalName == after.Organisation.LegalName)
	compare("organisation.display_name", before.Organisation.DisplayName == after.Organisation.DisplayName)
	compare("organisation.primary_contact", before.Organisation.PrimaryContact == after.Organisation.PrimaryContact)
	compare("organisation.data_protection_contact",
		before.Organisation.DataProtectionContact == after.Organisation.DataProtectionContact)
	compare("organisation.industry", before.Organisation.Industry == after.Organisation.Industry)
	compare("organisation.brand_colour", before.Organisation.BrandColour == after.Organisation.BrandColour)

	compare("defaults.screening_minutes", before.Defaults.ScreeningMinutes == after.Defaults.ScreeningMinutes)
	compare("defaults.maximum_minutes", before.Defaults.MaximumMinutes == after.Defaults.MaximumMinutes)
	compare("defaults.deadline_days", before.Defaults.DeadlineDays == after.Defaults.DeadlineDays)
	compare("defaults.reminder_days", before.Defaults.ReminderDays == after.Defaults.ReminderDays)

	compare("candidate_experience.invitation_subject",
		before.CandidateExperience.InvitationSubject == after.CandidateExperience.InvitationSubject)
	compare("candidate_experience.invitation_opening",
		before.CandidateExperience.InvitationOpening == after.CandidateExperience.InvitationOpening)
	compare("candidate_experience.evidence_disclosure",
		before.CandidateExperience.EvidenceDisclosure == after.CandidateExperience.EvidenceDisclosure)
	compare("candidate_experience.accommodations_statement",
		before.CandidateExperience.AccommodationsStatement == after.CandidateExperience.AccommodationsStatement)

	compare("notifications.on_screening_submitted",
		before.Notifications.OnScreeningSubmitted == after.Notifications.OnScreeningSubmitted)
	compare("notifications.on_low_confidence",
		before.Notifications.OnLowConfidence == after.Notifications.OnLowConfidence)
	compare("notifications.on_appeal_raised",
		before.Notifications.OnAppealRaised == after.Notifications.OnAppealRaised)
	compare("notifications.on_quota_threshold",
		before.Notifications.OnQuotaThreshold == after.Notifications.OnQuotaThreshold)
	compare("notifications.digest_cadence",
		before.Notifications.DigestCadence == after.Notifications.DigestCadence)

	return changed
}
