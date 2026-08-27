package evaluation_test

import (
	"encoding/json"
	"testing"
)

// Small readers for the shipped artifact's envelope, kept apart from the
// assertions so the test above reads as what it checks.

func unmarshalEnvelope(raw []byte, artifactType *string) error {
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return err
	}
	*artifactType = envelope.Type
	return nil
}

func shippedPolicyBody(t *testing.T, raw []byte) []byte {
	t.Helper()
	var envelope struct {
		Body json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("body: %v", err)
	}
	return envelope.Body
}
