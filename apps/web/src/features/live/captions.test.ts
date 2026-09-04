import { describe, expect, it } from "vitest";

import { emptyCaptions, foldCaptions } from "./captions";

/**
 * The caption fold: the transcript's finals in timeline order, corrections
 * replacing the line they name, and everything else only moving the cursor.
 */

function replayed(
  epoch: number,
  sequence: number,
  type: string,
  payload: Record<string, unknown> = {},
) {
  return {
    event_id: `evt-${epoch}-${sequence}`,
    connection_epoch: epoch,
    sequence,
    type,
    payload,
    occurred_at: "2026-09-05T10:00:00Z",
  };
}

describe("folding replayed events into captions", () => {
  it("appends finals in timeline order with the speaker named", () => {
    const state = foldCaptions(emptyCaptions, [
      replayed(1, 1, "connection.established"),
      replayed(1, 2, "transcript.segment.final", {
        speaker: "interviewer",
        text: "Tell me about a system you built.",
      }),
      replayed(1, 3, "transcript.segment.final", {
        speaker: "candidate",
        text: "I led the booking service rewrite.",
      }),
    ]);

    expect(state.lines).toEqual([
      { key: "1:2", who: "ai", text: "Tell me about a system you built." },
      { key: "1:3", who: "user", text: "I led the booking service rewrite." },
    ]);
    expect(state.cursor).toEqual({ epoch: 1, sequence: 3 });
  });

  it("a correction replaces the line it names and adds none of its own", () => {
    const first = foldCaptions(emptyCaptions, [
      replayed(1, 2, "transcript.segment.final", {
        speaker: "candidate",
        text: "we used a unique restraint",
      }),
    ]);

    const corrected = foldCaptions(first, [
      replayed(1, 4, "transcript.segment.corrected", {
        speaker: "candidate",
        text: "we used a unique constraint",
        supersedes_sequence: 2,
      }),
    ]);

    expect(corrected.lines).toEqual([
      { key: "1:2", who: "user", text: "we used a unique constraint" },
    ]);
    expect(corrected.cursor).toEqual({ epoch: 1, sequence: 4 });
  });

  it("a correction for a line replay has not delivered yet is quietly held for it", () => {
    // The orphan case the transcript read model lists: the fold does not
    // invent a line for it, and the cursor still advances so polling does
    // not loop on it.
    const state = foldCaptions(emptyCaptions, [
      replayed(1, 4, "transcript.segment.corrected", {
        speaker: "candidate",
        text: "corrected text",
        supersedes_sequence: 2,
      }),
    ]);

    expect(state.lines).toEqual([]);
    expect(state.cursor).toEqual({ epoch: 1, sequence: 4 });
  });

  it("lines survive across epochs and no event type is misread as a caption", () => {
    const before = foldCaptions(emptyCaptions, [
      replayed(1, 2, "transcript.segment.final", {
        speaker: "interviewer",
        text: "What happens when the database is the bottleneck?",
      }),
      replayed(1, 3, "connection.lost"),
    ]);
    const after = foldCaptions(before, [
      replayed(2, 1, "interruption", {
        cause: "connection_lost",
        duration_seconds: 12,
      }),
      replayed(2, 2, "transcript.segment.final", {
        speaker: "candidate",
        text: "Then I shard by clinic.",
      }),
    ]);

    expect(after.lines.map((line) => line.key)).toEqual(["1:2", "2:2"]);
    expect(after.cursor).toEqual({ epoch: 2, sequence: 2 });
  });

  it("no events means the same state back", () => {
    expect(foldCaptions(emptyCaptions, [])).toBe(emptyCaptions);
  });
});
