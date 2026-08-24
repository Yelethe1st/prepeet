// Package main reads the durable event contracts and generates the registry
// that producers and consumers share.
//
// ADR-0004 makes JSON Schema the source for events and Go and TypeScript types
// the generated artifacts. This file is the reading half: it turns a directory
// of hand-authored schemas into one catalogue that the generator, the tests and
// the compatibility gate all work from.
//
// The directory is the catalogue. There is no separate index listing which
// events exist, because an index is a second place to edit and therefore a
// place to forget.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Schema is the subset of JSON Schema this generator understands.
//
// A subset deliberately. Supporting all of JSON Schema would mean generating
// types for constructs an event payload has no business using, and every one of
// those is a way for a payload to become something a consumer cannot decode.
// What is not here is refused rather than ignored.
type Schema struct {
	Ref         string             `json:"$ref"`
	ID          string             `json:"$id"`
	Title       string             `json:"title"`
	Description string             `json:"description"`
	Type        string             `json:"type"`
	Format      string             `json:"format"`
	Enum        []string           `json:"enum"`
	Properties  map[string]*Schema `json:"properties"`
	Required    []string           `json:"required"`
	Items       *Schema            `json:"items"`
	// Defs holds shapes referenced by $ref from more than one property. Used by
	// the envelope for the actor; a payload that needs one is usually a payload
	// carrying more than identifiers.
	Defs map[string]*Schema `json:"$defs"`

	// AdditionalProperties is read so that a payload schema which permits
	// anything can be refused. A payload that accepts unknown fields is one
	// whose contract says nothing.
	AdditionalProperties *bool `json:"additionalProperties"`

	// Owner is the bounded context answerable for the event. An annotation
	// rather than a standard keyword, which is what x- prefixes are for.
	Owner string `json:"x-owner"`
	// Consumers is who reads it, recorded so that a proposed removal has a list
	// of people to ask rather than a guess.
	Consumers []string `json:"x-consumers"`
	// Since records the schema_version this field first appeared in. ADR-0004
	// makes additive change bump schema_version within one event type, and
	// without this the bump is a number nobody can check.
	Since string `json:"x-since"`
}

// Event is one entry in the catalogue.
type Event struct {
	// Type is the event type, including its contract version: the only thing a
	// consumer subscribes against.
	Type string
	// SchemaVersion describes the payload's evolution within that contract.
	SchemaVersion string
	Owner         string
	Consumers     []string
	Description   string
	Payload       *Schema
	// Path is where it was read from, for error messages that say which file to
	// open rather than which event is wrong.
	Path string
}

// Catalogue is every event contract, plus the envelope they all share.
type Catalogue struct {
	Envelope *Schema
	Events   map[string]*Event
}

// Sorted returns the events in type order.
//
// Generated output has to be byte-identical between runs or the drift gate
// fails for no reason, and Go map iteration is deliberately random.
func (c *Catalogue) Sorted() []*Event {
	events := make([]*Event, 0, len(c.Events))
	for _, event := range c.Events {
		events = append(events, event)
	}
	sort.Slice(events, func(i, j int) bool { return events[i].Type < events[j].Type })
	return events
}

// Load reads the envelope and every payload schema in dir.
func Load(dir string) (*Catalogue, error) {
	envelope, err := readSchema(filepath.Join(dir, "envelope.schema.json"))
	if err != nil {
		return nil, fmt.Errorf("reading the envelope: %w", err)
	}

	entries, err := os.ReadDir(filepath.Join(dir, "payloads"))
	if err != nil {
		return nil, fmt.Errorf("reading payloads: %w", err)
	}

	catalogue := &Catalogue{Envelope: envelope, Events: map[string]*Event{}}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".schema.json") {
			continue
		}

		path := filepath.Join(dir, "payloads", entry.Name())
		schema, err := readSchema(path)
		if err != nil {
			return nil, err
		}

		// The event type comes from the filename rather than from a field
		// inside, so that two files cannot claim the same event and the one
		// that loads last cannot silently win.
		eventType := strings.TrimSuffix(entry.Name(), ".schema.json")
		if _, duplicate := catalogue.Events[eventType]; duplicate {
			return nil, fmt.Errorf("%s: %s is declared twice", path, eventType)
		}

		catalogue.Events[eventType] = &Event{
			Type:          eventType,
			SchemaVersion: schema.Since,
			Owner:         schema.Owner,
			Consumers:     schema.Consumers,
			Description:   schema.Description,
			Payload:       schema,
			Path:          path,
		}
	}

	return catalogue, nil
}

// readSchema parses one schema file.
//
// Unknown fields are refused rather than ignored. A misspelled keyword that is
// silently dropped is a constraint the author believes is enforced and is not,
// which is worse than no constraint at all.
func readSchema(path string) (*Schema, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(strings.NewReader(string(source)))
	decoder.DisallowUnknownFields()

	var schema Schema
	if err := decoder.Decode(&schema); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &schema, nil
}
