import type { components } from "@contracts";

/**
 * Captions from the durable timeline: RTC-06.
 *
 * The server's record is the record, so captions are the transcript's final
 * segments read back through replay rather than a parallel channel that
 * could disagree with the evidence. A correction replaces the line it
 * names, exactly as the transcript read model does, and the fold is pure so
 * every rule here is a table-driven test rather than a timing test.
 */

type ReplayedEvent =
  components["schemas"]["ControlEventList"]["events"][number];

/** One caption line, keyed by its slot in the timeline. */
export interface CaptionLine {
  /** epoch:sequence, unique for the session's whole life. */
  key: string;
  who: "ai" | "user";
  text: string;
}

/** Where the next replay should start from. */
export interface CaptionCursor {
  epoch: number;
  sequence: number;
}

export interface CaptionState {
  lines: CaptionLine[];
  cursor: CaptionCursor;
}

export const emptyCaptions: CaptionState = {
  lines: [],
  cursor: { epoch: 0, sequence: 0 },
};

/**
 * Folds newly replayed events into the caption state. Finals append in
 * timeline order; a correction replaces the text of the final it names in
 * its own epoch and adds no line of its own; everything else moves the
 * cursor and says nothing.
 */
export function foldCaptions(
  state: CaptionState,
  events: ReplayedEvent[],
): CaptionState {
  if (events.length === 0) {
    return state;
  }
  const lines = [...state.lines];
  let cursor = state.cursor;

  for (const event of events) {
    cursor = { epoch: event.connection_epoch, sequence: event.sequence };
    const payload = event.payload as {
      speaker?: string;
      text?: string;
      supersedes_sequence?: number;
    };

    if (event.type === "transcript.segment.final") {
      lines.push({
        key: `${event.connection_epoch}:${event.sequence}`,
        who: payload.speaker === "candidate" ? "user" : "ai",
        text: payload.text ?? "",
      });
      continue;
    }
    if (event.type === "transcript.segment.corrected") {
      const superseded = `${event.connection_epoch}:${payload.supersedes_sequence ?? 0}`;
      const target = lines.findIndex((line) => line.key === superseded);
      const existing = lines[target];
      if (existing) {
        lines[target] = { ...existing, text: payload.text ?? "" };
      }
    }
  }
  return { lines, cursor };
}
