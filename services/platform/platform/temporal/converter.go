// Package temporal owns the connection to Temporal and the rules about what may
// travel through it.
//
// Durable execution is decided in ADR-0007: self-hosted, in region, on a
// database instance of its own. This package is the seam that decision rests
// on. Address, namespace and credentials are configuration and the client is
// built in exactly one place, so moving to Temporal Cloud is a configuration
// change plus paperwork rather than a rewrite.
//
// It deliberately does not abstract durable execution itself. Workflow code is
// written against the Temporal SDK and no interface hides that honestly; an
// abstraction over it would be a worse Temporal with a smaller test suite. What
// is behind a seam is the client, so a bounded context can start a workflow
// without importing the SDK, per ADR-0005.
//
// Implements part of PLT-06.
package temporal

import (
	"fmt"

	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/sdk/converter"

	"github.com/Yelethe1st/prepeet/services/platform/platform/telemetry"
)

// MaxPayloadBytes is the largest encoded value that may enter workflow history.
//
// Four kilobytes. A workflow argument built from identifiers and control values
// is a few hundred bytes, so this is generous for what is allowed and far below
// anything that could be a transcript, a CV or a model response.
//
// The cap is blunt and that is why it works. A rule phrased as "do not pass
// content" is a judgement somebody makes at two in the morning while trying to
// get an evaluation to run. A size is not.
const MaxPayloadBytes = 4096

// NewDataConverter returns the converter every client and worker must use.
//
// It enforces ADR-0007's payload rule on the encode path. Workflow history is
// durable storage on its own retention schedule, outside the deletion machinery
// that governs the tables, so an activity taking a transcript as an argument
// creates a second copy of it that a deletion request cannot reach.
//
// Failing at encode time is the right failure. The workflow does not start, the
// error names the call site, and nothing is stored. The alternative, discovering
// it later, means the content is already in history with a month to run.
func NewDataConverter() converter.DataConverter {
	return &restrictedConverter{inner: converter.GetDefaultDataConverter()}
}

// restrictedConverter wraps the default converter and refuses what must not be
// stored.
//
// Only the encode direction is filtered. Decoding stays unrestricted on
// purpose: history written before a rule tightened must remain readable, or
// changing the rule breaks every in-flight workflow, and refusing on the way out
// protects nothing because the content is already stored. Refusal belongs where
// it can still prevent something.
type restrictedConverter struct {
	inner converter.DataConverter
}

func (c *restrictedConverter) ToPayload(value any) (*commonpb.Payload, error) {
	payload, err := c.inner.ToPayload(value)
	if err != nil {
		return nil, err
	}
	if err := permitted(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// ToPayloads refuses the whole call if any one argument is refused.
//
// Anything else would start a workflow with an argument silently missing, which
// is a worse outcome than not starting it.
func (c *restrictedConverter) ToPayloads(values ...any) (*commonpb.Payloads, error) {
	payloads, err := c.inner.ToPayloads(values...)
	if err != nil {
		return nil, err
	}
	for i, payload := range payloads.GetPayloads() {
		if err := permitted(payload); err != nil {
			return nil, fmt.Errorf("argument %d: %w", i, err)
		}
	}
	return payloads, nil
}

func (c *restrictedConverter) FromPayload(payload *commonpb.Payload, valuePtr any) error {
	return c.inner.FromPayload(payload, valuePtr)
}

func (c *restrictedConverter) FromPayloads(payloads *commonpb.Payloads, valuePtrs ...any) error {
	return c.inner.FromPayloads(payloads, valuePtrs...)
}

func (c *restrictedConverter) ToString(payload *commonpb.Payload) string {
	return c.inner.ToString(payload)
}

func (c *restrictedConverter) ToStrings(payloads *commonpb.Payloads) []string {
	return c.inner.ToStrings(payloads)
}

// permitted reports whether an encoded payload may enter workflow history.
//
// Two checks, catching different things. The size catches bulk content, which is
// what a transcript, a CV or a model response looks like whatever field it
// arrived in. The shape scan catches things that are restricted regardless of
// length, which a contact address is.
//
// Neither error quotes what it found. An error message naming the address it
// refused would carry it straight into the log the refusal was protecting.
func permitted(payload *commonpb.Payload) error {
	data := payload.GetData()

	if len(data) > MaxPayloadBytes {
		return fmt.Errorf("temporal: a payload of %d bytes exceeds the %d byte limit; "+
			"a workflow carries identifiers and the activity reads what it needs from the database, "+
			"per ADR-0007", len(data), MaxPayloadBytes)
	}

	if shape, found := telemetry.FindRestricted(string(data)); found {
		return fmt.Errorf("temporal: a payload carries %s, which may not enter workflow history; "+
			"pass an identifier instead, per ADR-0007", shape)
	}
	return nil
}
