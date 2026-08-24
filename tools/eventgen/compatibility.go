package main

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Break is one incompatible change between two versions of the catalogue.
//
// It carries a remedy as well as a reason, because a gate that says only "this
// is breaking" and stops is a gate people learn to route around. The remedy is
// what turns it into a decision somebody can make.
type Break struct {
	EventType string
	Reason    string
	Remedy    string
}

func (b Break) String() string {
	return fmt.Sprintf("%s: %s\n    %s", b.EventType, b.Reason, b.Remedy)
}

// versionSuffix matches the contract version at the end of an event type.
var versionSuffix = regexp.MustCompile(`^(.*)\.v([0-9]+)$`)

// Compare reports every way the current catalogue would break a consumer built
// against the previous one.
//
// Consumer-facing only. A change that makes life harder for a producer is our
// problem and is caught by the compiler; a change that breaks a consumer is
// somebody else's problem and is caught here or in production. See ADR-0004 for
// what counts as breaking.
//
// Results are sorted so that two runs over the same pair report in the same
// order, which matters when the output is read in a CI log next to a diff.
func Compare(previous, current *Catalogue) []Break {
	var breaks []Break

	for eventType, before := range previous.Events {
		after, present := current.Events[eventType]
		if !present {
			// Retiring a version is allowed once a successor exists, which is
			// the whole reason the version lives in the event type rather than
			// only in schema_version.
			if successor, ok := hasSuccessor(eventType, current); !ok {
				breaks = append(breaks, Break{
					EventType: eventType,
					Reason:    "the event type was removed and no later version replaces it",
					Remedy:    "keep emitting it until every consumer has moved, or add the successor version alongside it",
				})
			} else {
				_ = successor
			}
			continue
		}
		breaks = append(breaks, comparePayloads(eventType, before, after)...)
	}

	sort.Slice(breaks, func(i, j int) bool {
		if breaks[i].EventType != breaks[j].EventType {
			return breaks[i].EventType < breaks[j].EventType
		}
		return breaks[i].Reason < breaks[j].Reason
	})
	return breaks
}

// hasSuccessor reports whether a later contract version of the same event
// exists in the catalogue.
//
// Later, not merely different. A v2 removed while v1 remains is a rollback
// rather than a migration, and a consumer that had already moved to v2 has
// nothing left to receive.
func hasSuccessor(eventType string, catalogue *Catalogue) (string, bool) {
	match := versionSuffix.FindStringSubmatch(eventType)
	if match == nil {
		return "", false
	}
	base, version := match[1], match[2]
	retired, err := strconv.Atoi(version)
	if err != nil {
		return "", false
	}

	for candidate := range catalogue.Events {
		other := versionSuffix.FindStringSubmatch(candidate)
		if other == nil || other[1] != base {
			continue
		}
		if n, err := strconv.Atoi(other[2]); err == nil && n > retired {
			return candidate, true
		}
	}
	return "", false
}

// comparePayloads reports the incompatible differences between two versions of
// one event's payload.
func comparePayloads(eventType string, before, after *Event) []Break {
	var breaks []Break

	previousFields := before.Payload.Properties
	currentFields := after.Payload.Properties

	for name, was := range previousFields {
		is, present := currentFields[name]
		if !present {
			breaks = append(breaks, Break{
				EventType: eventType,
				Reason:    fmt.Sprintf("payload field %q was removed", name),
				Remedy:    "leave it in place and stop populating it, or publish a new contract version",
			})
			continue
		}

		if was.Type != is.Type {
			breaks = append(breaks, Break{
				EventType: eventType,
				Reason:    fmt.Sprintf("payload field %q changed type from %q to %q", name, was.Type, is.Type),
				Remedy:    "add a differently named field instead, or publish a new contract version",
			})
		}

		// The closest mechanical reading of "changing a field's meaning". A
		// consumer with a branch per value silently loses one, and it is the
		// branch nobody has a test for.
		for _, value := range was.Enum {
			if !slices.Contains(is.Enum, value) {
				breaks = append(breaks, Break{
					EventType: eventType,
					Reason:    fmt.Sprintf("payload field %q no longer permits the value %q", name, value),
					Remedy:    "keep the value in the contract and stop emitting it, or publish a new contract version",
				})
			}
		}
	}

	// An addition is additive only if a consumer can tell it happened.
	// schema_version is the only thing it has to tell from, so an addition that
	// does not bump it is a change nobody downstream can notice.
	if added := addedFields(previousFields, currentFields); len(added) > 0 && before.SchemaVersion == after.SchemaVersion {
		breaks = append(breaks, Break{
			EventType: eventType,
			Reason: fmt.Sprintf("payload field(s) %s were added without raising the schema version above %q",
				strings.Join(added, ", "), before.SchemaVersion),
			Remedy: "raise x-since on the schema, which is what a consumer reads to decide what it can handle",
		})
	}

	return breaks
}

// addedFields returns the property names present now and absent before, sorted.
func addedFields(before, after map[string]*Schema) []string {
	var added []string
	for name := range after {
		if _, existed := before[name]; !existed {
			added = append(added, name)
		}
	}
	sort.Strings(added)
	return added
}
