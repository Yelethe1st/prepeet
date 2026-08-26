package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

// startRecording turns interview.session_started.v1 into egress: RTC-05's
// capture half, in the one place that may see both the interview context
// and the SFU. Transcript-only sessions return before any recorder call -
// egress never starts, so durable audio never exists (ADR-0013).
//
// The outbox retries this handler, and StartRecording converges on the
// unique (session, track) row, so a redelivery or a reconnection never
// starts a second recording.
func startRecording(sessions *interview.Store, recorder interview.Recorder) outbox.HandlerFunc {
	return func(ctx context.Context, event outbox.Pending) error {
		var payload struct {
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("decoding %s: %w", event.Type, err)
		}

		session, err := sessions.Get(ctx, payload.SessionID, event.Purpose, event.Actor.ID, event.TenantID)
		if err != nil {
			return err
		}
		// StartRecording is the preference gate: transcript_only returns
		// before any recorder call, so egress never exists to clean up.
		return sessions.StartRecording(ctx, recorder, session)
	}
}
