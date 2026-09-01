import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import type { SkillHistory } from "./api";
import { SkillsScreen } from "./SkillsScreen";

/**
 * PRG-04's three criteria, and nothing else.
 *
 * Every competency expands to the evidence behind it with its date; stale
 * evidence says so rather than being counted as current; and the chart carries
 * a text summary and a table, because a picture is not a reading for everyone.
 *
 * The distinction the whole progression context turns on shows up here too: a
 * competency nobody has been asked about is not a competency somebody answered
 * badly, and a screen is where that difference is most easily lost.
 */

const history: SkillHistory = {
  competencies: [
    {
      competency_id: "systems-design",
      name: "Systems design",
      discipline: "engineering",
      role: "backend",
      standing: "fresh",
      band: "strong",
      evidence: [
        {
          observed_at: "2026-08-20T10:00:00Z",
          age_days: 12,
          standing: "fresh",
          band: "strong",
          rubric_reference: "rubric/backend",
          rubric_version: "3.0.0",
        },
        {
          observed_at: "2026-04-02T10:00:00Z",
          age_days: 152,
          standing: "stale",
          band: "solid",
          rubric_reference: "rubric/backend",
          rubric_version: "2.1.0",
        },
      ],
    },
    {
      competency_id: "incident-response",
      name: "Incident response",
      discipline: "engineering",
      role: "backend",
      standing: "stale",
      band: "solid",
      evidence: [
        {
          observed_at: "2026-03-01T10:00:00Z",
          age_days: 184,
          standing: "stale",
          band: "solid",
          rubric_reference: "rubric/backend",
          rubric_version: "2.1.0",
        },
      ],
    },
    {
      competency_id: "code-review",
      name: "Code review",
      discipline: "engineering",
      role: "backend",
      standing: "none",
      evidence: [],
    },
  ],
};

describe("SkillsScreen", () => {
  it("expands a competency to the evidence behind it, with dates", async () => {
    render(<SkillsScreen history={history} />);

    await userEvent.click(
      screen.getByRole("button", { name: /Systems design/ }),
    );

    const evidence = screen.getByRole("region", { name: /Systems design/ });
    expect(within(evidence).getByText(/20 August 2026/)).toBeInTheDocument();
    expect(within(evidence).getByText(/3\.0\.0/)).toBeInTheDocument();
  });

  it("keeps the evidence collapsed until it is asked for", () => {
    // The dates are the detail; the standing is the answer. A screen that
    // opened every row would bury the reading it exists to give.
    render(<SkillsScreen history={history} />);

    expect(
      screen.queryByRole("region", { name: /Systems design/ }),
    ).not.toBeInTheDocument();
  });

  it("says a reading is stale rather than letting it pass as current", () => {
    render(<SkillsScreen history={history} />);

    const row = screen.getByRole("button", { name: /Incident response/ });

    // Not merely a different colour: colour alone is not a reading, and the
    // word is what a screen reader and a printout both carry.
    expect(within(row).getByText(/stale/i)).toBeInTheDocument();
  });

  it("does not render an unobserved competency as a weak one", () => {
    render(<SkillsScreen history={history} />);

    const row = screen.getByRole("button", { name: /Code review/ });

    // Never measured and measured badly are different facts. A band here, or a
    // zero, would turn a question nobody asked into an answer somebody failed.
    expect(within(row).getByText(/not yet assessed/i)).toBeInTheDocument();
    expect(within(row).queryByText(/developing|solid|strong/i)).toBeNull();
  });

  it("offers no evidence to expand where there is none", async () => {
    render(<SkillsScreen history={history} />);

    const row = screen.getByRole("button", { name: /Code review/ });
    await userEvent.click(row);

    expect(
      screen.getByText(/no sessions have covered this yet/i),
    ).toBeInTheDocument();
  });

  it("gives the chart a text summary", () => {
    // PRG-04's third criterion, and A11Y's: a chart that only exists as a
    // picture is a chart some people cannot read at all.
    render(<SkillsScreen history={history} />);

    expect(
      screen.getByText(/1 fresh, 1 stale, 1 not yet assessed/i),
    ).toBeInTheDocument();
  });

  it("gives the chart a table alternative", () => {
    render(<SkillsScreen history={history} />);

    const table = screen.getByRole("table", { name: /competency standings/i });
    expect(
      within(table).getByRole("row", { name: /Systems design/ }),
    ).toBeInTheDocument();
  });

  it("says so plainly when there is nothing yet", () => {
    // A first session is the common case for a new candidate, and an empty
    // grid reads as a broken screen rather than as a beginning.
    render(<SkillsScreen history={{ competencies: [] }} />);

    expect(
      screen.getByText(
        /once you have practised, your competencies appear here/i,
      ),
    ).toBeInTheDocument();
  });
});
