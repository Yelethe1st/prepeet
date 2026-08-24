import { describe, expect, it } from "vitest";

import { NAVIGATION, visibleNavigation } from "./navigation";

/**
 * Navigation is rendered from capabilities.
 *
 * The rule that matters is stated in WEB-02 and is easy to get backwards:
 * hiding an item is never the only thing stopping access. The server authorises
 * every request against the same capability set, so a person who reaches a route
 * by typing its address is refused there. What this filtering is for is not
 * offering somebody a control that will refuse them.
 *
 * That distinction is why these tests assert what is shown and never assert
 * that hiding protects anything. The protection is tested in the Go suite.
 */

describe("visibleNavigation", () => {
  it("shows nothing to a session holding nothing", () => {
    const groups = visibleNavigation([]);

    expect(groups).toEqual([]);
  });

  it("shows an item once its capability is held", () => {
    const groups = visibleNavigation(["candidate.practice.read_own"]);

    const labels = groups.flatMap((group) => group.items.map((item) => item.label));
    expect(labels).toContain("Practice");
  });

  it("does not show an item whose capability is not held", () => {
    const groups = visibleNavigation(["candidate.practice.read_own"]);

    const labels = groups.flatMap((group) => group.items.map((item) => item.label));
    expect(labels).not.toContain("Campaigns");
  });

  /**
   * A group with nothing visible in it is a heading over an empty space, which
   * reads as something failing to load.
   */
  it("drops a group once every item in it is hidden", () => {
    const groups = visibleNavigation(["candidate.practice.read_own"]);

    for (const group of groups) {
      expect(group.items.length).toBeGreaterThan(0);
    }
  });

  it("keeps the declared order rather than the order capabilities arrived in", () => {
    const forwards = visibleNavigation(["campaign.read", "candidate.practice.read_own"]);
    const backwards = visibleNavigation(["candidate.practice.read_own", "campaign.read"]);

    expect(forwards).toEqual(backwards);
  });

  /**
   * An owner holds far more than a candidate, and the difference is the whole
   * point: the same component renders a recruiter's workspace and a candidate's
   * practice history without either knowing about the other.
   */
  it("shows a recruiter more than a candidate", () => {
    const candidate = visibleNavigation(["candidate.practice.read_own", "session.create_practice"]);
    const recruiter = visibleNavigation([
      "candidate.practice.read_own",
      "session.create_practice",
      "campaign.read",
      "invitation.read",
      "tenant.member_manage",
    ]);

    const count = (groups: typeof candidate) =>
      groups.reduce((total, group) => total + group.items.length, 0);

    expect(count(recruiter)).toBeGreaterThan(count(candidate));
  });
});

describe("the navigation configuration", () => {
  /**
   * Every item is gated. An item with no capability would be visible to
   * everybody including somebody holding nothing, which is how a recruiter
   * control ends up on a candidate's screen.
   */
  it("gates every item on a capability", () => {
    for (const group of NAVIGATION) {
      for (const item of group.items) {
        expect(item.capability, `${item.label} is not gated`).toBeTruthy();
      }
    }
  });

  /**
   * Names come from the generated catalogue, so a capability that is renamed or
   * removed breaks the build here rather than silently hiding a menu item
   * forever. This is the assertion that makes that true at runtime as well.
   */
  it("names only capabilities the catalogue defines", async () => {
    const { CAPABILITIES } = await import("@contracts/capabilities");

    for (const group of NAVIGATION) {
      for (const item of group.items) {
        expect(CAPABILITIES as readonly string[]).toContain(item.capability);
      }
    }
  });

  it("gives every item a distinct destination", () => {
    const destinations = NAVIGATION.flatMap((group) => group.items.map((item) => item.href));

    expect(new Set(destinations).size).toBe(destinations.length);
  });

  it("gives every group a label, since a heading is what makes a group one", () => {
    for (const group of NAVIGATION) {
      expect(group.label).toBeTruthy();
    }
  });
});
