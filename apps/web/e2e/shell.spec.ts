import AxeBuilder from "@axe-core/playwright";

import { expect, setTheme, test } from "./fixtures";

/**
 * The application shell.
 *
 * The first screen where layout is genuinely hard: a sidebar that must go
 * off-canvas, a topbar that must not push content sideways, and a workspace
 * switcher that must fit at 320px. None of that is measurable in jsdom, which is
 * why the harness came first.
 *
 * The session is stubbed at the network rather than served by the API. What is
 * under test is the shell, and a real session would add a database to the
 * dependencies of a layout test.
 */

/** A recruiter in two workspaces, which is the widest the shell ever renders. */
const recruiter = {
  user_id: "01a0301d-aa10-7000-8f3e-1234567890ab",
  email: "daniel.okonkwo@example.com",
  email_verified: true,
  active_tenant_id: "01a0301d-aa10-7000-8f3e-00000000000a",
  memberships: [
    {
      tenant_id: "01a0301d-aa10-7000-8f3e-00000000000a",
      tenant_name: "Northwind Recruiting",
      status: "active",
    },
    {
      tenant_id: "01a0301d-aa10-7000-8f3e-00000000000b",
      tenant_name: "Orbital Labs",
      status: "active",
    },
  ],
  capabilities: [
    "candidate.practice.read_own",
    "campaign.read",
    "invitation.read",
    "tenant.member_manage",
    "tenant.settings_manage",
  ],
};

/** A candidate belonging to no workspace, which is the narrowest. */
const candidate = {
  ...recruiter,
  active_tenant_id: null,
  memberships: [],
  capabilities: ["candidate.practice.read_own"],
};

async function signedInAs(page: import("@playwright/test").Page, user: unknown) {
  await page.route("**/api/v1/me", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(user),
    });
  });
}

async function horizontalOverflow(page: import("@playwright/test").Page) {
  return page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
}

test.describe("the shell", () => {
  test("renders the destinations a recruiter holds", async ({ page }) => {
    await signedInAs(page, recruiter);
    await page.goto("/practice");

    await expect(page.getByRole("link", { name: "Campaigns" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Members" })).toBeVisible();
  });

  test("does not render a recruiter's destinations for a candidate", async ({ page }) => {
    await signedInAs(page, candidate);
    await page.goto("/practice");

    await expect(page.getByRole("main")).toBeVisible();
    await expect(page.getByRole("link", { name: "Campaigns" })).toHaveCount(0);
  });

  /**
   * The point of the whole tier. A sidebar that does not go off-canvas pushes
   * the content off the right of a small screen, and every page becomes
   * unusable rather than merely cramped.
   */
  test("does not scroll horizontally", async ({ page }) => {
    await signedInAs(page, recruiter);
    await page.goto("/practice");

    const { scrollWidth, clientWidth } = await horizontalOverflow(page);

    expect(scrollWidth, `document is ${scrollWidth}px in a ${clientWidth}px viewport`).toBeLessThanOrEqual(
      clientWidth,
    );
  });

  test("hides the sidebar behind a menu on a narrow viewport", async ({ page }, testInfo) => {
    test.skip(testInfo.project.name !== "narrow", "only meaningful at 320px");

    await signedInAs(page, recruiter);
    await page.goto("/practice");

    const menu = page.getByRole("button", { name: /menu/i });
    await expect(menu).toBeVisible();
    await expect(menu).toHaveAttribute("aria-expanded", "false");

    await menu.click();
    await expect(page.getByRole("navigation", { name: "Main" })).toBeInViewport();
  });

  test("shows the sidebar without a menu when there is room", async ({ page }, testInfo) => {
    test.skip(testInfo.project.name !== "desktop", "only meaningful at desktop width");

    await signedInAs(page, recruiter);
    await page.goto("/practice");

    await expect(page.getByRole("navigation", { name: "Main" })).toBeInViewport();
  });

  /**
   * A workspace name is user supplied and can be long. At 320px it is the string
   * most likely to push the layout sideways.
   */
  test("survives a long workspace name", async ({ page }) => {
    await signedInAs(page, {
      ...recruiter,
      memberships: recruiter.memberships.map((membership) => ({
        ...membership,
        tenant_name: `${membership.tenant_name} International Holdings and Partners Limited`,
      })),
    });
    await page.goto("/practice");

    const { scrollWidth, clientWidth } = await horizontalOverflow(page);

    expect(scrollWidth).toBeLessThanOrEqual(clientWidth);
  });

  test("survives text being doubled", async ({ page }) => {
    await signedInAs(page, recruiter);
    await page.goto("/practice");
    await page.addStyleTag({ content: "html { font-size: 200% !important; }" });

    const { scrollWidth, clientWidth } = await horizontalOverflow(page);

    expect(scrollWidth).toBeLessThanOrEqual(clientWidth);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  });

  for (const theme of ["dark", "light"] as const) {
    test(`has sufficient contrast in the ${theme} theme`, async ({ page }) => {
      await signedInAs(page, recruiter);
      await page.goto("/practice");
      await setTheme(page, theme);

      const results = await new AxeBuilder({ page })
        .withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
        .withRules(["color-contrast"])
        .analyze();

      expect(
        results.violations.flatMap((v) =>
          v.nodes.map((n) => ({ element: n.target.join(" "), problem: n.failureSummary })),
        ),
      ).toEqual([]);
    });
  }

  test("has no accessibility violations", async ({ page }) => {
    await signedInAs(page, recruiter);
    await page.goto("/practice");

    const results = await new AxeBuilder({ page })
      .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
      .analyze();

    expect(
      results.violations.map((v) => ({
        rule: v.id,
        impact: v.impact,
        nodes: v.nodes.map((n) => n.target.join(" ")),
      })),
    ).toEqual([]);
  });
});
