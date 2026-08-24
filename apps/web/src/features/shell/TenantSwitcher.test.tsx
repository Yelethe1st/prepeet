import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { axe } from "vitest-axe";

import { TenantSwitcher } from "./TenantSwitcher";

/**
 * Choosing which workspace to act under.
 *
 * The control that makes IAM-03 usable. Everything about which tenant a request
 * runs under is decided on the server and stored on the session; this only asks
 * for a change and reflects the answer.
 */

const onSwitch = vi.fn();

beforeEach(() => {
  onSwitch.mockReset();
  onSwitch.mockResolvedValue(undefined);
});

const memberships = [
  { tenantId: "t-northwind", tenantName: "Northwind Recruiting", status: "active" },
  { tenantId: "t-orbital", tenantName: "Orbital Labs", status: "active" },
];

/** Opens the listbox. Radix renders the options only while it is open. */
async function open() {
  await userEvent.click(screen.getByRole("combobox"));
}

/** Opens the listbox and picks a workspace by its name. */
async function choose(name: string) {
  await open();
  await userEvent.click(await screen.findByRole("option", { name }));
}

function renderSwitcher(overrides: Partial<Parameters<typeof TenantSwitcher>[0]> = {}) {
  return render(
    <TenantSwitcher
      memberships={memberships}
      activeTenantId="t-northwind"
      onSwitch={onSwitch}
      {...overrides}
    />,
  );
}

describe("TenantSwitcher", () => {
  /**
   * Somebody who belongs to one workspace has no choice to make, and a control
   * offering one option is a control that only invites a misclick.
   */
  it("is not rendered for a person with a single workspace", () => {
    const { container } = renderSwitcher({ memberships: [memberships[0]!] });

    expect(container).toBeEmptyDOMElement();
  });

  it("is not rendered for a person with no workspace at all", () => {
    const { container } = renderSwitcher({ memberships: [], activeTenantId: null });

    expect(container).toBeEmptyDOMElement();
  });

  it("names the workspace currently being acted under", () => {
    renderSwitcher();

    expect(screen.getByRole("combobox")).toHaveTextContent("Northwind Recruiting");
  });

  it("lists every workspace the person belongs to once opened", async () => {
    renderSwitcher();

    await open();

    expect(await screen.findByRole("option", { name: "Northwind Recruiting" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Orbital Labs" })).toBeInTheDocument();
  });

  it("asks the server to switch when another is chosen", async () => {
    renderSwitcher();

    await choose("Orbital Labs");

    await waitFor(() => expect(onSwitch).toHaveBeenCalledWith("t-orbital"));
  });

  /**
   * Switching changes what the person may do, so the control must not accept a
   * second change while the first is in flight: two overlapping switches can
   * settle in either order, and the interface would then show authority for one
   * workspace while the session is in another.
   */
  it("cannot be changed again while a switch is in flight", async () => {
    let release: () => void = () => {};
    onSwitch.mockReturnValue(new Promise<void>((resolve) => { release = resolve; }));
    renderSwitcher();

    await choose("Orbital Labs");

    expect(screen.getByRole("combobox")).toBeDisabled();
    release();
    await waitFor(() => expect(screen.getByRole("combobox")).toBeEnabled());
  });

  /**
   * A refused switch must put the control back to what the session actually is,
   * or the interface claims a workspace the server never accepted.
   */
  it("returns to the current workspace when the switch is refused", async () => {
    onSwitch.mockRejectedValue(new Error("forbidden"));
    renderSwitcher();

    await choose("Orbital Labs");

    // The trigger still names the workspace the session is actually in, because
    // the value is the session's rather than whatever was clicked.
    await waitFor(() => expect(screen.getByRole("combobox")).toHaveTextContent("Northwind"));
  });

  it("says so when a switch is refused, rather than failing silently", async () => {
    onSwitch.mockRejectedValue(new Error("forbidden"));
    renderSwitcher();

    await choose("Orbital Labs");

    expect(await screen.findByRole("alert")).toBeInTheDocument();
  });

  /**
   * A revoked membership is listed by the server so the interface can explain
   * where a workspace went, and must not be offered as somewhere to act.
   */
  it("does not offer a workspace whose membership was revoked", async () => {
    renderSwitcher({
      memberships: [...memberships, { tenantId: "t-gone", tenantName: "Former Ltd", status: "revoked" }],
    });

    await open();

    await screen.findByRole("option", { name: "Northwind Recruiting" });
    expect(screen.queryByRole("option", { name: "Former Ltd" })).not.toBeInTheDocument();
  });

  it("has an accessible name, since a bare select says nothing about what it changes", () => {
    renderSwitcher();

    expect(screen.getByRole("combobox")).toHaveAccessibleName(/workspace/i);
  });

  it("has no accessibility violations", async () => {
    const { container } = renderSwitcher();

    expect(await axe(container)).toHaveNoViolations();
  });
});
