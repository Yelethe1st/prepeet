import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { axe } from "vitest-axe";

import Page from "./page";

/**
 * The marketing route.
 *
 * A placeholder until WEB-06 ports it, and tested because it is the first thing
 * anybody sees at the bare address and the only page reachable without a
 * session.
 */
describe("the marketing page", () => {
  it("has exactly one first-level heading", () => {
    render(<Page />);

    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
  });

  it("has no accessibility violations", async () => {
    const { container } = render(<Page />);

    expect(await axe(container)).toHaveNoViolations();
  });
});
