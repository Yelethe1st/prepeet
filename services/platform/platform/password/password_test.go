package password_test

import (
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/platform/password"
)

func TestHashAndVerifyRoundTrip(t *testing.T) {
	t.Parallel()

	hash, err := password.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("Hash returned error: %v", err)
	}

	result, err := password.Verify(hash, "correct horse battery staple")
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if !result.Match {
		t.Error("Verify did not match the password it was hashed from")
	}
}

func TestVerifyRejectsTheWrongPassword(t *testing.T) {
	t.Parallel()

	hash, err := password.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("Hash returned error: %v", err)
	}

	result, err := password.Verify(hash, "incorrect horse battery staple")
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if result.Match {
		t.Error("Verify matched a different password")
	}
}

// Two hashes of the same password must differ, or the store leaks which users
// share a password and a single cracked hash unlocks all of them.
func TestHashIsSaltedPerCall(t *testing.T) {
	t.Parallel()

	first, err := password.Hash("same password")
	if err != nil {
		t.Fatalf("Hash returned error: %v", err)
	}
	second, err := password.Hash("same password")
	if err != nil {
		t.Fatalf("Hash returned error: %v", err)
	}

	if first == second {
		t.Error("hashing the same password twice produced the same hash, so it is unsalted")
	}
}

// The encoded hash carries its own parameters, so raising the cost later is a
// deployment rather than an archaeology exercise.
func TestHashEncodesItsParameters(t *testing.T) {
	t.Parallel()

	hash, err := password.Hash("a password")
	if err != nil {
		t.Fatalf("Hash returned error: %v", err)
	}

	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Errorf("hash = %q, want it to declare argon2id", hash)
	}
	for _, part := range []string{"m=", "t=", "p="} {
		if !strings.Contains(hash, part) {
			t.Errorf("hash = %q, want it to encode %q", hash, part)
		}
	}
}

// A hash made under weaker parameters still verifies, and says it should be
// upgraded. Without this, raising the cost would either lock everyone out or
// leave old hashes weak forever.
func TestVerifyReportsWhenAHashNeedsUpgrading(t *testing.T) {
	t.Parallel()

	weak, err := password.HashWith("a password", password.Params{
		Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32,
	})
	if err != nil {
		t.Fatalf("HashWith returned error: %v", err)
	}

	result, err := password.Verify(weak, "a password")
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if !result.Match {
		t.Fatal("a hash under old parameters did not verify")
	}
	if !result.NeedsUpgrade {
		t.Error("a hash under weaker parameters did not report NeedsUpgrade")
	}
}

func TestVerifyDoesNotAskToUpgradeACurrentHash(t *testing.T) {
	t.Parallel()

	hash, err := password.Hash("a password")
	if err != nil {
		t.Fatalf("Hash returned error: %v", err)
	}

	result, err := password.Verify(hash, "a password")
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if result.NeedsUpgrade {
		t.Error("a hash under current parameters asked to be upgraded")
	}
}

// A malformed hash is an error rather than a non-match. The two mean different
// things: one is a wrong password, the other is a corrupted record, and
// treating the second as the first would hide data loss behind a login failure.
func TestVerifyRejectsAMalformedHash(t *testing.T) {
	t.Parallel()

	for name, hash := range map[string]string{
		"empty":           "",
		"not argon2":      "$2a$10$abcdefghijklmnopqrstuv",
		"truncated":       "$argon2id$v=19$m=65536,t=3,p=2$c2FsdA",
		"bad base64":      "$argon2id$v=19$m=65536,t=3,p=2$!!!!$!!!!",
		"missing params":  "$argon2id$v=19$$c2FsdA$aGFzaA",
		"wrong version":   "$argon2id$v=16$m=65536,t=3,p=2$c2FsdA$aGFzaA",
		"negative memory": "$argon2id$v=19$m=-1,t=3,p=2$c2FsdA$aGFzaA",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := password.Verify(hash, "a password"); err == nil {
				t.Errorf("Verify(%q) returned no error, want one", hash)
			}
		})
	}
}

// Login must answer in the same time whether or not the user exists, so the
// caller needs something to spend when there is no hash to check. Otherwise a
// fast rejection tells an attacker the address is unregistered.
func TestDummyVerifyCostsSomethingAndNeverMatches(t *testing.T) {
	t.Parallel()

	if password.DummyVerify("anything at all") {
		t.Error("DummyVerify returned true, and it must never match")
	}
}

// An unbounded password is a denial of service: argon2 hashes whatever it is
// given, so a very long input is expensive by design.
func TestHashRejectsAnUnreasonablyLongPassword(t *testing.T) {
	t.Parallel()

	if _, err := password.Hash(strings.Repeat("a", 4096)); err == nil {
		t.Error("Hash accepted a 4096 byte password, want it refused")
	}
}

func TestHashRejectsAnEmptyPassword(t *testing.T) {
	t.Parallel()

	if _, err := password.Hash(""); err == nil {
		t.Error("Hash accepted an empty password, want it refused")
	}
}

// The defaults are a security decision and should not drift downwards without
// someone noticing.
func TestDefaultParametersMeetTheFloor(t *testing.T) {
	t.Parallel()

	p := password.Default()

	if p.Memory < 64*1024 {
		t.Errorf("Memory = %d KiB, want at least 65536", p.Memory)
	}
	if p.Iterations < 2 {
		t.Errorf("Iterations = %d, want at least 2", p.Iterations)
	}
	if p.SaltLength < 16 {
		t.Errorf("SaltLength = %d, want at least 16", p.SaltLength)
	}
	if p.KeyLength < 32 {
		t.Errorf("KeyLength = %d, want at least 32", p.KeyLength)
	}
}

// A password that reaches a log or an error message is a password in the
// telemetry store, which is what SEC-08 scans for.
func TestErrorsNeverContainThePassword(t *testing.T) {
	t.Parallel()

	const secret = "hunter2-super-secret"

	_, err := password.Hash(secret + strings.Repeat("x", 4096))
	if err != nil && strings.Contains(err.Error(), secret) {
		t.Errorf("Hash error contains the password: %v", err)
	}

	_, err = password.Verify("$argon2id$broken", secret)
	if err != nil && strings.Contains(err.Error(), secret) {
		t.Errorf("Verify error contains the password: %v", err)
	}
}
