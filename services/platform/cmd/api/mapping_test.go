package main

import (
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/internal/candidate"
	"github.com/Yelethe1st/prepeet/services/platform/internal/identity"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
)

// The mappings between a context's vocabulary and the API's.
//
// These are the quietest thing in the composition root. A field added to a
// domain type and forgotten in the mapping compiles, passes every other test,
// and loses the value on its way to the person who typed it: somebody sets an
// accessibility preference, it is saved, and it comes back empty forever.
//
// So the tests are structural rather than a list of fields. A round trip
// proves nothing survives being dropped, and a reflection walk proves the next
// field cannot be forgotten either.

// nonZeroProfile fills every field, so a mapping that drops one is visible as
// a zero value rather than as a value that happened to match.
func nonZeroProfile() candidate.Profile {
	return candidate.Profile{
		Disciplines:             []string{"software"},
		TargetRoles:             []string{"backend"},
		Seniority:               "senior",
		CareerContext:           "eleven years, mostly payments",
		DefaultDurationMinutes:  45,
		DefaultStyle:            "technical",
		DefaultPressure:         "high",
		ExtendedTime:            true,
		Captions:                true,
		ReducedMotion:           true,
		AccessibilityNotes:      "please speak a little slower",
		NotifyProductUpdates:    true,
		NotifyPracticeReminders: true,
	}
}

func TestAProfileSurvivesTheRoundTrip(t *testing.T) {
	original := nonZeroProfile()

	returned := fromAPIProfile(toAPIProfile(original))

	if !reflect.DeepEqual(original, returned) {
		t.Fatalf("the profile changed on the way out and back:\n  before %+v\n  after  %+v",
			original, returned)
	}
}

// The round trip above passes if a field is dropped in both directions
// symmetrically, which is exactly what copying one mapping to write the other
// produces. This walks the fields instead.
func TestEveryProfileFieldIsCarriedOut(t *testing.T) {
	mapped := toAPIProfile(nonZeroProfile())

	dropped := zeroFields(t, reflect.ValueOf(mapped), "candidate.Profile")

	sort.Strings(dropped)
	if len(dropped) > 0 {
		t.Fatalf("these fields are set on the domain profile and empty on the API one,\n"+
			"so whatever a candidate put in them is lost on the way out:\n  %s",
			strings.Join(dropped, "\n  "))
	}
}

func TestEveryProfileFieldIsCarriedBack(t *testing.T) {
	mapped := fromAPIProfile(toAPIProfile(nonZeroProfile()))

	dropped := zeroFields(t, reflect.ValueOf(mapped), "candidate.Profile")

	sort.Strings(dropped)
	if len(dropped) > 0 {
		t.Fatalf("these fields are lost on the way back in, so a saved profile\n"+
			"comes back empty:\n  %s", strings.Join(dropped, "\n  "))
	}
}

// notCarried are the fields a mapping deliberately does not populate, each
// with the reason.
//
// A declaration rather than a looser test, so that leaving a field out is a
// decision in a diff. Both of these were found by the walk below reporting
// them, which is the check working: one is correct and one would have been a
// defect, and only reading them tells you which.
var notCarried = map[string]map[string]string{
	"candidate.Profile": {
		// Server-set. A client does not choose when its profile was last
		// written, and accepting one would let it lie about that.
		"UpdatedAt": "set by the store on write, never sent by a client",
	},
	"api.Session": {
		// A session is issued acting under no workspace. The selection is made
		// afterwards and reaches responses through the principal, not through
		// the session the sign-in returned.
		"ActiveTenantID": "chosen after sign-in, and carried by the principal",
	},
}

// zeroFields names every exported field left at its zero value, less the ones
// declared as deliberately not carried.
func zeroFields(t *testing.T, value reflect.Value, kind string) []string {
	t.Helper()

	exempt := notCarried[kind]
	empty := []string{}
	for i := 0; i < value.NumField(); i++ {
		field := value.Type().Field(i)
		if !field.IsExported() {
			continue
		}
		if _, allowed := exempt[field.Name]; allowed {
			continue
		}
		if value.Field(i).IsZero() {
			empty = append(empty, field.Name)
		}
	}
	return empty
}

// The exemptions cannot outlive the fields they name, or one becomes a licence
// for a field that is genuinely being dropped the day a name is reused.
func TestNothingIsExemptedFromAMappingThatDoesNotExist(t *testing.T) {
	present := map[string]map[string]bool{
		"candidate.Profile": fieldNames(reflect.TypeOf(candidate.Profile{})),
		"api.Session":       fieldNames(reflect.TypeOf(sessionFrom(identity.Session{}))),
	}

	stale := []string{}
	for kind, fields := range notCarried {
		for name := range fields {
			if !present[kind][name] {
				stale = append(stale, kind+"."+name)
			}
		}
	}

	sort.Strings(stale)
	if len(stale) > 0 {
		t.Fatalf("these are exempted and no longer exist:\n  %s", strings.Join(stale, "\n  "))
	}
}

func fieldNames(t reflect.Type) map[string]bool {
	names := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		names[t.Field(i).Name] = true
	}
	return names
}

// A session is the one mapping where a dropped field is not a lost preference
// but a person who cannot stay signed in.
func TestASessionCarriesEveryFieldTheApiNeeds(t *testing.T) {
	mapped := sessionFrom(identity.Session{
		ID: "sid", UserID: "uid", FamilyID: "fam",
		SessionToken: "ses_x", RefreshToken: "ref_x",
		ExpiresAt:       nonZeroTime(),
		RefreshExpires:  nonZeroTime(),
		AuthenticatedAt: nonZeroTime(),
	})

	dropped := zeroFields(t, reflect.ValueOf(mapped), "api.Session")

	sort.Strings(dropped)
	if len(dropped) > 0 {
		t.Fatalf("these session fields did not reach the API:\n  %s", strings.Join(dropped, "\n  "))
	}
}

// oauthProviders is the seam IAM-08's sixth criterion rests on: a provider is
// configuration, and a half-configured one must be absent rather than broken.
func TestOnlyFullyConfiguredProvidersAreOffered(t *testing.T) {
	complete := config.OAuthProvider{
		ClientID: "id", ClientSecret: "secret", RedirectURI: "https://app.example/cb",
		AuthorizeURL: "https://p.example/auth", TokenURL: "https://p.example/token",
		UserInfoURL: "https://p.example/userinfo",
	}

	t.Run("none configured", func(t *testing.T) {
		if got := oauthProviders(config.Config{}); len(got) != 0 {
			t.Fatalf("a deployment configuring nothing offered %v", got)
		}
	})

	t.Run("one configured", func(t *testing.T) {
		got := oauthProviders(config.Config{OAuthGoogle: complete})
		if len(got) != 1 {
			t.Fatalf("want google alone, got %d providers", len(got))
		}
		if _, ok := got["google"]; !ok {
			t.Fatalf("google is not among %v", got)
		}
	})

	t.Run("a missing secret leaves it out rather than offering it broken", func(t *testing.T) {
		half := complete
		half.ClientSecret = ""

		if got := oauthProviders(config.Config{OAuthGoogle: half}); len(got) != 0 {
			// Offering the button and failing at the token endpoint is a worse
			// experience than not offering it.
			t.Fatalf("a provider with no secret was offered: %v", got)
		}
	})

	t.Run("a missing redirect leaves it out", func(t *testing.T) {
		half := complete
		half.RedirectURI = ""

		if got := oauthProviders(config.Config{OAuthMicrosoft: half}); len(got) != 0 {
			t.Fatalf("a provider with no redirect was offered: %v", got)
		}
	})
}

// The three remaining translations, each of which turns a context's refusal
// into something a person reads. A fall-through here is a 500 for a condition
// somebody named, which is the same defect error_translation_test.go guards
// for identity.
func TestTheCandidateRefusalsBecomeApiAnswers(t *testing.T) {
	for name, refusal := range map[string]error{
		"profile":  candidate.ErrCareerContextTooLong,
		"fact":     candidate.ErrFactNotFound,
		"document": candidate.ErrDocumentNotFound,
	} {
		t.Run(name, func(t *testing.T) {
			var translated error
			switch name {
			case "profile":
				translated = translateProfileError(refusal)
			case "fact":
				translated = translateFactError(refusal)
			case "document":
				translated = translateDocumentError(refusal)
			}

			if translated == nil {
				t.Fatal("a refusal became a success")
			}
			if translated == refusal {
				t.Fatalf("%v reaches the client unchanged, so it becomes a 500", refusal)
			}
		})
	}
}

func TestTheCandidateTranslationsLeaveSuccessAlone(t *testing.T) {
	for name, translate := range map[string]func(error) error{
		"profile":  translateProfileError,
		"fact":     translateFactError,
		"document": translateDocumentError,
		"member":   translateMemberError,
	} {
		if got := translate(nil); got != nil {
			t.Fatalf("%s turned success into %v", name, got)
		}
	}
}

func TestAMemberCarriesEveryFieldTheApiNeeds(t *testing.T) {
	mapped := toAPIMember(identity.Member{
		MembershipID: "mid", UserID: "uid", Email: "a@example.com",
		Role: "recruiter", Status: "active", Version: 3,
		CreatedAt: nonZeroTime(),
	})

	dropped := zeroFields(t, reflect.ValueOf(mapped), "identity.Member")

	sort.Strings(dropped)
	if len(dropped) > 0 {
		t.Fatalf("these member fields did not reach the API:\n  %s", strings.Join(dropped, "\n  "))
	}
}

// nonZeroTime is a time no field can hold by accident.
func nonZeroTime() time.Time {
	return time.Date(2026, 8, 30, 9, 0, 0, 0, time.UTC)
}
