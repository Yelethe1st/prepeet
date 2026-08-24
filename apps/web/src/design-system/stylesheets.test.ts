import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

import { colorTokens, spaceTokens } from "./tokens";

/**
 * The ported stylesheets, checked against the tokens they are allowed to use.
 *
 * /screens is the design source and these files are a port of it, so the value
 * of a test here is not "does it look right" — that is settled by the source.
 * It is that a rule cannot reference a custom property nothing defines, which
 * renders as an unstyled element rather than as an error and is therefore the
 * failure most likely to survive review.
 */

const stylesheets = ["base", "components", "layout"] as const;

function read(name: string): string {
  return readFileSync(resolve(process.cwd(), `src/design-system/${name}.css`), "utf8");
}

/** Every custom property a stylesheet declares, in any block. */
function declared(css: string): Set<string> {
  return new Set([...css.matchAll(/(--[a-z0-9-]+)\s*:/g)].map((m) => m[1] as string));
}

const tokens = declared(read("tokens"));

/**
 * Every custom property a stylesheet reads through var() without a fallback.
 *
 * A reference with a fallback is excluded on purpose. `var(--cols, 2)` is a
 * page-local property the layout sets inline, and its fallback is what makes
 * being undefined the normal case rather than a bug. Treating those as missing
 * tokens would mean either a false failure or a list of exceptions to maintain.
 */
function referenced(css: string): Set<string> {
  const withoutFallback = /var\(\s*(--[a-z0-9-]+)\s*\)/g;
  return new Set([...css.matchAll(withoutFallback)].map((m) => m[1] as string));
}

describe("ported stylesheets", () => {
  it.each(stylesheets)("%s.css references only properties something defines", (name) => {
    const css = read(name);
    // A stylesheet may declare its own component-scoped properties, as .ring
    // does with --size and --stroke. Those are not tokens and are not meant to
    // be: they are how a variant changes one component without a new rule.
    const available = new Set([...tokens, ...declared(css)]);

    const missing = [...referenced(css)].filter((property) => !available.has(property));

    expect(missing).toEqual([]);
  });

  it.each(stylesheets)("%s.css records where it was ported from", (name) => {
    expect(read(name)).toContain(`screens/assets/css/${name}.css`);
  });

  /**
   * The typed token lists exist so a component prop can be checked at compile
   * time. They are only useful while they describe what the stylesheets
   * actually define, so this is the assertion that keeps them honest.
   */
  it("declares every token the typed lists name", () => {
    const named = [...colorTokens, ...spaceTokens];

    const missing = named.filter((name) => !tokens.has(name));

    expect(missing).toEqual([]);
  });

  /**
   * A hard-coded colour in a ported stylesheet is a value that will not follow
   * the theme, which is how a component ends up unreadable in dark mode. The
   * prototype allows them only where a colour is deliberately fixed, so this
   * counts rather than forbids: a jump means somebody stopped using tokens.
   */
  it("does not accumulate hard-coded colours", () => {
    const hexes = stylesheets.flatMap((name) => [...read(name).matchAll(/#[0-9a-fA-F]{3,8}\b/g)]);

    // Nine at the time of the port, all in the prototype: fixed brand and
    // provider colours that are meant not to follow the theme. The ceiling is
    // the count itself, so adding one is a decision somebody makes rather than
    // a side effect nobody sees.
    expect(hexes.length).toBeLessThanOrEqual(9);
  });
});
