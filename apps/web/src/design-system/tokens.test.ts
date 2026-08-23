import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

import { colorTokens, spaceTokens, token } from "./tokens";

// Read from the working directory rather than import.meta.url: Vite serves
// modules under its own scheme, so the module URL is not a file path here.
const css = readFileSync(resolve(process.cwd(), "src/design-system/tokens.css"), "utf8");

/**
 * Returns the custom property names declared inside the first block matching
 * `selector`, so a test can compare what each theme actually defines.
 */
function declaredIn(selector: string): Set<string> {
  const start = css.indexOf(selector);
  if (start === -1) return new Set();
  const open = css.indexOf("{", start);
  const close = css.indexOf("}", open);
  const block = css.slice(open, close);
  return new Set([...block.matchAll(/(--[a-z0-9-]+)\s*:/g)].map((m) => m[1] as string));
}

describe("design tokens", () => {
  /**
   * The prototype's rule, carried across by WEB-06: a colour must never have
   * its only definition inside a theme block, or the theme that does not match
   * renders with no value at all.
   */
  it("defines every semantic colour in the light theme", () => {
    const light = declaredIn(":root, [data-theme=\"light\"]");

    const missing = colorTokens.filter((token) => !light.has(token));

    expect(missing).toEqual([]);
  });

  it("redefines every semantic colour in the dark theme", () => {
    const dark = declaredIn("[data-theme=\"dark\"]");

    const missing = colorTokens.filter((token) => !dark.has(token));

    expect(missing).toEqual([]);
  });

  /**
   * A token may be declared on the bare `:root` block, which both themes
   * inherit, or on the light theme block. What it may never be is declared
   * only under `[data-theme="dark"]`, because then the light theme renders it
   * as an empty value.
   */
  it("declares no colour only in the dark theme", () => {
    const base = declaredIn(":root");
    const light = declaredIn(":root, [data-theme=\"light\"]");
    const dark = declaredIn("[data-theme=\"dark\"]");

    const darkOnly = [...dark].filter((token) => !base.has(token) && !light.has(token));

    expect(darkOnly).toEqual([]);
  });

  it("exposes the spacing scale it declares", () => {
    const root = declaredIn(":root");

    const missing = spaceTokens.filter((token) => !root.has(token));

    expect(missing).toEqual([]);
  });

  /**
   * Components read semantic tokens, never raw palette values. A component that
   * reaches for `--reef-600` breaks the moment the brand changes, and it will
   * not follow the theme.
   */
  it("keeps every exported colour token semantic rather than a raw palette value", () => {
    const rawPaletteName = /^--(reef|ember|plum|coral|moss|sky|stone)-\d+$/;

    const raw = colorTokens.filter((token) => rawPaletteName.test(token));

    expect(raw).toEqual([]);
  });

  it("exports no token that the stylesheet does not declare", () => {
    const everything = new Set([...css.matchAll(/(--[a-z0-9-]+)\s*:/g)].map((m) => m[1] as string));

    const undeclared = [...colorTokens, ...spaceTokens].filter((token) => !everything.has(token));

    expect(undeclared).toEqual([]);
  });
});

describe("token()", () => {
  it("returns a CSS var reference a component can use directly", () => {
    expect(token("--primary")).toBe("var(--primary)");
    expect(token("--sp-4")).toBe("var(--sp-4)");
  });

  /**
   * The point of the helper is that a renamed token becomes a type error at the
   * call site rather than an empty value at runtime, so it must not silently
   * accept an arbitrary string.
   */
  it("only accepts tokens the stylesheet declares", () => {
    // @ts-expect-error a token that does not exist must not type check
    expect(() => token("--not-a-token")).not.toThrow();
  });
});
