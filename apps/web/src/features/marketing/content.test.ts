import { describe, expect, it } from "vitest";

import * as content from "./content";

/**
 * The copy, held to the two rules the port had to bend to keep.
 *
 * Both are recorded deviations from the prototype, and a recorded deviation
 * that nothing enforces is one that drifts back the first time somebody adds a
 * line of copy. These are cheap and they fail loudly.
 */

/** Every string anywhere in the content module, with the path that reached it. */
function strings(
  value: unknown,
  path: string,
): { path: string; text: string }[] {
  if (typeof value === "string") return [{ path, text: value }];
  if (Array.isArray(value))
    return value.flatMap((item, index) => strings(item, `${path}[${index}]`));
  if (typeof value === "object" && value !== null) {
    return Object.entries(value).flatMap(([key, item]) =>
      strings(item, `${path}.${key}`),
    );
  }
  return [];
}

const everything = strings(content, "content");

describe("the marketing copy", () => {
  it("has copy to check, so this is not vacuous", () => {
    expect(everything.length).toBeGreaterThan(150);
  });

  /**
   * The product's copy rule forbids em dashes and the prototype's copy is full
   * of them. Every one was rewritten as a colon, a comma or a full stop, and
   * this is what stops the next one arriving.
   */
  it("uses no em dashes", () => {
    const offenders = everything
      .filter((entry) => entry.text.includes("—"))
      .map((entry) => `${entry.path}: ${entry.text}`);

    expect(offenders).toEqual([]);
  });

  /**
   * The prototype links to 56 HTML files. Most of those screens are not ported,
   * and a link from the front page to a screen that does not exist is a 404 in
   * the most visible place in the product. Every destination here is either an
   * anchor to a section of this page or a route the application actually has.
   */
  it("links only to routes that exist", () => {
    const routes = new Set([
      "/",
      "/login",
      "/register",
      "/practice",
      "/practice/new",
      "/profile",
    ]);
    const links = [
      ...content.primaryNav,
      ...content.footer.columns.flatMap((column) => column.links),
    ];

    const dead = links
      .filter((link) => !link.href.startsWith("#") && !routes.has(link.href))
      .map((link) => `${link.label} -> ${link.href}`);

    expect(dead).toEqual([]);
  });

  /** An anchor that points at no section is the same dead end, one page in. */
  it("points every anchor at a section of the page", () => {
    const sections = new Set([
      "#product",
      "#use-cases",
      "#how",
      "#evidence",
      "#faq",
      "#realtime",
    ]);
    const anchors = [
      ...content.primaryNav,
      ...content.footer.columns.flatMap((column) => column.links),
    ].filter((link) => link.href.startsWith("#"));

    expect(anchors.filter((link) => !sections.has(link.href))).toEqual([]);
  });

  /** React needs them unique, and so does the accordion's `aria-controls`. */
  it("gives every question its own identifier", () => {
    const ids = content.faq.items.map((item) => item.id);

    expect(new Set(ids).size).toBe(ids.length);
  });

  /**
   * ADR-0018 requires the isolation guarantee in candidate-facing copy wherever
   * the two modes meet, and this page is where a visitor first meets both. The
   * three halves of it: a screen candidate is shown nothing, coaching is never
   * produced rather than merely hidden, and practice reaches no employer.
   */
  it("states the isolation guarantee", () => {
    const screenRow = content.howItWorks.rows.find(
      (row) => row.what === "Overall score and band",
    );
    expect(screenRow?.screen.text).toBe("Never shown");

    const coaching = content.howItWorks.rows.find(
      (row) => row.what === "Per-answer coaching and model rewrites",
    );
    expect(coaching?.screen.text).toBe("Never generated at all");

    const practice = content.faq.items.find(
      (item) => item.id === "practice-history",
    );
    expect(practice?.answer).toContain("never visible to any employer tenant");
  });

  /**
   * Every cell says what it means in words. The glyph and the colour are how it
   * reads at a glance; the sentence is how it reads at all.
   */
  it("writes out every answer in the visibility table", () => {
    const empty = content.howItWorks.rows.flatMap((row) =>
      [row.practice, row.screen, row.recruiter].filter(
        (cell) => cell.text.trim() === "",
      ),
    );

    expect(empty).toEqual([]);
  });
});
