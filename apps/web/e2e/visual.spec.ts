import type { Page } from "@playwright/test";

import { expect, hydrated, setTheme, test } from "./fixtures";

/**
 * Visual regression.
 *
 * Nothing else in the repository compares what a screen looks like to what it
 * looked like. A stylesheet edit could move every control, change every
 * spacing, and leave the auth panel in the wrong place, and all 146 jsdom tests
 * would pass, because none of them looks at pixels.
 *
 * What this cannot do is say whether a screen looks *right*. It says only that
 * it looks the same, which means a baseline is only worth what it was worth
 * when it was taken. Updating one is a decision: `pnpm test:browser:update`,
 * and the diff in the committed image is the thing to review.
 *
 * Baselines are per platform, because font rendering differs. Playwright names
 * them accordingly, so a machine without a baseline for its platform fails
 * rather than silently comparing against somebody else's rendering.
 */

const routes = [
  { path: "/login", name: "login" },
  { path: "/register", name: "register" },
  // The two form-heavy recovery screens. The rest of IAM-02's pages are
  // states of the same card and are covered by the accessibility and layout
  // sweeps; a baseline pair per state would be maintenance without a matching
  // chance of catching anything those two do not.
  { path: "/forgot-password", name: "forgot-password" },
  { path: "/otp", name: "otp" },
] as const;

test.describe("appearance", () => {
  for (const route of routes) {
    for (const theme of ["dark", "light"] as const) {
      test(`${route.name} in the ${theme} theme`, async ({ page }) => {
        await page.goto(route.path);
        await setTheme(page, theme);

        // Fonts settle after first paint, and a screenshot taken before they do
        // compares a fallback face against the real one on every run.
        await page.evaluate(() => document.fonts.ready);

        // Nothing on the page still claims to be loading.
        //
        // A region that draws placeholders while it waits is taller or shorter
        // depending on whether its request has finished, so a screenshot taken
        // mid-flight compares one run's timing against another's. That is not a
        // hypothetical: the sign-in screen began drawing placeholders for its
        // provider buttons, and the baseline captured locally, where the request
        // failed immediately, was 112 pixels shorter than the one CI captured
        // while the same request was still outstanding.
        //
        // Waiting for the settled state is also the more useful baseline. A
        // person reads this screen after it has loaded, and a placeholder is not
        // a design anybody reviews.
        await settled(page);

        await expect(page).toHaveScreenshot(`${route.name}-${theme}.png`, {
          fullPage: true,
          // An absolute count, not a ratio.
          //
          // maxDiffPixelRatio: 0.01 was the first attempt and made the whole
          // tier decorative: one percent of a full-page screenshot is roughly
          // ten thousand pixels, and squaring the corners off every button on
          // the page changes far fewer than that. It passed.
          //
          // A few hundred absorbs antialiasing between runs on one machine and
          // nothing else.
          maxDiffPixels: 0,
        });
      });
    }
  }

  /**
   * The states a person actually sees when something goes wrong, which are the
   * ones nobody looks at while building and therefore the ones that rot.
   */
  test("login with a rejected credential", async ({ page }) => {
    await page.route("**/api/v1/auth/login", async (route) => {
      await route.fulfill({
        status: 401,
        contentType: "application/json",
        body: JSON.stringify({
          error: {
            code: "UNAUTHENTICATED",
            message: "Those details did not sign you in.",
            retryable: false,
            field_errors: [],
            request_id: "req_visualbaseline",
          },
        }),
      });
    });

    await page.goto("/login");
    await hydrated(page);
    await page.getByLabel(/email/i).fill("daniel.okonkwo@example.com");
    await page.getByLabel(/^password$/i).fill("the-wrong-password");
    await page.getByRole("button", { name: /sign in/i }).click();

    // Scoped to the main landmark. Next injects its own role="alert" route
    // announcer into the document, so an unscoped query matches two elements
    // and fails on strict mode rather than on anything about the page.
    await expect(page.getByRole("main").getByRole("alert")).toBeVisible();
    await page.evaluate(() => document.fonts.ready);
    await settled(page);

    await expect(page).toHaveScreenshot("login-rejected.png", {
      fullPage: true,
      maxDiffPixels: 0,
    });
  });

  test("register showing a field error", async ({ page }) => {
    await page.route("**/api/v1/auth/register", async (route) => {
      await route.fulfill({
        status: 400,
        contentType: "application/json",
        body: JSON.stringify({
          error: {
            code: "VALIDATION_FAILED",
            message: "Some of the details were not accepted.",
            retryable: false,
            field_errors: [
              {
                field: "password",
                code: "PASSWORD_INVALID",
                message: "A password needs at least 12 characters.",
              },
            ],
            request_id: "req_visualbaseline",
          },
        }),
      });
    });

    await page.goto("/register");
    await hydrated(page);
    await page.getByLabel(/email/i).fill("daniel.okonkwo@example.com");
    await page.getByLabel(/^password$/i).fill("short");
    await page.getByRole("button", { name: /create/i }).click();

    await expect(page.getByLabel(/^password$/i)).toHaveAttribute(
      "aria-invalid",
      "true",
    );
    await page.evaluate(() => document.fonts.ready);
    // Register draws no provider buttons today, so this waits on nothing. It
    // is here because the rule is every screenshot, not every screenshot
    // somebody remembered: the last commit added the wait to the route loop
    // and missed its two siblings in the same file, and CI found it.
    await settled(page);

    await expect(page).toHaveScreenshot("register-field-error.png", {
      fullPage: true,
      maxDiffPixels: 0,
    });
  });
});

/**
 * Waits until no region on the page reports that it is busy.
 *
 * `aria-busy` is the same signal a screen reader uses to decide whether to
 * announce a region as still arriving, so a component that gets this right for
 * assistive technology gets it right for the camera at no extra cost. A
 * component that does not set it is invisible here, which is a reason to set it
 * rather than a reason to look for something else.
 */
async function settled(page: Page) {
  await page.waitForFunction(
    () => document.querySelectorAll('[aria-busy="true"]').length === 0,
    undefined,
    { timeout: 10_000 },
  );
}
