import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { axe } from "vitest-axe";

import { AuthShell } from "./AuthShell";

/**
 * The shared authentication layout.
 *
 * Presentational, so what is asserted is the structure assistive technology
 * depends on rather than how it looks: the landmarks, and the skip link that
 * lets somebody using a keyboard reach the form without walking the side panel.
 */
describe("AuthShell", () => {
  it("offers a skip link to the form", () => {
    render(<AuthShell>form</AuthShell>);

    const skip = screen.getByRole("link", { name: /skip to main content/i });
    expect(skip).toHaveAttribute("href", "#main-content");
  });

  it("gives the skip link somewhere to land", () => {
    const { container } = render(<AuthShell>form</AuthShell>);

    // A skip link pointing at nothing is worse than none: it moves focus
    // nowhere and the person cannot tell whether it worked.
    expect(container.querySelector("#main-content")).toBeInTheDocument();
  });

  it("puts the form in the main landmark", () => {
    render(
      <AuthShell>
        <button type="button">Sign in</button>
      </AuthShell>,
    );

    expect(screen.getByRole("main")).toContainElement(
      screen.getByRole("button"),
    );
  });

  it("names the side panel, so it is skippable rather than anonymous", () => {
    render(<AuthShell>form</AuthShell>);

    expect(screen.getByRole("complementary")).toHaveAccessibleName(
      /why people practise/i,
    );
  });

  it("has no accessibility violations", async () => {
    const { container } = render(
      <AuthShell>
        <h1>Sign in</h1>
      </AuthShell>,
    );

    expect(await axe(container)).toHaveNoViolations();
  });
});
