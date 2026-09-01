import type { Capability } from "@contracts/capabilities";

/**
 * What the application offers, and to whom.
 *
 * One configuration rather than a menu built into each surface, so the sidebar,
 * the mobile navigation and anything else that lists destinations agree without
 * being kept in step by hand.
 *
 * Every item is gated on a capability, and the capability names come from the
 * generated catalogue, so renaming one breaks this file at compile time rather
 * than silently hiding a menu item for everybody, forever.
 *
 * # What this is not
 *
 * It is not access control. The server authorises every request against the
 * same capabilities, so somebody who types an address they cannot use is
 * refused there. Filtering here exists so that nobody is offered a control that
 * will refuse them, which is a courtesy rather than a boundary. Treating it as a
 * boundary is how a product ends up with an unprotected endpoint behind a
 * hidden button.
 */

export interface NavigationItem {
  label: string;
  href: string;
  /** The capability that reveals this item. Every item has one. */
  capability: Capability;
}

export interface NavigationGroup {
  label: string;
  items: NavigationItem[];
}

/**
 * The destinations, in the order they are shown.
 *
 * Short, because most of the product is not built. An entry is added when the
 * screen it points at exists, so the navigation never offers a route that is not
 * there.
 */
export const NAVIGATION: readonly NavigationGroup[] = [
  {
    label: "Practice",
    items: [
      {
        label: "Practice",
        href: "/practice",
        capability: "candidate.practice.read_own",
      },
      {
        label: "Skills",
        href: "/skills",
        capability: "candidate.practice.read_own",
      },
      {
        label: "Profile",
        href: "/profile",
        capability: "candidate.profile.read_own",
      },
    ],
  },
  {
    label: "Recruiting",
    items: [
      { label: "Campaigns", href: "/campaigns", capability: "campaign.read" },
      {
        label: "Invitations",
        href: "/invitations",
        capability: "invitation.read",
      },
    ],
  },
  {
    label: "Workspace",
    items: [
      {
        label: "Members",
        href: "/workspace/members",
        // member_read, not manage: a read-only member sees who belongs to
        // the workspace; the screen itself withholds the controls.
        capability: "tenant.member_read",
      },
      {
        label: "Settings",
        href: "/workspace/settings",
        capability: "tenant.settings_manage",
      },
    ],
  },
];

/**
 * visibleNavigation returns the groups and items this session may be offered.
 *
 * A group whose items are all hidden is dropped rather than rendered empty: a
 * heading over nothing reads as something that failed to load, which sends
 * people to support for a screen that is working exactly as intended.
 */
export function visibleNavigation(held: readonly string[]): NavigationGroup[] {
  const holds = new Set(held);

  return NAVIGATION.map((group) => ({
    label: group.label,
    items: group.items.filter((item) => holds.has(item.capability)),
  })).filter((group) => group.items.length > 0);
}
