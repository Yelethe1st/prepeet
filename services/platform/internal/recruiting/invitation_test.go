package recruiting_test

import (
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/internal/recruiting"
)

// The status a recruiter reads is computed, not stored, so a live invitation
// whose expiry has passed reads expired without anything flipping a column,
// and a terminal outcome always wins over the clock.
func TestInvitationStatusComputesExpiryFromTime(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	ended := now.Add(-time.Hour)

	cases := []struct {
		name     string
		inv      recruiting.Invitation
		want     string
		wantLive bool
	}{
		{
			name: "live within the window",
			inv:  recruiting.Invitation{ExpiresAt: now.Add(time.Hour)},
			want: "live", wantLive: true,
		},
		{
			name: "live but past expiry reads expired",
			inv:  recruiting.Invitation{ExpiresAt: now.Add(-time.Minute)},
			want: "expired", wantLive: false,
		},
		{
			name: "revoked wins over an expiry that also passed",
			inv: recruiting.Invitation{
				ExpiresAt: now.Add(-time.Minute),
				Outcome:   recruiting.InvitationRevoked, OutcomeAt: &ended,
			},
			want: "revoked", wantLive: false,
		},
		{
			name: "accepted reads accepted while still inside the window",
			inv: recruiting.Invitation{
				ExpiresAt: now.Add(time.Hour),
				Outcome:   recruiting.InvitationAccepted, OutcomeAt: &ended,
			},
			want: "accepted", wantLive: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.inv.Status(now); got != tc.want {
				t.Fatalf("Status = %q, want %q", got, tc.want)
			}
			if got := tc.inv.Live(now); got != tc.wantLive {
				t.Fatalf("Live = %v, want %v", got, tc.wantLive)
			}
		})
	}
}

// An expiry measured in days, because an interview invitation is answered
// around a life and not in the one sitting a verification link assumes. The
// constant is asserted so a change to it is a deliberate edit to a test, not a
// silent shortening of every candidate's window.
func TestInvitationExpiryIsMeasuredInDays(t *testing.T) {
	if recruiting.InvitationExpiry < 24*time.Hour {
		t.Fatalf("invitation expiry %v is under a day", recruiting.InvitationExpiry)
	}
	if recruiting.InvitationExpiry != 7*24*time.Hour {
		t.Fatalf("invitation expiry is %v, expected seven days", recruiting.InvitationExpiry)
	}
}
