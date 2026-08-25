//go:build integration

package interview_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// CAT-04's storage half: the wizard's selection is written whole at creation
// and cannot change afterwards, because a session whose configuration
// drifted after composition would carry a bundle describing choices nobody
// made.

func TestTheSelectionIsStoredWholeAndImmutable(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)

	config := json.RawMessage(`{"discipline":"software-engineering","role":"rl_swe","shape":"shape_technical","minutes":40,"persona":"per_ravi"}`)
	session := interview.Session{
		ID: id.New().String(), Mode: "practice", CandidateID: candidateID,
		BlueprintID: "plan/shape_technical", Config: config,
	}
	if err := store.Create(ctx, session, candidate); err != nil {
		t.Fatalf("create: %v", err)
	}

	created, err := store.Get(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var stored map[string]any
	if err := json.Unmarshal(created.Config, &stored); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if stored["persona"] != "per_ravi" || stored["minutes"] != float64(40) {
		t.Fatalf("config = %s", created.Config)
	}

	// The guard, attacked directly: an update that rewrites config is
	// refused by the trigger whatever role attempts it.
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	_, err = admin.Exec(ctx,
		`UPDATE interview.sessions SET config = '{"persona":"rewritten"}' WHERE id = $1`, session.ID)
	if err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("rewriting config = %v, want the trigger's refusal", err)
	}

	// And the guard is real: drop it, watch the rewrite succeed, restore it.
	if _, err := admin.Exec(ctx, `ALTER TABLE interview.sessions DISABLE TRIGGER sessions_config_immutable`); err != nil {
		t.Fatalf("disabling: %v", err)
	}
	if _, err := admin.Exec(ctx,
		`UPDATE interview.sessions SET config = '{"persona":"rewritten"}' WHERE id = $1`, session.ID); err != nil {
		t.Fatalf("the rewrite still failed with the trigger off; something else is guarding: %v", err)
	}
	if _, err := admin.Exec(ctx, `ALTER TABLE interview.sessions ENABLE TRIGGER sessions_config_immutable`); err != nil {
		t.Fatalf("restoring: %v", err)
	}
	var restored string
	if err := admin.QueryRow(ctx,
		`SELECT config->>'persona' FROM interview.sessions WHERE id = $1`, session.ID).Scan(&restored); err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if restored != "rewritten" {
		t.Fatal("the disabled-trigger write did not land; the proof proved nothing")
	}
	// Put the row back so no later test reads the vandalism.
	if _, err := admin.Exec(ctx, `ALTER TABLE interview.sessions DISABLE TRIGGER sessions_config_immutable`); err != nil {
		t.Fatalf("disabling for repair: %v", err)
	}
	if _, err := admin.Exec(ctx, `UPDATE interview.sessions SET config = $2 WHERE id = $1`, session.ID, config); err != nil {
		t.Fatalf("repairing: %v", err)
	}
	if _, err := admin.Exec(ctx, `ALTER TABLE interview.sessions ENABLE TRIGGER sessions_config_immutable`); err != nil {
		t.Fatalf("restoring after repair: %v", err)
	}
}
