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
      /the story this was built from/i,
    );
  });

  /**
   * The panel is the product's argument, and every part of it says something a
   * person signing in is entitled to know: whose story this is, how much of it
   * has been run, and where the data lives.
   */
  it("carries the story, the numbers and what is promised", () => {
    render(<AuthShell>form</AuthShell>);

    expect(
      screen.getByText(/Nineteen graduate applications/),
    ).toBeInTheDocument();
    expect(screen.getByText(/Kelvin Onouha/)).toBeInTheDocument();
    expect(screen.getByText("184,600")).toBeInTheDocument();
    expect(
      screen.getByText("Practice data never reaches employers"),
    ).toBeInTheDocument();
  });

  /** Both authentication screens get the brand and the footer from here. */
  it("puts the brand and the footer around the form", () => {
    render(<AuthShell>form</AuthShell>);

    expect(screen.getByRole("link", { name: "Prepeet home" })).toHaveAttribute(
      "href",
      "/",
    );
    expect(
      screen.getByRole("link", { name: /trouble accessing a workspace/i }),
    ).toHaveAttribute("href", "/no-workspace");
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
