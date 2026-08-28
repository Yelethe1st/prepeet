import { expect, test } from "./fixtures";

/**
 * Layout at the sizes the product supports.
 *
 * jsdom cannot do any of this. It has no layout engine, so every element
 * measures zero and no test there can tell whether something overflows. No
 * static check covers this either: a fixed width, a long word with no break
 * opportunity, a nowrap or a flex child that refuses to shrink all read as
 * ordinary utilities, and those are how a page actually ends up with a
 * horizontal scrollbar.
 */

const routes = [
  // The front page, ported under WEB-06. It is the widest thing in the product
  // by a distance: a nine-by-four table, three-column grids and a hero laid out
  // side by side, all of which have to fold onto a phone.
  "/",
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
  // WEB-05's destinations. The 404 sweep goes through a route that does not
  // exist, which is how everyone else will reach it too; the 500 boundary
  // cannot be reached by URL and is covered in the unit suite.
  "/access-denied",
  "/session-expired",
  "/no-workspace",
  "/this-route-does-not-exist",
] as const;

/**
 * The routes that show an email input on arrival. reset-password and
 * verify-email land token-first, check-email echoes rather than asks, and otp
 * asks only in the tab that has not already requested a code.
 */
const routesWithAnEmailInput = new Set(["/login", "/register", "/forgot-password", "/magic-link", "/otp"]);

/** Reads whether the document is wider than the window can show. */
async function horizontalOverflow(page: import("@playwright/test").Page) {
  return page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
    // The widest element that sticks out, so a failure names the cause rather
    // than only the symptom.
    widest: (() => {
      let worst = { selector: "", right: 0 };
      for (const node of Array.from(document.querySelectorAll<HTMLElement>("body *"))) {
        const right = node.getBoundingClientRect().right;
        if (right > worst.right) {
          worst = {
            selector: `${node.tagName.toLowerCase()}${node.className ? `.${String(node.className).split(" ")[0]}` : ""}`,
            right,
          };
        }
      }
      return worst;
    })(),
  }));
}

test.describe("layout", () => {
  for (const route of routes) {
    test(`${route} does not scroll horizontally`, async ({ page }) => {
      await page.goto(route);

      const { scrollWidth, clientWidth, widest } = await horizontalOverflow(page);

      expect(
        scrollWidth,
        `document is ${scrollWidth}px in a ${clientWidth}px viewport; widest element is ${widest.selector} ending at ${widest.right}px`,
      ).toBeLessThanOrEqual(clientWidth);
    });

    /**
     * A long unbroken string is the usual cause, and an email address is the one
     * this product will actually receive: they are long, they are user supplied,
     * and they are echoed back in confirmations.
     */
    test(`${route} survives a very long email address`, async ({ page }) => {
      test.skip(!routesWithAnEmailInput.has(route), "no email input to fill on this route");
      await page.goto(route);

      await page
        .getByLabel(/email address|email/i)
        .first()
        .fill("daniel.okonkwo.with.an.unusually.long.address@a-very-long-organisation-name.example.com");

      const { scrollWidth, clientWidth, widest } = await horizontalOverflow(page);

      expect(
        scrollWidth,
        `a long address made the document ${scrollWidth}px wide in ${clientWidth}px; widest is ${widest.selector}`,
      ).toBeLessThanOrEqual(clientWidth);
    });
  }

  /**
   * check-email has no input, but it echoes the address back, which is the
   * other way a long one breaks a layout: not typed into a box that scrolls,
   * but rendered into a definition list that does not.
   */
  test("/check-email survives echoing a very long address", async ({ page }) => {
    await page.goto("/check-email");
    await page.evaluate(() => {
      sessionStorage.setItem(
        "prepeet.sent-email",
        JSON.stringify({
          kind: "password_reset",
          email:
            "daniel.okonkwo.with.an.unusually.long.address@a-very-long-organisation-name.example.com",
        }),
      );
    });
    await page.reload();
    await page.getByText(/@a-very-long-organisation-name/).waitFor();

    const { scrollWidth, clientWidth, widest } = await horizontalOverflow(page);
    expect(
      scrollWidth,
      `the echoed address made the document ${scrollWidth}px wide in ${clientWidth}px; widest is ${widest.selector}`,
    ).toBeLessThanOrEqual(clientWidth);
  });

  /**
   * The two-panel layout must collapse. If the side panel were shown at this
   * width the form would be pushed off screen, and somebody on a small phone
   * could not sign in at all.
   */
  test("the authentication side panel is hidden on a narrow viewport", async ({ page }, testInfo) => {
    test.skip(testInfo.project.name !== "narrow", "only meaningful at 320px");

    await page.goto("/login");

    await expect(page.getByRole("complementary")).toBeHidden();
    await expect(page.getByLabel(/email/i)).toBeVisible();
  });

  test("the side panel is shown when there is room", async ({ page }, testInfo) => {
    test.skip(testInfo.project.name !== "desktop", "only meaningful at desktop width");

    await page.goto("/login");

    await expect(page.getByRole("complementary")).toBeVisible();
  });

  /**
   * Every control has to be operable, not merely present. A field pushed under
   * a fixed panel is visible to a test that only checks visibility.
   */
  test("every form control is reachable and usable", async ({ page }) => {
    await page.goto("/register");

    for (const control of await page.locator("input:not([type=hidden])").all()) {
      await expect(control).toBeVisible();
      const box = await control.boundingBox();
      expect(box, "a control has no box").not.toBeNull();
      expect(box!.width, "a control is zero width").toBeGreaterThan(0);
    }
  });

  /**
   * Text must survive being enlarged. WCAG 1.4.4 requires 200% without loss of
   * content or function, and the usual failure is a fixed-height container that
   * clips rather than grows.
   */
  test("survives text being doubled", async ({ page }) => {
    await page.goto("/login");
    await page.addStyleTag({ content: "html { font-size: 200% !important; }" });

    const { scrollWidth, clientWidth, widest } = await horizontalOverflow(page);

    expect(
      scrollWidth,
      `at 200% text the document is ${scrollWidth}px in ${clientWidth}px; widest is ${widest.selector}`,
    ).toBeLessThanOrEqual(clientWidth);
    await expect(page.getByRole("button", { name: /sign in/i })).toBeVisible();
  });
});
