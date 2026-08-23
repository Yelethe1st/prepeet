// Package id generates the opaque, time ordered identifiers the public API
// contract requires.
//
// docs/contracts/public-api.md fixes two properties. Identifiers are UUIDv7, so
// they sort by creation time and do not fragment a primary key index on insert.
// Identifiers are opaque, so tenant is never inferred from one: an identifier
// says what a resource is, never who may read it. Authorization is decided by
// the policy layer against the active tenant, never by parsing an identifier.
//
// Implements part of PLT-01.
package id

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// UUID is a 128 bit RFC 9562 version 7 identifier.
type UUID [16]byte

// ErrInvalid is returned by Parse when the input is not a canonical UUIDv7.
var ErrInvalid = errors.New("id: not a canonical UUIDv7")

// clock guards the monotonic counter that keeps identifiers ordered when two
// are generated inside the same millisecond.
var clock struct {
	sync.Mutex
	lastMillis int64
	sequence   uint16
}

// New returns a new UUIDv7.
//
// The layout is 48 bits of Unix milliseconds, then the version nibble, then a
// 12 bit sequence, then the variant bits and 62 bits of randomness. The
// sequence is what makes two identifiers generated in the same millisecond
// still sort in creation order; without it, ordering inside a millisecond would
// be random and the time ordering guarantee would only hold at millisecond
// resolution.
func New() UUID {
	now := time.Now().UnixMilli()

	clock.Lock()
	switch {
	case now > clock.lastMillis:
		clock.lastMillis = now
		clock.sequence = 0
	case now == clock.lastMillis:
		clock.sequence++
		// A full sequence within one millisecond borrows from the next, rather
		// than wrapping and breaking the ordering guarantee.
		if clock.sequence > 0x0FFF {
			clock.lastMillis++
			now = clock.lastMillis
			clock.sequence = 0
		}
	default:
		// The wall clock moved backwards. Keep issuing from the last value we
		// used so ordering survives a clock correction.
		clock.lastMillis++
		now = clock.lastMillis
		clock.sequence = 0
	}
	millis, sequence := clock.lastMillis, clock.sequence
	clock.Unlock()

	var u UUID
	u[0] = byte(millis >> 40)
	u[1] = byte(millis >> 32)
	u[2] = byte(millis >> 24)
	u[3] = byte(millis >> 16)
	u[4] = byte(millis >> 8)
	u[5] = byte(millis)

	// Version 7 in the high nibble of byte 6, sequence in the remaining 12 bits.
	u[6] = 0x70 | byte(sequence>>8&0x0F)
	u[7] = byte(sequence)

	// rand.Read is documented never to fail, and panicking is the correct
	// response if it somehow does: an identifier without entropy is guessable,
	// and a guessable identifier is a security problem rather than an outage.
	if _, err := rand.Read(u[8:]); err != nil {
		panic(fmt.Sprintf("id: crypto/rand unavailable: %v", err))
	}

	// RFC 4122 variant in the top two bits of byte 8.
	u[8] = (u[8] & 0x3F) | 0x80

	return u
}

// String returns the canonical 8-4-4-4-12 hexadecimal form.
func (u UUID) String() string {
	var buf [36]byte
	hex.Encode(buf[0:8], u[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], u[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], u[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], u[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], u[10:16])
	return string(buf[:])
}

// Prefixed returns an identifier carrying a short type prefix, such as
// "ses_0190a1b2c3d4...". The prefix is for humans reading logs and support
// tickets. It is not a namespace and it is never an authorization input.
func Prefixed(prefix string) string {
	u := New()
	return prefix + "_" + hex.EncodeToString(u[:])
}

// Parse reads a canonical UUIDv7 string.
//
// It rejects any other UUID version and any non RFC 4122 variant, so a caller
// cannot smuggle in a random version 4 value where a time ordered identifier is
// required.
func Parse(s string) (UUID, error) {
	if len(s) != 36 {
		return UUID{}, ErrInvalid
	}
	if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return UUID{}, ErrInvalid
	}

	var u UUID
	for _, group := range [][3]int{{0, 8, 0}, {9, 13, 4}, {14, 18, 6}, {19, 23, 8}, {24, 36, 10}} {
		if _, err := hex.Decode(u[group[2]:], []byte(s[group[0]:group[1]])); err != nil {
			return UUID{}, ErrInvalid
		}
	}

	if u[6]>>4 != 7 {
		return UUID{}, ErrInvalid
	}
	if u[8]&0xC0 != 0x80 {
		return UUID{}, ErrInvalid
	}
	return u, nil
}
