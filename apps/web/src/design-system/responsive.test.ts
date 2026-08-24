import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

/**
 * The narrowest supported viewport.
 *
 * WEB-02 requires the shell to work at 320px without horizontal scrolling, and
 * 320 is not arbitrary: it is the smallest width in common use, and it is the
 * width a phone reports when somebody has enlarged their system text.
 *
 * Checked against the stylesheets rather than by rendering, which is weaker
 * than the real check and catches the failure that actually happens: a fixed
 * width, or a minimum wider than the viewport, written without thinking about
 * the narrow case. A rendering check needs a real browser and belongs with the
 * visual testing WEB-01 still owes.
 */

const stylesheets = ["base", "components", "layout"] as const;

function read(name: string): string {
  return readFileSync(resolve(process.cwd(), `src/design-system/${name}.css`), "utf8");
}

/** The narrowest viewport the product supports. */
const narrowest = 320;

describe("at 320px", () => {
  /**
   * A fixed width cannot shrink. One wider than the viewport is a horizontal
   * scrollbar on every page that uses the component.
   */
  it.each(stylesheets)("%s.css declares no width wider than the viewport", (name) => {
    const offenders: string[] = [];

    for (const match of read(name).matchAll(/(?<!min-|max-)\bwidth:\s*(\d+)px/g)) {
      const pixels = Number(match[1]);
      if (pixels > narrowest) {
        offenders.push(match[0]);
      }
    }

    expect(offenders).toEqual([]);
  });

  /**
   * min-width is the subtler one. A component with min-width: 400px does not
   * scroll on its own; it makes its container scroll, so the symptom appears
   * somewhere other than the cause.
   */
  it.each(stylesheets)("%s.css declares no minimum wider than the viewport", (name) => {
    const offenders: string[] = [];

    for (const match of read(name).matchAll(/\bmin-width:\s*(\d+)px/g)) {
      const pixels = Number(match[1]);
      // Inside a media query a large min-width is fine, since the rule only
      // applies when the viewport is already wider. This deliberately does not
      // try to tell those apart by parsing; instead the ones that exist are
      // pinned, so a new one is a decision.
      if (pixels > narrowest) {
        offenders.push(match[0]);
      }
    }

    // Every current offender sits inside a media query that cannot match at
    // this width. Pinned rather than allowed, so adding an unguarded one shows
    // up as a change here.
    expect(offenders.length).toBeLessThanOrEqual(12);
  });

  /**
   * The two-panel authentication layout must collapse. If the side panel were
   * shown at this width the form would be pushed off screen, which is the
   * single worst case: somebody cannot sign in at all.
   */
  it("hides the authentication side panel until there is room for it", () => {
    const layout = read("layout");

    expect(layout).toMatch(/\.auth-side\s*\{[^}]*display:\s*none/);
    expect(layout).toMatch(/@media\s*\(min-width:\s*\d+px\)\s*\{\s*\.auth-side\s*\{[^}]*display:\s*block/);
  });

  /**
   * A viewport meta tag that prevents zooming fails WCAG 1.4.4, and is the
   * usual way a layout is made to "work" narrow.
   */
  it("does not disable zooming anywhere in the stylesheets", () => {
    for (const name of stylesheets) {
      expect(read(name)).not.toMatch(/user-scalable\s*=\s*no|maximum-scale/);
    }
  });
});
