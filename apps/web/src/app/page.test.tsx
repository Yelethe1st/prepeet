import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import Page, { metadata } from "./page";

/**
 * The marketing route.
 *
 * The page itself is covered in features/marketing, where it can be rendered
 * without a router. What is only true here is the composition and the metadata,
 * which is the copy a search result and a shared link show and which no
 * component test can see.
 */
describe("the marketing route", () => {
  it("renders the ported page", () => {
    render(<Page />);

    expect(
      screen.getByRole("heading", {
        level: 1,
        name: /Interviews that leave evidence/,
      }),
    ).toBeInTheDocument();
  });

  /** The prototype's own title and description, which are this page's first copy. */
  it("carries the title and description a link preview will show", () => {
    expect(metadata.title).toBe(
      "Prepeet · Voice interviews that leave evidence",
    );
    expect(metadata.description).toContain("practice with honest coaching");
    expect(metadata.description).toContain(
      "traceable to something the candidate actually said",
    );
  });
});
