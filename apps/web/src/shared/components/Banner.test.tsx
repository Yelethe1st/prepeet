import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { axe } from "vitest-axe";

import { Banner } from "./Banner";

describe("Banner", () => {
  /**
   * Each tone renders differently, without naming the utilities. A tone that
   * looked like another would be a tone that says nothing, and which colours it
   * uses is settled by the browser suite against a screenshot.
   */
  it("renders each tone differently", () => {
    const seen = new Set<string>();

    for (const tone of ["success", "danger", "warning", "info"] as const) {
      const { container, unmount } = render(
        <Banner tone={tone}>Something happened.</Banner>,
      );
      seen.add((container.firstChild as HTMLElement).className);
      unmount();
    }

    expect(seen.size).toBe(4);
  });

  /**
   * A failure interrupts and anything else waits. Announcing a success as an
   * alert is how people learn to ignore alerts, which costs exactly when one
   * matters.
   */
  it("announces a failure as an alert", () => {
    render(<Banner tone="danger">Sign-in was cancelled.</Banner>);

    expect(screen.getByRole("alert")).toHaveTextContent(
      "Sign-in was cancelled.",
    );
  });

  it.each(["success", "warning", "info"] as const)(
    "announces %s as a status, not an alert",
    (tone) => {
      render(<Banner tone={tone}>Something happened.</Banner>);

      expect(screen.getByRole("status")).toBeInTheDocument();
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    },
  );

  it("has no accessibility violations", async () => {
    const { container } = render(
      <Banner tone="danger">Sign-in was cancelled.</Banner>,
    );

    expect(await axe(container)).toHaveNoViolations();
  });
});
