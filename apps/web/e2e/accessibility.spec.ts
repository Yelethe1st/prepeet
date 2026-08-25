import AxeBuilder from "@axe-core/playwright";

import { expect, setTheme, test } from "./fixtures";

/**
 * Accessibility, as rendered.
 *
 * The jsdom suite already runs axe against every component, and that check is
 * blind to anything involving colour or layout: jsdom does not apply
 * stylesheets, so a colour set by a class comes back as the browser's default
 * and every contrast rule is skipped.
 *
 * This is the same tool with the stylesheets actually applied, which is where
 * contrast is decided.
 */

/** Every route worth checking. Each new screen is added here as it is ported. */
const routes = [
  "/login",
  "/register",
  // IAM-02's screens. The consume pages land on their invalid state without a
  // token, which is itself a state worth auditing: it is what a truncated
  // link renders.
  "/forgot-password",
  "/check-email",
  "/reset-password",
  "/verify-email",
  "/magic-link",
  "/otp",
] as const;

/**
 * WCAG 2.2 AA, which is what the product commits to.
 *
 * The tags matter: without them axe runs its full rule set including best
 * practices, and a suite that fails on advice rather than on obligations is one
 * people start ignoring.
 */
const standard = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"];

test.describe("accessibility", () => {
  for (const route of routes) {
    test(`${route} has no violations`, async ({ page }) => {
      await page.goto(route);

      const results = await new AxeBuilder({ page }).withTags(standard).analyze();

      // The message names the rule and the element, because "3 violations" sends
      // whoever reads it back to the browser to find out what.
      expect(
        results.violations.map((v) => ({
          rule: v.id,
          impact: v.impact,
          help: v.help,
          nodes: v.nodes.map((n) => n.target.join(" ")),
        })),
      ).toEqual([]);
    });

    /**
     * Contrast in both themes, checked separately from the run above.
     *
     * The default is dark and that is what the run above sees, so light would
     * otherwise never be rendered by anything. A palette can satisfy every
     * token-level ratio and still fail here, because this measures what a text
     * node actually sits on rather than the pair somebody chose to check.
     */
    for (const theme of ["dark", "light"] as const) {
      test(`${route} has sufficient contrast in the ${theme} theme`, async ({ page }) => {
        await page.goto(route);
        await setTheme(page, theme);

        const results = await new AxeBuilder({ page })
          .withTags(standard)
          .include("body")
          .withRules(["color-contrast"])
          .analyze();

        expect(
          results.violations.flatMap((v) =>
            v.nodes.map((n) => ({ element: n.target.join(" "), problem: n.failureSummary })),
          ),
        ).toEqual([]);
      });
    }
  }

  /**
   * The skip link is the first thing a keyboard user meets and is invisible
   * until focused, which is exactly why it is the control most often left
   * broken: nobody sees it while building the page.
   */
  test("the skip link is reachable and moves focus", async ({ page }) => {
    await page.goto("/login");

    await page.keyboard.press("Tab");

    const focused = page.locator(":focus");
    await expect(focused).toHaveText(/skip to main content/i);

    await page.keyboard.press("Enter");
    await expect(page.locator("#main-content")).toBeVisible();
  });

  /**
   * A focus ring that is invisible is the same as no keyboard support for
   * anybody who cannot track focus by memory.
   */
  test("focused controls are visibly focused", async ({ page }) => {
    await page.goto("/login");

    const email = page.getByLabel(/email/i);
    await email.focus();

    const outline = await email.evaluate((node) => {
      const style = window.getComputedStyle(node);
      return {
        outlineWidth: style.outlineWidth,
        outlineStyle: style.outlineStyle,
        boxShadow: style.boxShadow,
      };
    });

    const visible =
      (outline.outlineStyle !== "none" && outline.outlineWidth !== "0px") ||
      (outline.boxShadow !== "none" && outline.boxShadow !== "");

    expect(visible, `focus styles were ${JSON.stringify(outline)}`).toBe(true);
  });
});
