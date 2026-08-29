package main

import (
	"fmt"
	"sort"
	"strings"
)

// Break is one incompatible change between two versions of the document.
//
// It carries a remedy as well as a reason, because a gate that says only "this
// is breaking" and stops is a gate people learn to route around. The remedy is
// what turns it into a decision somebody can make.
type Break struct {
	Where  string
	Reason string
	Remedy string
}

func (b Break) String() string {
	return fmt.Sprintf("%s: %s\n    %s", b.Where, b.Reason, b.Remedy)
}

// Document is as much of an OpenAPI document as this gate reads.
type Document struct {
	raw map[string]any
}

// methods are the operations a path item can carry. Listed rather than
// inferred, so that a key like "parameters" or "summary" is never mistaken for
// an operation somebody removed.
var methods = []string{"get", "put", "post", "delete", "patch", "head", "options"}

// Compare reports every way current would break a client built against previous.
//
// Sorted, so two runs over the same pair report in the same order, which
// matters when the output is read in a CI log beside a diff.
func Compare(previous, current *Document) []Break {
	breaks := []Break{}

	for path, before := range previous.paths() {
		after, kept := current.paths()[path]
		if !kept {
			breaks = append(breaks, Break{
				Where:  path,
				Reason: "the path was removed, so a client still calling it gets a 404",
				Remedy: "keep the path and mark it deprecated until the next major version",
			})
			continue
		}

		for _, method := range methods {
			operation, existed := mapAt(before, method)
			if !existed {
				continue
			}
			replacement, still := mapAt(after, method)
			if !still {
				breaks = append(breaks, Break{
					Where:  strings.ToUpper(method) + " " + path,
					Reason: "the operation was removed, so a client still calling it gets a 405 or a 404",
					Remedy: "keep the operation and mark it deprecated until the next major version",
				})
				continue
			}
			breaks = append(breaks,
				compareRequest(previous, current, strings.ToUpper(method)+" "+path, operation, replacement)...)
			breaks = append(breaks,
				compareResponses(previous, current, strings.ToUpper(method)+" "+path, operation, replacement)...)
		}
	}

	sort.Slice(breaks, func(i, j int) bool {
		if breaks[i].Where != breaks[j].Where {
			return breaks[i].Where < breaks[j].Where
		}
		return breaks[i].Reason < breaks[j].Reason
	})
	return breaks
}

// compareRequest looks for the two ways a request body breaks a client: a
// property it must now send, and a value it may no longer send.
func compareRequest(previous, current *Document, where string, before, after map[string]any) []Break {
	beforeSchema := previous.resolve(bodySchema(before))
	afterSchema := current.resolve(bodySchema(after))
	if beforeSchema == nil || afterSchema == nil {
		return nil
	}

	breaks := []Break{}

	wasRequired := requiredNames(beforeSchema)
	for _, name := range sortedNames(requiredNames(afterSchema)) {
		if !wasRequired[name] {
			breaks = append(breaks, Break{
				Where:  where,
				Reason: fmt.Sprintf("request property %q is now required, so an existing client's request is rejected", name),
				Remedy: "make it optional with a default, or take it in a new operation",
			})
		}
	}

	// A value the client may no longer send. Removing one from a request enum
	// rejects a request that used to work, which is the same break as a new
	// required field wearing a different hat.
	for name, beforeProperty := range properties(previous, beforeSchema) {
		afterProperty, kept := properties(current, afterSchema)[name]
		if !kept {
			continue
		}
		for _, value := range removedEnumValues(beforeProperty, afterProperty) {
			breaks = append(breaks, Break{
				Where:  where,
				Reason: fmt.Sprintf("request property %q no longer accepts %q", name, value),
				Remedy: "keep accepting it and ignore it, or reject it in a new version of the operation",
			})
		}
		if from, to, changed := changedType(beforeProperty, afterProperty); changed {
			breaks = append(breaks, Break{
				Where:  where,
				Reason: fmt.Sprintf("request property %q changed from %s to %s", name, from, to),
				Remedy: "add a new property of the new type and keep the old one",
			})
		}
	}
	return breaks
}

// compareResponses looks for what a client reads and would stop finding.
func compareResponses(previous, current *Document, where string, before, after map[string]any) []Break {
	beforeResponses, ok := mapAt(before, "responses")
	if !ok {
		return nil
	}
	afterResponses, ok := mapAt(after, "responses")
	if !ok {
		return nil
	}

	breaks := []Break{}
	for status, beforeResponse := range beforeResponses {
		// Only success bodies. An error shape is read by people far more than
		// by code, and the error envelope has its own contract test.
		if !strings.HasPrefix(status, "2") {
			continue
		}
		beforeMap, ok := beforeResponse.(map[string]any)
		if !ok {
			continue
		}
		afterMap, ok := mapAt(afterResponses, status)
		if !ok {
			breaks = append(breaks, Break{
				Where:  where,
				Reason: fmt.Sprintf("the %s response was removed, so a client expecting it has no branch left", status),
				Remedy: "keep the status and change the body in a new operation instead",
			})
			continue
		}

		beforeSchema := previous.resolve(bodySchema(beforeMap))
		afterSchema := current.resolve(bodySchema(afterMap))
		if beforeSchema == nil || afterSchema == nil {
			continue
		}

		nowRequired := requiredNames(afterSchema)
		afterProperties := properties(current, afterSchema)
		for _, name := range sortedNames(requiredNames(beforeSchema)) {
			if _, present := afterProperties[name]; !present {
				breaks = append(breaks, Break{
					Where:  where,
					Reason: fmt.Sprintf("response property %q was removed, and it was one a client could rely on", name),
					Remedy: "keep sending it, empty if it no longer means anything",
				})
				continue
			}
			if !nowRequired[name] {
				breaks = append(breaks, Break{
					Where:  where,
					Reason: fmt.Sprintf("response property %q is no longer guaranteed, so a client reading it may find nothing", name),
					Remedy: "keep it required, or version the operation",
				})
			}
		}

		for name, beforeProperty := range properties(previous, beforeSchema) {
			afterProperty, kept := afterProperties[name]
			if !kept {
				continue
			}
			if from, to, changed := changedType(beforeProperty, afterProperty); changed {
				breaks = append(breaks, Break{
					Where:  where,
					Reason: fmt.Sprintf("response property %q changed from %s to %s", name, from, to),
					Remedy: "add a new property of the new type and keep the old one",
				})
			}
		}
	}
	return breaks
}
