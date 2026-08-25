import type { components } from "@contracts";

/**
 * The pure rules of the review surface: which CV is current, how a fact
 * reads as text, and what a correction carries back to the server.
 *
 * Extraction is assistive, never authoritative, and these rules are where the
 * interface keeps that promise: the correction always wins the reading, and a
 * correction sent back is the candidate's whole version of the value - the
 * extractor's confidence deliberately not among it, because a person is not
 * 0.8 sure of their own job title.
 */

export type Document = components["schemas"]["Document"];
export type Fact = components["schemas"]["Fact"];
export type FactKind = Fact["kind"];
export type FactStatus = Fact["status"];

/**
 * The CV the profile shows: the newest stored version. Deleted and failed
 * versions stay in history but are nobody's current document.
 */
export function currentDocument(
  documents: readonly Document[],
): Document | undefined {
  return documents
    .filter((document) => document.state === "stored")
    .sort((a, b) => b.version - a.version)[0];
}

/** The field that carries each kind's reading, and that an edit rewrites. */
export function primaryField(kind: FactKind): string {
  switch (kind) {
    case "role":
      return "title";
    case "skill":
      return "name";
    case "date_range":
      return "start";
    default:
      return "text";
  }
}

/**
 * How a fact reads as one line of text. The correction wins where one exists,
 * because from the moment the candidate speaks, theirs is the version
 * everything downstream uses.
 */
export function factText(fact: Fact): string {
  const value = (fact.corrected_value ?? fact.value) as Record<string, unknown>;
  if (fact.kind === "date_range") {
    const start = String(value.start ?? "");
    const end = String(value.end ?? "");
    return end ? `${start} to ${end}` : start;
  }
  return String(value[primaryField(fact.kind)] ?? "");
}

/**
 * The corrected value an edit sends: the version being edited, with the
 * primary field replaced and the extractor's confidence removed. The original
 * extraction is never in this payload - the server keeps it untouched beside
 * the correction.
 */
export function correctionFor(
  fact: Fact,
  text: string,
): Record<string, unknown> {
  const base = {
    ...((fact.corrected_value ?? fact.value) as Record<string, unknown>),
  };
  delete base.confidence;
  base[primaryField(fact.kind)] = text;
  return base;
}

/** The review states, named as the candidate's own acts. */
export function statusLabel(status: FactStatus): string {
  switch (status) {
    case "proposed":
      return "Parsed";
    case "confirmed":
      return "Confirmed by you";
    case "corrected":
      return "Edited by you";
    case "rejected":
      return "Rejected by you";
  }
}

/** Where the fact came from, in the CV's own coordinates. */
export function spanSentence(fact: Fact): string {
  return `Characters ${fact.span_start} to ${fact.span_end} of your CV`;
}
