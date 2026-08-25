import { describe, expect, it } from "vitest";

import {
  correctionFor,
  currentDocument,
  factText,
  primaryField,
  spanSentence,
  statusLabel,
  type Document,
  type Fact,
} from "./facts";

/**
 * The pure half of the review surface: which document is current, how a fact
 * reads as text, and what a correction carries back. Pinned separately from
 * the component because these rules decide what the candidate believes about
 * their own data, and a rendering test would let them drift under a css
 * change.
 */

const doc = (overrides: Partial<Document>): Document =>
  ({
    id: "d1",
    kind: "cv",
    version: 1,
    media_type: "application/pdf",
    size_bytes: 10,
    state: "stored",
    extraction_state: "extracted",
    created_at: "2026-08-25T10:00:00Z",
    ...overrides,
  }) as Document;

const fact = (overrides: Partial<Fact>): Fact =>
  ({
    id: "f1",
    document_id: "d1",
    kind: "skill",
    value: { name: "Go", confidence: 0.8 },
    span_start: 80,
    span_end: 82,
    confidence: 0.8,
    extractor_version: "extract-1",
    status: "proposed",
    created_at: "2026-08-25T10:00:00Z",
    ...overrides,
  }) as Fact;

describe("currentDocument", () => {
  it("is the newest stored version, not a deleted or failed one", () => {
    const documents = [
      doc({ id: "d3", version: 3, state: "failed" }),
      doc({ id: "d2", version: 2, state: "stored" }),
      doc({ id: "d1", version: 1, state: "deleted" }),
    ];

    expect(currentDocument(documents)?.id).toBe("d2");
  });

  it("is nothing when no version is stored", () => {
    expect(currentDocument([doc({ state: "deleted" })])).toBeUndefined();
    expect(currentDocument([])).toBeUndefined();
  });
});

describe("factText", () => {
  it("reads each kind by its own field", () => {
    expect(
      factText(fact({ kind: "role", value: { title: "Senior Engineer" } })),
    ).toBe("Senior Engineer");
    expect(factText(fact({ kind: "skill", value: { name: "Go" } }))).toBe("Go");
    expect(
      factText(
        fact({
          kind: "date_range",
          value: { start: "Mar 2020", end: "Present" },
        }),
      ),
    ).toBe("Mar 2020 to Present");
    expect(
      factText(
        fact({ kind: "achievement", value: { text: "Led the migration" } }),
      ),
    ).toBe("Led the migration");
    expect(
      factText(fact({ kind: "unparsed", value: { text: "I also volunteer" } })),
    ).toBe("I also volunteer");
  });

  it("reads the correction when one exists, because the candidate's word wins", () => {
    const corrected = fact({
      status: "corrected",
      corrected_value: { name: "Golang" },
    });

    expect(factText(corrected)).toBe("Golang");
  });
});

describe("correctionFor", () => {
  it("carries the whole value with the primary field replaced, minus the confidence", () => {
    const original = fact({
      kind: "role",
      value: { title: "Senior Enginer", confidence: 0.7 },
    });

    expect(correctionFor(original, "Senior Engineer")).toEqual({
      title: "Senior Engineer",
    });
  });

  it("edits from the existing correction when there is one", () => {
    const corrected = fact({
      kind: "skill",
      status: "corrected",
      corrected_value: { name: "Golang" },
    });

    expect(correctionFor(corrected, "Go")).toEqual({ name: "Go" });
  });
});

describe("the words", () => {
  it("names each status as the candidate's own act", () => {
    expect(statusLabel("proposed")).toBe("Parsed");
    expect(statusLabel("confirmed")).toBe("Confirmed by you");
    expect(statusLabel("corrected")).toBe("Edited by you");
    expect(statusLabel("rejected")).toBe("Rejected by you");
  });

  it("speaks the span in characters of the CV, half-open honestly closed", () => {
    expect(spanSentence(fact({ span_start: 80, span_end: 82 }))).toBe(
      "Characters 80 to 82 of your CV",
    );
  });

  it("knows each kind's primary field", () => {
    expect(primaryField("role")).toBe("title");
    expect(primaryField("skill")).toBe("name");
    expect(primaryField("achievement")).toBe("text");
    expect(primaryField("unparsed")).toBe("text");
  });
});
