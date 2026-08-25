// Package candidate owns the candidate's own data: their profile, and in
// later tickets their documents, extracted facts and private evidence.
//
// Everything here is owner-scoped. No tenant authority reaches it through any
// route - IAM-06's guarantee, which the candidate schema's structural guards
// enforce below this package and this package must never work around.
//
// Implements PRO-01.
package candidate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/candidate/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
)

// Profile is the candidate's own record: practice targets, interview
// defaults, accessibility and notification preferences.
//
// Every field is optional. PRO-01 requires a partial profile to be usable,
// so the zero value of this struct IS a valid profile - the one every
// candidate has before their first save.
type Profile struct {
	Disciplines            []string
	TargetRoles            []string
	Seniority              string
	CareerContext          string
	DefaultDurationMinutes int
	DefaultStyle           string
	DefaultPressure        string

	// The accessibility preferences the prepare and live screens honour by
	// default. Voluntarily stored, per the domain model.
	ExtendedTime       bool
	Captions           bool
	ReducedMotion      bool
	AccessibilityNotes string

	NotifyProductUpdates    bool
	NotifyPracticeReminders bool

	UpdatedAt time.Time
}

// Validation refusals, each a stable code the API maps to a field error.
var (
	ErrCareerContextTooLong = errors.New("candidate: CAREER_CONTEXT_TOO_LONG: keep the career context under 4000 characters")
	ErrTooManyEntries       = errors.New("candidate: TOO_MANY_ENTRIES: at most 20 disciplines or target roles")
	ErrEntryTooLong         = errors.New("candidate: ENTRY_TOO_LONG: keep each discipline or role under 80 characters")
	ErrPressureUnknown      = errors.New("candidate: PRESSURE_UNKNOWN: pressure is low, standard or high")
	ErrDurationOutOfRange   = errors.New("candidate: DURATION_OUT_OF_RANGE: a default duration is between 10 and 90 minutes")
)

// Service is the candidate use cases.
type Service struct {
	store *Store
}

// NewService builds the service.
func NewService(store *Store) *Service {
	return &Service{store: store}
}

// GetProfile returns the candidate's profile, empty if never saved.
//
// Absence is not an error: a candidate who never opened the profile screen
// has the empty profile, and telling them "not found" about their own record
// would make the first visit look broken.
func (s *Service) GetProfile(ctx context.Context, userID string) (Profile, error) {
	return s.store.Get(ctx, userID)
}

// SaveProfile validates and stores the whole profile.
//
// Whole-record replace rather than patch: the profile is one screen, the
// screen submits what it shows, and field-level merges are where two tabs
// quietly assemble a profile neither of them displayed.
func (s *Service) SaveProfile(ctx context.Context, userID string, profile Profile) (Profile, error) {
	if err := validate(profile); err != nil {
		return Profile{}, err
	}
	if err := s.store.Upsert(ctx, userID, normalise(profile)); err != nil {
		return Profile{}, err
	}
	return s.store.Get(ctx, userID)
}

// validate refuses what the schema cannot express.
func validate(profile Profile) error {
	if len(profile.CareerContext) > 4000 {
		return ErrCareerContextTooLong
	}
	if len(profile.Disciplines) > 20 || len(profile.TargetRoles) > 20 {
		return ErrTooManyEntries
	}
	for _, entry := range append(append([]string{}, profile.Disciplines...), profile.TargetRoles...) {
		if len(entry) > 80 {
			return ErrEntryTooLong
		}
	}
	switch profile.DefaultPressure {
	case "", "low", "standard", "high":
	default:
		return ErrPressureUnknown
	}
	if profile.DefaultDurationMinutes != 0 &&
		(profile.DefaultDurationMinutes < 10 || profile.DefaultDurationMinutes > 90) {
		return ErrDurationOutOfRange
	}
	return nil
}

// normalise trims and drops empty entries, so "React, , Go" is two
// disciplines rather than three, and does nothing cleverer: these are the
// candidate's own words until CAT-03's catalogue exists.
func normalise(profile Profile) Profile {
	profile.Disciplines = cleaned(profile.Disciplines)
	profile.TargetRoles = cleaned(profile.TargetRoles)
	profile.Seniority = strings.TrimSpace(profile.Seniority)
	profile.CareerContext = strings.TrimSpace(profile.CareerContext)
	return profile
}

func cleaned(entries []string) []string {
	kept := make([]string, 0, len(entries))
	for _, entry := range entries {
		if trimmed := strings.TrimSpace(entry); trimmed != "" {
			kept = append(kept, trimmed)
		}
	}
	return kept
}

// Store persists profiles, always as the owner.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore builds the store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Get reads the owner's profile, or the empty one.
func (s *Store) Get(ctx context.Context, userID string) (Profile, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Profile{}, fmt.Errorf("candidate: beginning read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetUser(ctx, tx, userID); err != nil {
		return Profile{}, err
	}

	row, err := db.New(tx).GetProfile(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{NotifyPracticeReminders: true}, nil
	}
	if err != nil {
		return Profile{}, fmt.Errorf("candidate: reading profile: %w", err)
	}
	return Profile{
		Disciplines:             row.Disciplines,
		TargetRoles:             row.TargetRoles,
		Seniority:               row.Seniority,
		CareerContext:           row.CareerContext,
		DefaultDurationMinutes:  int(row.DefaultDurationMinutes),
		DefaultStyle:            row.DefaultStyle,
		DefaultPressure:         row.DefaultPressure,
		ExtendedTime:            row.ExtendedTime,
		Captions:                row.Captions,
		ReducedMotion:           row.ReducedMotion,
		AccessibilityNotes:      row.AccessibilityNotes,
		NotifyProductUpdates:    row.NotifyProductUpdates,
		NotifyPracticeReminders: row.NotifyPracticeReminders,
		UpdatedAt:               row.UpdatedAt,
	}, nil
}

// Upsert writes the whole profile as the owner.
func (s *Store) Upsert(ctx context.Context, userID string, profile Profile) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("candidate: beginning save: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetUser(ctx, tx, userID); err != nil {
		return err
	}

	if err := db.New(tx).UpsertProfile(ctx, db.UpsertProfileParams{
		UserID:                  userID,
		Disciplines:             profile.Disciplines,
		TargetRoles:             profile.TargetRoles,
		Seniority:               profile.Seniority,
		CareerContext:           profile.CareerContext,
		DefaultDurationMinutes:  int32(profile.DefaultDurationMinutes),
		DefaultStyle:            profile.DefaultStyle,
		DefaultPressure:         profile.DefaultPressure,
		ExtendedTime:            profile.ExtendedTime,
		Captions:                profile.Captions,
		ReducedMotion:           profile.ReducedMotion,
		AccessibilityNotes:      profile.AccessibilityNotes,
		NotifyProductUpdates:    profile.NotifyProductUpdates,
		NotifyPracticeReminders: profile.NotifyPracticeReminders,
	}); err != nil {
		return fmt.Errorf("candidate: saving profile: %w", err)
	}
	return tx.Commit(ctx)
}
