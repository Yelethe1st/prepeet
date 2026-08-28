import { test as base, expect } from "@playwright/test";

/**
 * The browser test fixture.
 *
 * Its one job is to stop the page moving while it is being measured. Every other
 * concern belongs in a test.
 */

/**
 * Motion is disabled for every browser test, and the reason is a real failure
 * rather than tidiness.
 *
 * The components carry colour transitions. Switching the theme and immediately
 * asking axe about contrast measured colours mid-interpolation: white text
 * halfway to the primary background reads as #97a2a2, which fails 4.5:1 and is
 * not a colour the product ever renders. The first run of the accessibility
 * suite reported three failures on that basis and none of them was real.
 *
 * The same movement makes a screenshot compare against whatever frame it caught.
 *
 * It is injected rather than set through prefers-reduced-motion, because that
 * would test the reduced-motion variant of the product rather than the product.
 */
const stillness = `
  *, *::before, *::after {
    transition: none !important;
    animation: none !important;
    scroll-behavior: auto !important;
    caret-color: transparent !important;
  }
`;

export const test = base.extend({
  page: async ({ page }, use) => {
    await page.addInitScript((css) => {
      const apply = () => {
        const style = document.createElement("style");
        style.setAttribute("data-test-stillness", "");
        style.textContent = css;
        document.head.append(style);
      };
      if (document.head) {
        apply();
      } else {
        document.addEventListener("DOMContentLoaded", apply);
      }
    }, stillness);

    await use(page);
  },
});

/**
 * setTheme switches theme and waits for the change to have taken effect.
 *
 * The wait is not a sleep: it reads back the resolved value of a token that
 * differs between themes, so it returns when the browser has actually
 * recalculated rather than after an interval somebody guessed.
 */
export async function setTheme(
  page: import("@playwright/test").Page,
  theme: "light" | "dark",
): Promise<void> {
  await page.evaluate((chosen) => {
    document.documentElement.setAttribute("data-theme", chosen);
  }, theme);

  await page.waitForFunction((chosen) => {
    const background = getComputedStyle(document.body).backgroundColor;
    // Both themes define a background; what matters is that the recalculation
    // has happened, which it has once the attribute and the computed value
    // agree about which theme is in force.
    return (
      document.documentElement.getAttribute("data-theme") === chosen &&
      background !== ""
    );
  }, theme);
}

/**
 * hydrated waits until React has taken over the server-rendered markup.
 *
 * Precautionary rather than fixing an observed failure, and worth saying so:
 * the first interactive test here failed for a different reason and this was
 * added while diagnosing it.
 *
 * It is kept because the race is real. These pages are statically built, so the
 * markup arrives before any handler is attached, and a click in that window goes
 * nowhere while the control is present and visible. That failure looks exactly
 * like a broken form.
 *
 * The check is that a controlled input actually responds, which is only true
 * once React is managing it. Typing into an unhydrated input leaves the value in
 * the DOM; typing into a hydrated controlled one round-trips through state, and
 * clearing it is what tells the two apart.
 */
export async function hydrated(
  page: import("@playwright/test").Page,
): Promise<void> {
  const probe = page.getByLabel(/email/i);
  await probe.waitFor({ state: "visible" });

  await expect(async () => {
    await probe.fill("hydration-probe");
    await expect(probe).toHaveValue("hydration-probe", { timeout: 250 });
    await probe.fill("");
    await expect(probe).toHaveValue("", { timeout: 250 });
  }).toPass({ timeout: 10_000 });
}

export { expect };
