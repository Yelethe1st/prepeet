import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { axe } from "vitest-axe";

import type { ShellUser } from "./AppShell";
import { AppShell } from "./AppShell";

/**
 * The application shell.
 *
 * Navigation, the workspace switcher, and signing out. What is asserted is what
 * a person depends on: that they can reach every destination they hold, that
 * they are not offered ones they do not, and that the structure assistive
 * technology navigates by is present on every page.
 */

const signOut = vi.fn();
const switchTenant = vi.fn();
const push = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push }),
  usePathname: () => "/practice",
}));

beforeEach(() => {
  signOut.mockReset();
  signOut.mockResolvedValue(undefined);
  switchTenant.mockReset();
  switchTenant.mockResolvedValue(undefined);
  push.mockReset();
});

const candidate: ShellUser = {
  email: "daniel.okonkwo@example.com",
  activeTenantId: null,
  memberships: [],
  capabilities: ["candidate.practice.read_own", "session.create_practice"],
};

const recruiter: ShellUser = {
  ...candidate,
  activeTenantId: "t-northwind",
  memberships: [
    { tenantId: "t-northwind", tenantName: "Northwind Recruiting", status: "active" },
    { tenantId: "t-orbital", tenantName: "Orbital Labs", status: "active" },
  ],
  capabilities: [
    "candidate.practice.read_own",
    "campaign.read",
    "invitation.read",
    "tenant.member_manage",
  ],
};

function renderShell(user: ShellUser = candidate) {
  return render(
    <AppShell user={user} onSignOut={signOut} onSwitchTenant={switchTenant}>
      <h1>Practice</h1>
    </AppShell>,
  );
}

describe("AppShell", () => {
  it("renders the page inside the main landmark", () => {
    renderShell();

    expect(screen.getByRole("main")).toContainElement(screen.getByRole("heading", { level: 1 }));
  });

  /**
   * The skip link is the first thing a keyboard user meets and is invisible
   * until focused, which is why it is the control most often left broken.
   */
  it("offers a skip link to the content", () => {
    renderShell();

    expect(screen.getByRole("link", { name: /skip to main content/i })).toHaveAttribute(
      "href",
      "#main-content",
    );
  });

  it("names its navigation, so it can be skipped rather than walked", () => {
    renderShell();

    expect(screen.getByRole("navigation", { name: /main/i })).toBeInTheDocument();
  });

  // ───────────────────────────────────────────── what is offered

  it("offers a candidate their own practice", () => {
    renderShell();

    expect(screen.getByRole("link", { name: "Practice" })).toBeInTheDocument();
  });

  it("does not offer a candidate a recruiter's destinations", () => {
    renderShell();

    expect(screen.queryByRole("link", { name: "Campaigns" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Members" })).not.toBeInTheDocument();
  });

  it("offers a recruiter what their capabilities allow", () => {
    renderShell(recruiter);

    expect(screen.getByRole("link", { name: "Campaigns" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Invitations" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Members" })).toBeInTheDocument();
  });

  /**
   * Held capabilities decide, not membership. Somebody in a workspace without
   * the capability for a destination is not offered it, which is the difference
   * between a role-driven menu and a capability-driven one.
   */
  it("offers only what is held, not everything the workspace has", () => {
    renderShell({ ...recruiter, capabilities: ["campaign.read"] });

    expect(screen.getByRole("link", { name: "Campaigns" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Members" })).not.toBeInTheDocument();
  });

  it("marks the destination currently open", () => {
    renderShell();

    expect(screen.getByRole("link", { name: "Practice" })).toHaveAttribute("aria-current", "page");
  });

  // ───────────────────────────────────────────── the workspace

  it("offers the switcher to somebody in more than one workspace", () => {
    renderShell(recruiter);

    expect(screen.getByRole("combobox", { name: /workspace/i })).toBeInTheDocument();
  });

  it("does not offer the switcher to somebody in none", () => {
    renderShell();

    expect(screen.queryByRole("combobox", { name: /workspace/i })).not.toBeInTheDocument();
  });

  // ───────────────────────────────────────────── signing out

  it("offers a way to sign out", () => {
    renderShell();

    expect(screen.getByRole("button", { name: /sign out/i })).toBeInTheDocument();
  });

  it("signs out and sends the person to the sign-in screen", async () => {
    renderShell();

    await userEvent.click(screen.getByRole("button", { name: /sign out/i }));

    await waitFor(() => expect(signOut).toHaveBeenCalledOnce());
    expect(push).toHaveBeenCalledWith("/login");
  });

  /**
   * A failed sign-out still leaves the person believing they signed out, so the
   * safe thing is to send them away regardless: the session is revoked server
   * side or it is not, and staying on an authenticated screen is the worse of
   * the two outcomes.
   */
  it("sends the person away even when signing out failed", async () => {
    signOut.mockRejectedValue(new Error("network"));
    renderShell();

    await userEvent.click(screen.getByRole("button", { name: /sign out/i }));

    await waitFor(() => expect(push).toHaveBeenCalledWith("/login"));
  });

  it("names who is signed in, so a shared machine shows whose session it is", () => {
    renderShell();

    expect(screen.getByText("daniel.okonkwo@example.com")).toBeInTheDocument();
  });

  it("has no accessibility violations", async () => {
    const { container } = renderShell(recruiter);

    expect(await axe(container)).toHaveNoViolations();
  });
});
