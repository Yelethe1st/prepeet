import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { axe } from "vitest-axe";

import {
  LoadingSurface,
  SkeletonBlock,
  SkeletonCircle,
  SkeletonText,
} from "./index";

/**
 * Loading, as a shape rather than a spinner.
 *
 * The ticket's second box: skeletons match the shape of the content they
 * replace. That cannot be asserted in general - the shape is each surface's
 * own - so what the shared pieces guarantee instead is the half screens get
 * wrong: the loading state is announced once, by name, and the decorative
 * shimmer is invisible to assistive technology rather than read as a page of
 * nothing.
 */

describe("LoadingSurface", () => {
  it("announces what is loading, by name", () => {
    render(
      <LoadingSurface label="your sessions">
        <SkeletonText />
        <SkeletonText width="75" />
      </LoadingSurface>,
    );

    expect(screen.getByRole("status")).toHaveTextContent(
      /loading your sessions/i,
    );
  });

  it("marks the surface busy and hides the shapes from assistive technology", () => {
    const { container } = render(
      <LoadingSurface label="your sessions">
        <SkeletonText />
      </LoadingSurface>,
    );

    expect(screen.getByRole("status")).toHaveAttribute("aria-busy", "true");
    const shapes = container.querySelector("[aria-hidden='true']");
    expect(shapes).not.toBeNull();
    expect(shapes).toContainElement(container.querySelector(".skeleton"));
  });

  it("has no axe violations", async () => {
    const { container } = render(
      <LoadingSurface label="your profile">
        <SkeletonCircle />
        <SkeletonText width="50" />
        <SkeletonBlock />
      </LoadingSurface>,
    );

    expect(await axe(container)).toHaveNoViolations();
  });
});

describe("the pieces", () => {
  /**
   * The prototype's widths are the vocabulary (full, 75, 50, 25); a skeleton
   * line that could be any width is a skeleton that stops matching the text
   * it stands in for.
   */
  it("renders each text width differently", () => {
    const widths = ["full", "75", "50", "25"] as const;
    const seen = new Set<string>();

    for (const width of widths) {
      const { container, unmount } = render(<SkeletonText width={width} />);
      seen.add((container.firstChild as HTMLElement).className);
      unmount();
    }

    expect(seen.size).toBe(widths.length);
  });

  it("keeps circle and block distinguishable from text", () => {
    const text = render(<SkeletonText />).container.firstChild as HTMLElement;
    const circle = render(<SkeletonCircle />).container
      .firstChild as HTMLElement;
    const block = render(<SkeletonBlock />).container.firstChild as HTMLElement;

    expect(circle.className).not.toBe(text.className);
    expect(block.className).not.toBe(text.className);
  });
});
