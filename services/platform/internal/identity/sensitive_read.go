package identity

import (
	"context"
	"fmt"
)

// SensitiveRead is one recorded access to restricted content.
//
// Identity records it because the question the row answers is about an actor:
// who reached for a transcript, under which workspace, and whether they got it.
// The audit schema is shared by every module, and this is the one write that
// belongs to whoever established the identity in the first place.
type SensitiveRead struct {
	ActorID     string
	TenantID    string
	Action      string
	SubjectType string
	SubjectID   string
	Outcome     string
	RequestID   string
}

// RecordSensitiveRead writes the access record.
//
// It uses the pool directly rather than a caller's transaction. The read it
// describes is not part of any write the caller is making, and joining it to
// one would mean a rolled back request erased the record of an access that had
// already been served.
func (s *Service) RecordSensitiveRead(ctx context.Context, read SensitiveRead) error {
	if read.Action == "" || read.Outcome == "" {
		// A row that cannot say what happened is worse than none: it occupies
		// the space where the answer should be.
		return fmt.Errorf("identity: a sensitive read needs an action and an outcome")
	}
	return s.repo.RecordSensitiveRead(ctx, read)
}
