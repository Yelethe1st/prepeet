// Package password hashes and verifies user passwords.
//
// ADR-0003 decides that authentication is built here rather than bought, which
// makes password handling a standing obligation of this codebase. That is the
// real cost of the decision, and this package is where most of it lives.
//
// argon2id, with no bespoke cryptography anywhere. Every hash carries the
// parameters it was made under, so raising the cost later is a deployment
// rather than an archaeology exercise: old hashes keep verifying and are
// upgraded on the next successful login.
//
// Nothing here ever puts a password into an error, a log or a panic. A password
// that reaches telemetry is a password in the telemetry store, which is exactly
// what SEC-08 scans for.
//
// Implements part of IAM-01.
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Password length bounds.
//
// The maximum is not a policy about password strength. argon2 hashes whatever
// it is given, so an unbounded input is a denial of service: a request with a
// one megabyte password would consume real CPU before being rejected.
const (
	minLength = 1
	maxLength = 1024
)

// ErrInvalidHash means the stored hash could not be parsed.
//
// This is deliberately distinct from a non-match. A wrong password is an
// ordinary event; an unparseable hash means a corrupted or truncated record,
// and reporting it as a wrong password would hide data loss behind a login
// failure nobody investigates.
var ErrInvalidHash = errors.New("password: hash is not a valid argon2id encoding")

// ErrPasswordLength means the password is empty or unreasonably long.
var ErrPasswordLength = fmt.Errorf("password: must be between %d and %d bytes", minLength, maxLength)

// Params are the argon2id cost parameters.
type Params struct {
	// Memory in KiB.
	Memory uint32
	// Iterations is the time cost.
	Iterations uint32
	// Parallelism is the number of lanes.
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

// Default returns the current parameters.
//
// These follow the OWASP argon2id guidance of 64 MiB with two iterations, which
// trades a slower login for a much more expensive offline attack. Raising them
// is safe at any time: existing hashes keep verifying and report NeedsUpgrade.
// Lowering them is a security decision that should not be made quietly, which
// is why a test asserts the floor.
func Default() Params {
	return Params{
		Memory:      64 * 1024,
		Iterations:  2,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	}
}

// Result is the outcome of a verification.
type Result struct {
	// Match is whether the password was correct.
	Match bool
	// NeedsUpgrade is true when the password was correct but the stored hash
	// was made under weaker parameters than Default. The caller should rehash
	// and store the result, which is the only moment the plaintext is available
	// to do so.
	NeedsUpgrade bool
}

// Hash hashes a password under the current parameters.
func Hash(plaintext string) (string, error) {
	return HashWith(plaintext, Default())
}

// HashWith hashes a password under explicit parameters.
//
// It exists so a test can create a hash under old parameters, and so a future
// migration can rehash in bulk. Production code calls Hash.
func HashWith(plaintext string, params Params) (string, error) {
	if len(plaintext) < minLength || len(plaintext) > maxLength {
		// The error names neither the password nor its length: length alone is
		// a small leak, and there is nothing useful the caller does with it.
		return "", ErrPasswordLength
	}

	salt := make([]byte, params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("password: reading salt: %w", err)
	}

	key := argon2.IDKey([]byte(plaintext), salt,
		params.Iterations, params.Memory, params.Parallelism, params.KeyLength)

	return encode(params, salt, key), nil
}

// Verify checks a password against a stored hash.
//
// The comparison is constant time. A variable-time comparison leaks the hash
// one byte at a time to anyone who can measure it, which is a slower but real
// route to forging a session.
func Verify(encoded, plaintext string) (Result, error) {
	params, salt, want, err := decode(encoded)
	if err != nil {
		return Result{}, err
	}

	got := argon2.IDKey([]byte(plaintext), salt,
		params.Iterations, params.Memory, params.Parallelism, params.KeyLength)

	if subtle.ConstantTimeCompare(got, want) != 1 {
		return Result{Match: false}, nil
	}

	return Result{Match: true, NeedsUpgrade: weakerThanDefault(params)}, nil
}

// DummyVerify spends roughly the cost of a real verification and always fails.
//
// Login calls this when the address is unknown. Without it, an unregistered
// address is rejected far faster than a registered one with a wrong password,
// and that timing difference tells an attacker which addresses exist however
// carefully the response body is worded.
func DummyVerify(plaintext string) bool {
	params := Default()
	// A fixed salt is fine here: the output is discarded and never stored. What
	// matters is that the work happens.
	salt := make([]byte, params.SaltLength)
	argon2.IDKey([]byte(plaintext), salt,
		params.Iterations, params.Memory, params.Parallelism, params.KeyLength)
	return false
}

// weakerThanDefault reports whether a hash should be upgraded. Only weaker
// parameters trigger it: a hash made under stronger ones is left alone rather
// than being downgraded to the current default.
func weakerThanDefault(params Params) bool {
	current := Default()
	return params.Memory < current.Memory ||
		params.Iterations < current.Iterations ||
		params.KeyLength < current.KeyLength ||
		params.SaltLength < current.SaltLength
}

// encode renders the PHC string format, which is what every argon2
// implementation reads, so a stored hash is not tied to this codebase.
func encode(params Params, salt, key []byte) string {
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, params.Memory, params.Iterations, params.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key))
}

// decode parses the PHC string format.
//
// Every failure returns the same error. The caller cannot act differently on a
// truncated hash than on a corrupted one, and a detailed parse error would end
// up in a log next to the record it describes.
func decode(encoded string) (Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return Params{}, nil, nil, ErrInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return Params{}, nil, nil, ErrInvalidHash
	}

	var params Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d",
		&params.Memory, &params.Iterations, &params.Parallelism); err != nil {
		return Params{}, nil, nil, ErrInvalidHash
	}
	if params.Memory == 0 || params.Iterations == 0 || params.Parallelism == 0 {
		return Params{}, nil, nil, ErrInvalidHash
	}

	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return Params{}, nil, nil, ErrInvalidHash
	}
	key, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return Params{}, nil, nil, ErrInvalidHash
	}

	params.SaltLength = uint32(len(salt))
	params.KeyLength = uint32(len(key))
	return params, salt, key, nil
}
