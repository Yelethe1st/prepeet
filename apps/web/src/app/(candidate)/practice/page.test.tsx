import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { axe } from "vitest-axe";

import PracticePage from "./page";

vi.mock("@/features/sessions/SessionsScreen", () => ({
  SessionsScreen: () => <p>history</p>,
}));

/**
 * The practice destination: where signing in lands. The heading and the
 * wizard entry are this page's own; the history is SessionsScreen's and
 * has its own suite.
 */
describe("PracticePage", () => {
  it("has exactly one first-level heading", () => {
    render(<PracticePage />);

    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
  });

  it("says what the screen is for", () => {
    render(<PracticePage />);

    // The promise this screen exists to keep: practice is not visible to an
    // employer. Worth asserting even on a placeholder, because it is the line
    // most likely to be dropped when the real screen replaces it.
    expect(screen.getByText(/employer can see/i)).toBeInTheDocument();
  });

  it("has no accessibility violations", async () => {
    const { container } = render(<PracticePage />);

    expect(await axe(container)).toHaveNoViolations();
  });
});
