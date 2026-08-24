import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { axe } from "vitest-axe";

import { Banner } from "./Banner";

describe("Banner", () => {
  it("emits the prototype's tone class", () => {
    const { container } = render(<Banner tone="success">Password changed.</Banner>);

    expect(container.querySelector(".banner.banner-success")).toBeInTheDocument();
  });

  /**
   * A failure interrupts and anything else waits. Announcing a success as an
   * alert is how people learn to ignore alerts, which costs exactly when one
   * matters.
   */
  it("announces a failure as an alert", () => {
    render(<Banner tone="danger">Sign-in was cancelled.</Banner>);

    expect(screen.getByRole("alert")).toHaveTextContent("Sign-in was cancelled.");
  });

  it.each(["success", "warning", "info"] as const)("announces %s as a status, not an alert", (tone) => {
    render(<Banner tone={tone}>Something happened.</Banner>);

    expect(screen.getByRole("status")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("has no accessibility violations", async () => {
    const { container } = render(<Banner tone="danger">Sign-in was cancelled.</Banner>);

    expect(await axe(container)).toHaveNoViolations();
  });
});
