/**
 * The analysis document's shape, as the calculators serve it.
 *
 * The contract types the document as an open object because its shape is
 * the calculator's (named in calculation_version); this is the reading
 * side's declaration of what articulation-features-v1 and its profile and
 * coaching put there, with every field optional so an older or newer
 * document renders what it has rather than crashing.
 */

export interface TurnFeatures {
  sequence: number;
  words: number;
  duration_ms: number;
  words_per_minute: number;
  pause_count: number;
  long_pause_count: number;
  max_pause_ms: number;
  filler_count: number;
  fillers_per_100_words: number;
  restart_count: number;
  repeated_phrase_count: number;
  transcript_confidence: number;
  status: string;
  warnings: string[];
}

export interface Dimension {
  level: string;
  evidence_sequences: number[];
  reason: string;
}

export interface Priority {
  dimension: string;
  level: string;
  listener_impact: string;
  action: string;
  evidence_sequences: number[];
  drill: string;
}

export interface ShapePart {
  slot: string;
  kind: "quote" | "placeholder";
  text: string;
  sequence: number | null;
}

export interface Analysis {
  assessability?: {
    status?: string;
    warnings?: string[];
    note?: string;
    transcript_confidence?: number;
  };
  metrics?: {
    words?: number;
    words_per_minute?: number;
    fillers_per_100_words?: number;
    long_pause_count?: number;
  };
  turns?: TurnFeatures[];
  profile?: {
    profile_version?: string;
    dimensions?: Record<string, Dimension>;
  };
  coaching?:
    | {
        coaching_version?: string;
        priorities?: Priority[];
        suggested_shape?: ShapePart[];
      }
    | { available: false; note: string };
}

/** The eight delivery drills, from practice-mode.md, keyed as the coach names them. */
export const DRILLS: {
  key: string;
  title: string;
  minutes: number;
  how: string;
}[] = [
  {
    key: "headline_first",
    title: "Headline first",
    minutes: 5,
    how: "Answer one question by saying the decision or result in your first sentence, then the reasoning.",
  },
  {
    key: "sixty_second_compression",
    title: "Sixty-second compression",
    minutes: 6,
    how: "Give a full answer in under a minute, then again in thirty seconds, keeping the outcome.",
  },
  {
    key: "deliberate_pause",
    title: "Deliberate pause instead of filler",
    minutes: 5,
    how: "Answer while pausing silently wherever you would say a filler; count the pauses afterwards.",
  },
  {
    key: "star_compression",
    title: "STAR compression",
    minutes: 6,
    how: "Situation, task, action, result: one sentence each, spoken aloud.",
  },
  {
    key: "signposting",
    title: "Signposting",
    minutes: 4,
    how: "Answer using first, then, and as a result, once each, out loud.",
  },
  {
    key: "concrete_language",
    title: "Concrete-language substitution",
    minutes: 5,
    how: "Retell an answer replacing every vague phrase with a number, a name or a date you can stand behind.",
  },
  {
    key: "one_example",
    title: "One-example constraint",
    minutes: 5,
    how: "Answer with exactly one example, fully, rather than three partly.",
  },
  {
    key: "playback_and_redo",
    title: "Playback and redo",
    minutes: 8,
    how: "Read the transcript of one answer, then answer it again and compare.",
  },
];

/** mm:ss on the session's own clock. */
export function clock(ms: number): string {
  const total = Math.floor(ms / 1000);
  return `${String(Math.floor(total / 60)).padStart(2, "0")}:${String(total % 60).padStart(2, "0")}`;
}

/** A human title from a snake_case dimension or drill key. */
export function titleOf(key: string): string {
  const words = key.replaceAll("_", " ");
  return words.charAt(0).toUpperCase() + words.slice(1);
}

/** The text summary a pace chart owes its reader (WEB-04, A11Y). */
export function paceSummary(turns: TurnFeatures[]): string {
  const measured = turns.filter((t) => t.status === "assessable");
  if (measured.length === 0) {
    return "No answer was long enough to measure a speaking rate.";
  }
  const rates = measured.map((t) => t.words_per_minute);
  const fastest = measured.reduce((a, b) =>
    a.words_per_minute >= b.words_per_minute ? a : b,
  );
  const pauses = measured.reduce((n, t) => n + t.long_pause_count, 0);
  return `You spoke between ${Math.min(...rates)} and ${Math.max(...rates)} words per minute across ${measured.length} measured answer${measured.length === 1 ? "" : "s"}. The answer at turn ${fastest.sequence} was the fastest. ${pauses} pause${pauses === 1 ? "" : "s"} of 700 ms or more were counted in total.`;
}
