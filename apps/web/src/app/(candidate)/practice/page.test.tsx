import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { axe } from "vitest-axe";

import PracticePage from "./page";

/**
 * The practice destination.
 *
 * A placeholder until SES-07 ports the real screen, and tested anyway: it is
 * where signing in lands, so an empty or unheaded page here is the first thing
 * a new person sees.
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
