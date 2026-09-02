import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import SkillsPage from "./page";

vi.mock("@/features/skills/SkillsSection", () => ({
  SkillsSection: () => <div data-testid="skills-section" />,
}));

describe("the skills page", () => {
  it("says the evidence carries its date and its rubric", () => {
    render(<SkillsPage />);

    expect(
      screen.getByText(
        /the date it was measured and the rubric that judged it/i,
      ),
    ).toBeInTheDocument();
  });

  it("gives the section a labelled region to live in", () => {
    render(<SkillsPage />);

    expect(
      screen.getByRole("region", { name: /your competencies/i }),
    ).toBeInTheDocument();
  });
});
