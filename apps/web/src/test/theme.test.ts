import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

/**
 * Both themes, and the contrast the accessibility commitment depends on.
 *
 * Nothing had exercised the dark theme at all. Every component test renders in
 * whatever jsdom defaults to, which is neither theme in any meaningful sense,
 * so a token defined only in light would have passed everything and rendered as
 * nothing in dark.
 *
 * Contrast is computed from the tokens rather than measured in a browser. That
 * is weaker than the real check WEB-01 asks for, which needs rendering, and it
 * is worth having because it catches the failure that gets introduced by
 * editing a colour, which is most of them.
 */

const css = readFileSync(resolve(process.cwd(), "src/shared/styles/tokens.css"), "utf8");

/** The declarations inside the first block matching a selector. */
function block(selector: string): Map<string, string> {
  const start = css.indexOf(selector);
  if (start === -1) return new Map();
  const open = css.indexOf("{", start);
  const close = css.indexOf("}", open);
  const declarations = new Map<string, string>();
  for (const match of css.slice(open, close).matchAll(/(--[a-z0-9-]+)\s*:\s*([^;]+);/g)) {
    declarations.set(match[1] as string, (match[2] as string).trim());
  }
  return declarations;
}

const light = block(':root, [data-theme="light"]');
const dark = block('[data-theme="dark"]');

/**
 * The semantic colours, taken from the light theme rather than from a list
 * somebody maintains.
 *
 * There used to be a typed array of token names for component props to accept.
 * Nothing takes a token as a prop now that styling is utilities, so the array
 * survived only because its own test imported it. Deriving the list from the
 * file it describes is both less to maintain and impossible to leave stale.
 */
const semanticColours = [...light.keys()].filter(
  (name) =>
    !/^--(stone|reef|ember|moss|coral|sky|plum)-/.test(name) &&
    !/^--(sp|r|text|dur|ease|font|shadow|sidebar-w|topbar-h|content-max|focus-ring|z)-/.test(name),
);

/**
 * Every declaration in the file, used to resolve the palette.
 *
 * The semantic tokens are deliberately indirect: --fg is var(--stone-900), so
 * that the palette can be adjusted in one place. Contrast has to see through
 * that, which is why this resolves rather than reading the declaration.
 */
const everything = new Map<string, string>();
for (const match of css.matchAll(/(--[a-z0-9-]+)\s*:\s*([^;]+);/g)) {
  const name = match[1] as string;
  if (!everything.has(name)) everything.set(name, (match[2] as string).trim());
}

/**
 * resolveToken follows var() references until it reaches a literal.
 *
 * Named for what it resolves rather than just "resolve", which shadowed
 * node:path's and turned readFileSync's path into an empty string. The file
 * then failed to load with EISDIR and reported zero tests rather than a
 * failure, which is the worst way for a test file to break.
 *
 * Bounded, because a token defined in terms of itself would otherwise hang the
 * test rather than fail it, and a hang in a suite is far harder to diagnose
 * than an assertion.
 */
function resolveToken(value: string, theme: Map<string, string>, depth = 0): string {
  if (depth > 8) return "";

  const reference = /^var\(\s*(--[a-z0-9-]+)\s*\)$/.exec(value.trim());
  if (!reference) return value.trim();

  const name = reference[1] as string;
  const next = theme.get(name) ?? everything.get(name);
  return next === undefined ? "" : resolveToken(next, theme, depth + 1);
}

/** The literal colour a token has in one theme. */
function colour(theme: Map<string, string>, name: string): string {
  return resolveToken(theme.get(name) ?? "", theme);
}

/** Relative luminance, per WCAG. */
function luminance(hex: string): number | null {
  const parsed = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(hex.trim());
  if (!parsed) return null;

  let digits = parsed[1] as string;
  if (digits.length === 3) {
    digits = digits
      .split("")
      .map((d) => d + d)
      .join("");
  }

  const channels = [0, 2, 4].map((i) => {
    const value = parseInt(digits.slice(i, i + 2), 16) / 255;
    return value <= 0.03928 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
  }) as [number, number, number];

  return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2];
}

/** WCAG contrast ratio between two colours, or null if either is not a hex. */
function contrast(a: string, b: string): number | null {
  const first = luminance(a);
  const second = luminance(b);
  if (first === null || second === null) return null;

  const lighter = Math.max(first, second);
  const darker = Math.min(first, second);
  return (lighter + 0.05) / (darker + 0.05);
}

describe("themes", () => {
  it("has semantic colours to check, so this is not vacuous", () => {
    expect(semanticColours.length).toBeGreaterThan(20);
  });

  /**
   * A colour defined only in one theme renders as nothing in the other, which
   * is a blank element rather than an error and therefore the failure most
   * likely to reach somebody.
   */
  it("redefines every semantic colour in the dark theme", () => {
    const missing = semanticColours.filter((name) => !dark.has(name));

    expect(missing).toEqual([]);
  });

  /**
   * Body text on the page background, in both themes. 4.5:1 is the WCAG 2.2 AA
   * threshold for normal text, and the product commits to AA.
   */
  it.each([
    ["light", light],
    ["dark", dark],
  ])("has readable body text in the %s theme", (_name, theme) => {
    const ratio = contrast(colour(theme, "--fg"), colour(theme, "--bg"));

    expect(ratio).not.toBeNull();
    expect(ratio as number).toBeGreaterThanOrEqual(4.5);
  });

  /**
   * Secondary text is the one that gets missed, because it is chosen to look
   * quieter and quieter is exactly what fails contrast.
   */
  it.each([
    ["light", light],
    ["dark", dark],
  ])("has readable secondary text in the %s theme", (_name, theme) => {
    for (const name of ["--fg-2", "--fg-3"]) {
      const ratio = contrast(colour(theme, name), colour(theme, "--bg"));

      expect(ratio, `${name} against --bg`).not.toBeNull();
      expect(ratio as number, `${name} against --bg`).toBeGreaterThanOrEqual(4.5);
    }
  });

  /**
   * A theme that reuses the other's background is a theme that was never
   * actually written, and the mistake is invisible until somebody switches.
   */
  it("does not reuse the light background in the dark theme", () => {
    expect(colour(dark, "--bg")).not.toBe(colour(light, "--bg"));
    expect(colour(dark, "--fg")).not.toBe(colour(light, "--fg"));
  });

  /**
   * The dark theme has to actually be darker. A palette can satisfy every
   * contrast rule above while being inverted by mistake.
   */
  it("is actually darker in the dark theme", () => {
    const lightBackground = luminance(colour(light, "--bg"));
    const darkBackground = luminance(colour(dark, "--bg"));

    expect(lightBackground).not.toBeNull();
    expect(darkBackground).not.toBeNull();
    expect(darkBackground as number).toBeLessThan(lightBackground as number);
  });
});
