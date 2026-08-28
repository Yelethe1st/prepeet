import { render } from "@testing-library/react";
import { Check } from "lucide-react";
import { describe, expect, it } from "vitest";

import { Icon } from "./Icon";

describe("Icon", () => {
  /**
   * Every icon in this product is decoration and sits beside the words it
   * illustrates. Announced, it would be the label read twice, and this is the
   * one place that can be guaranteed for all of them.
   */
  it("hides itself from assistive technology", () => {
    const { container } = render(<Icon glyph={Check} />);

    expect(container.querySelector("svg")).toHaveAttribute(
      "aria-hidden",
      "true",
    );
    expect(container.querySelector("svg")).toHaveAttribute(
      "focusable",
      "false",
    );
  });

  /** Stroke weight tracks size, or a small glyph reads as a blob. */
  it("draws each size at its own weight", () => {
    const seen = new Set<string>();

    for (const size of ["sm", "md", "lg"] as const) {
      const { container, unmount } = render(<Icon glyph={Check} size={size} />);
      const svg = container.querySelector("svg")!;
      seen.add(
        `${svg.getAttribute("width")}/${svg.getAttribute("stroke-width")}`,
      );
      unmount();
    }

    expect(seen.size).toBe(3);
  });

  it("takes a colour when the icon is meant to read differently from its text", () => {
    const { container } = render(<Icon glyph={Check} tone="text-success" />);

    expect(container.querySelector("svg")).toHaveClass("text-success");
  });
});
