import { readFileSync } from "node:fs";
import { readdirSync, statSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import { describe, expect, it } from "vitest";

/**
 * Tailwind, checked against the tokens it is configured from.
 *
 * This replaced a test that read the ported stylesheets, which no longer exist:
 * the styling is utilities now. The risk it guarded against has changed shape
 * rather than gone away, and in one respect got worse.
 *
 * Tailwind does not complain about a utility it does not recognise. `bg-surfce`
 * generates no CSS and no warning, and the element renders unstyled. Nothing
 * here can catch that in a class string, and the browser suite is what does: an
 * unstyled element fails contrast, or the screenshot, or both.
 *
 * What can be checked here is the mapping, which is where a token and its
 * utility stop agreeing.
 */

const styles = resolve(process.cwd(), "src/shared/styles");
const themeCss = readFileSync(join(styles, "theme.css"), "utf8");
const tokensCss = readFileSync(join(styles, "tokens.css"), "utf8");

/** Every custom property tokens.css declares, in any block. */
const tokens = new Set([...tokensCss.matchAll(/(--[a-z0-9-]+)\s*:/g)].map((m) => m[1] as string));

/** The `@theme inline` block, which is the whole of the Tailwind configuration. */
function themeBlock(): string {
  const start = themeCss.indexOf("@theme inline {");
  const end = themeCss.indexOf("\n}", start);
  return themeCss.slice(start, end);
}

describe("the Tailwind theme", () => {
  it("maps only tokens that exist", () => {
    const missing = [...themeBlock().matchAll(/var\((--[a-z0-9-]+)\)/g)]
      .map((match) => match[1] as string)
      .filter((token) => !tokens.has(token));

    expect(missing).toEqual([]);
  });

  /**
   * `@theme inline` rather than `@theme`, and this is the line that keeps dark
   * mode working.
   *
   * A plain `@theme` copies the token's value into every utility at build time,
   * so `bg-surface` would carry the light colour and stay light when the
   * attribute changes. `inline` emits a reference instead, resolved at use.
   */
  it("emits references rather than baking in the light values", () => {
    expect(themeCss).toContain("@theme inline");
  });

  /**
   * The product defaults to dark and an explicit choice wins, so the browser's
   * own preference is deliberately not consulted. Tailwind's default `dark:`
   * is `prefers-color-scheme`, which would disagree with that on most machines.
   */
  it("selects dark by the attribute rather than the media query", () => {
    expect(themeCss).toMatch(/@custom-variant dark \(\[data-theme="dark"\] &\)/);
  });

  it("defines no colour of its own", () => {
    // Every value in the theme block is a reference. A literal here would be a
    // colour that exists in Tailwind and not in the design source.
    const literals = [...themeBlock().matchAll(/:\s*(#[0-9a-fA-F]{3,8}|rgb|hsl)/g)];

    expect(literals.map((match) => match[0])).toEqual([]);
  });
});

/** Every component and feature file, which is where utilities are written. */
function markupFiles(): string[] {
  const found: string[] = [];
  const walk = (path: string) => {
    for (const entry of readdirSync(path)) {
      const full = join(path, entry);
      if (statSync(full).isDirectory()) {
        walk(full);
        continue;
      }
      if (full.endsWith(".tsx") && !full.endsWith(".test.tsx")) found.push(full);
    }
  };
  walk(resolve(process.cwd(), "src"));
  return found;
}

describe("markup", () => {
  /**
   * A hard-coded colour in a component is a colour that will not follow the
   * theme, which is how something ends up unreadable in dark mode. It is also
   * invisible in review, because it looks like any other class.
   */
  it("hard-codes no colour", () => {
    const offenders: string[] = [];

    for (const file of markupFiles()) {
      const source = readFileSync(file, "utf8");
      // Tailwind's arbitrary-value syntax is the way a raw colour gets in:
      // bg-[#123456], text-[rgb(...)].
      for (const match of source.matchAll(/-\[(#[0-9a-fA-F]{3,8}|rgba?\([^)]*\))\]/g)) {
        offenders.push(`${relative(process.cwd(), file)}: ${match[0]}`);
      }
    }

    expect(offenders).toEqual([]);
  });
});
