import { defineConfig, devices } from "@playwright/test";

/**
 * Browser tests.
 *
 * These exist because jsdom, which every other test in this package runs in, has
 * no layout engine and does not apply stylesheets. A 900px box measures zero
 * there, and a colour set by a class comes back as the browser default. Three
 * things therefore cannot be checked without a real browser, and all three are
 * requirements rather than nice to have:
 *
 *   Contrast as rendered. The token-level check in src/test/theme.test.ts
 *   computes ratios for the pairs somebody thought of. A component putting quiet
 *   text on a raised surface is a pair nobody thought of.
 *
 *   Layout at 320px. The stylesheet check in src/test/tailwind.test.ts
 *   catches a fixed width somebody typed. It cannot catch a long email address
 *   with no break opportunity, which overflows without any width being declared.
 *
 *   Visual regression. Nothing else compares what a screen looks like to what it
 *   looked like, so a stylesheet edit could move everything and every test would
 *   still pass.
 *
 * They are separate from the vitest suite rather than folded into it, because
 * they cost seconds rather than milliseconds and need a server. Vitest collects
 * only src/, so nothing here is picked up twice.
 */

/**
 * Chromium flags that make text render the same way twice.
 *
 * Without them an unchanged page differs from its own baseline by around 130
 * pixels between runs, purely from subpixel antialiasing, and that noise floor
 * is larger than many real changes: squaring the corners off a button alters
 * about thirty pixels. A visual suite whose noise exceeds its signal reports
 * nothing useful in either direction.
 */
const launchOptions = {
  args: [
    // Greyscale antialiasing instead of subpixel, which is what varies.
    "--disable-lcd-text",
    // No hinting, so glyph shapes do not depend on font cache state.
    "--font-render-hinting=none",
    // Rendering that does not depend on the GPU the run happened to get.
    "--disable-gpu",
    "--force-color-profile=srgb",
  ],
};

/** Where the application under test is. */
const baseURL = process.env.PREPEET_WEB_URL ?? "http://localhost:3100";

export default defineConfig({
  testDir: "./e2e",
  // Deliberately not the default 30s. A browser test that takes longer than
  // this is stuck rather than slow, and a long timeout turns a hang into a wait
  // nobody watches.
  timeout: 20_000,
  expect: { timeout: 5_000 },

  // Fail the run if a test was committed with .only, which passes locally and
  // silently stops everything else from running in CI.
  forbidOnly: Boolean(process.env.CI),

  // Retry in CI only. Locally a flake should be seen and fixed; in CI a single
  // retry tells a flake from a failure without hiding either, because the
  // report says a test was retried.
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 2 : undefined,

  reporter: process.env.CI ? [["github"], ["list"]] : [["list"]],

  use: {
    baseURL,
    // Kept only for a failure, so a passing run leaves nothing behind and a
    // failing one leaves enough to see what happened without rerunning it.
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },

  projects: [
    {
      name: "desktop",
      use: { ...devices["Desktop Chrome"], launchOptions },
    },
    {
      // The narrowest viewport the product supports. 320 is not arbitrary: it
      // is the smallest width in common use, and the width a phone reports when
      // somebody has enlarged their system text.
      name: "narrow",
      use: {
        ...devices["Desktop Chrome"],
        viewport: { width: 320, height: 640 },
        launchOptions,
      },
    },
  ],

  // Built and served rather than run in development mode. Development serves
  // CSS through a different pipeline and injects an overlay, and a visual
  // baseline taken against that is a baseline of something nobody ships.
  //
  // reuseExistingServer is off everywhere, including locally, and that is the
  // most important line in this file.
  //
  // With it on, a server left running from an earlier build is reused, and every
  // assertion is then made against code that is no longer in the repository.
  // That is not theoretical: while writing this suite, a stale server made a
  // deliberately broken stylesheet pass at zero tolerance, twice, and the
  // conclusion drawn each time was about the tolerance rather than about the
  // server. A suite that can silently test something other than the working tree
  // is worse than no suite, because it reports success.
  //
  // The cost is a build per run. Next caches, so it is seconds.
  webServer: {
    command: "pnpm build && pnpm start --port 3100",
    url: baseURL,
    reuseExistingServer: false,
    timeout: 180_000,
  },
});
