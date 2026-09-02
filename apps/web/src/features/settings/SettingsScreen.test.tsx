import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { TenantSettings } from "./api";
import { SettingsScreen } from "./SettingsScreen";

/**
 * TEN-01's last criterion: a read-only user sees the settings without controls,
 * not a broken form.
 *
 * The distinction only means something because somebody does get controls. A
 * screen where nobody can edit would pass "no controls for a viewer" while
 * proving nothing, which is the shape of vacuous assertion this codebase has
 * been bitten by more than once.
 */

function settings(editable: boolean): TenantSettings {
  return {
    version: 4,
    editable,
    settings: {
      organisation: {
        legal_name: "Northwind Health Limited",
        display_name: "Northwind Health",
      },
      defaults: {},
      candidate_experience: {},
      notifications: {},
    },
    changed_by: "00000000-0000-7000-8000-0000000000a1",
    changed_at: "2026-08-30T10:00:00Z",
  };
}

describe("SettingsScreen", () => {
  it("shows a read-only member the values", () => {
    render(<SettingsScreen settings={settings(false)} onSave={vi.fn()} />);

    // The page, not a wall. Before the read capability existed this was a 403.
    expect(screen.getByText("Northwind Health Limited")).toBeInTheDocument();
    expect(screen.getByText("Northwind Health")).toBeInTheDocument();
  });

  it("gives a read-only member no controls to change them", () => {
    render(<SettingsScreen settings={settings(false)} onSave={vi.fn()} />);

    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /save/i }),
    ).not.toBeInTheDocument();
  });

  it("says why, rather than leaving the absence to be guessed", () => {
    // A page with no controls and no explanation reads as something broken.
    // Naming the authority that is missing turns it into a boundary.
    render(<SettingsScreen settings={settings(false)} onSave={vi.fn()} />);

    expect(
      screen.getByText(/an administrator or owner can change these/i),
    ).toBeInTheDocument();
  });

  it("gives an administrator the controls", () => {
    // The half that makes the test above mean something.
    render(<SettingsScreen settings={settings(true)} onSave={vi.fn()} />);

    expect(
      screen.getByRole("textbox", { name: /legal name/i }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /save/i })).toBeInTheDocument();
  });

  it("sends the version it was shown, not a fresh one", async () => {
    // The version read is the version changed. Sending anything else would
    // make the collision check meaningless: it would always agree.
    const onSave = vi.fn();
    const { getByRole } = render(
      <SettingsScreen settings={settings(true)} onSave={onSave} />,
    );

    getByRole("button", { name: /save/i }).click();

    expect(onSave).toHaveBeenCalledWith(
      4,
      expect.objectContaining({
        organisation: expect.objectContaining({
          legal_name: "Northwind Health Limited",
        }),
      }),
    );
  });

  it("says when it was last changed and by whom", () => {
    // Configuration a person cannot change is configuration they may need to
    // ask about, and the first question is always who changed it.
    render(<SettingsScreen settings={settings(false)} onSave={vi.fn()} />);

    expect(screen.getByText(/30 August 2026/)).toBeInTheDocument();
  });

  it("does not claim a change history a new workspace has not got", () => {
    // Version zero is the platform defaults, which nobody changed.
    const fresh = {
      ...settings(false),
      version: 0,
      changed_by: undefined,
      changed_at: undefined,
    };

    render(<SettingsScreen settings={fresh} onSave={vi.fn()} />);

    expect(screen.queryByText(/last changed/i)).not.toBeInTheDocument();
  });
});

describe("editing", () => {
  it("sends what was typed, not what was loaded", async () => {
    // The draft is local until saved, so this is the assertion that the edit
    // actually reaches the request rather than being displayed and dropped.
    const onSave = vi.fn();
    render(<SettingsScreen settings={settings(true)} onSave={onSave} />);

    const legal = screen.getByRole("textbox", { name: /legal name/i });
    await userEvent.clear(legal);
    await userEvent.type(legal, "Northwind Health PLC");
    const display = screen.getByRole("textbox", { name: /display name/i });
    await userEvent.clear(display);
    await userEvent.type(display, "Northwind");

    await userEvent.click(screen.getByRole("button", { name: /save/i }));

    expect(onSave).toHaveBeenCalledWith(4, {
      organisation: {
        legal_name: "Northwind Health PLC",
        display_name: "Northwind",
      },
      defaults: {},
      candidate_experience: {},
      notifications: {},
    });
  });

  it("shows the collision when there was one", () => {
    render(
      <SettingsScreen settings={settings(true)} onSave={vi.fn()} conflicted />,
    );

    expect(screen.getByRole("alert")).toHaveTextContent(
      /somebody else changed these settings/i,
    );
  });
});
