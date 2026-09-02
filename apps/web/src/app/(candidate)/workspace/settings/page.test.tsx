import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import WorkspaceSettingsPage from "./page";

vi.mock("@/features/settings/SettingsSection", () => ({
  SettingsSection: () => <div data-testid="settings-section" />,
}));

describe("the workspace settings page", () => {
  it("names what changes and what does not", () => {
    // The one thing a person must understand before editing: a change reaches
    // campaigns opened afterwards, never one already running.
    render(<WorkspaceSettingsPage />);

    expect(
      screen.getByText(/never to ones already running/i),
    ).toBeInTheDocument();
  });

  it("gives the section a labelled region to live in", () => {
    render(<WorkspaceSettingsPage />);

    expect(
      screen.getByRole("region", { name: /workspace settings/i }),
    ).toBeInTheDocument();
  });
});
