import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { QueryProvider } from "@/lib/api/QueryProvider";
import { ApiError } from "@/lib/api/client";

import { DecisionPanel } from "./DecisionPanel";
import * as api from "./api";

vi.mock("./api");

/**
 * REV-03 from the panel's side: no outcome without an outcome and a
 * reason, the request carries no decider for a body to forge, an override
 * travels only when stated, the server's refusals surface in their own
 * words, and the history renders every decision with its true actor.
 */

const decided: api.ReviewDecision = {
  id: "00000000-0000-7000-8000-00000000d001",
  session_id: "00000000-0000-7000-8000-00000000a001",
  decided_by: "00000000-0000-7000-8000-00000000ab01",
  decision: "hold",
  reason: "waiting on the take-home",
  evaluation_id: "00000000-0000-7000-8000-00000000ee01",
  result_digest: "sha256:result",
  rubric_digest: "sha256:rubric",
  overrides: [
    {
      competency_id: "sd",
      recorded_band: "strong",
      override_band: "solid",
      rationale: "one incident restated three times",
    },
  ],
  decided_at: "2026-09-04T09:00:00Z",
};

function renderPanel(history: api.ReviewDecision[] = []) {
  vi.mocked(api.fetchDecisions).mockResolvedValue(history);
  return render(
    <DecisionPanel
      campaignId="cmp-1"
      sessionId="ses-1"
      assessed={[{ competencyID: "sd", band: "strong" }]}
    />,
    {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryProvider>{children}</QueryProvider>
      ),
    },
  );
}

afterEach(() => {
  vi.mocked(api.fetchDecisions).mockReset();
  vi.mocked(api.recordDecision).mockReset();
});

describe("the decision panel", () => {
  it("records nothing without an outcome and a reason", async () => {
    renderPanel();
    const user = userEvent.setup();
    const record = screen.getByRole("button", { name: /record decision/i });

    expect(record).toBeDisabled();
    await user.click(screen.getByRole("radio", { name: /advance/i }));
    expect(record).toBeDisabled();
    await user.type(
      screen.getByRole("textbox", { name: /reason/i }),
      "evidenced throughout",
    );
    expect(record).toBeEnabled();
  });

  it("submits the reviewer's words, and no decider a body could forge", async () => {
    vi.mocked(api.recordDecision).mockResolvedValue(decided);
    renderPanel();
    const user = userEvent.setup();

    await user.click(screen.getByRole("radio", { name: /decline/i }));
    await user.type(
      screen.getByRole("textbox", { name: /reason/i }),
      "the contradictions stand unresolved",
    );
    await user.click(screen.getByRole("button", { name: /record decision/i }));

    await waitFor(() =>
      expect(api.recordDecision).toHaveBeenCalledWith("cmp-1", "ses-1", {
        decision: "reject",
        reason: "the contradictions stand unresolved",
      }),
    );
    // No overrides stated, none sent; no decider field exists at all.
    const request = vi.mocked(api.recordDecision).mock.calls[0]?.[2];
    expect(request).not.toHaveProperty("overrides");
    expect(request).not.toHaveProperty("decided_by");
  });

  it("an override travels only when stated, with its band and rationale", async () => {
    vi.mocked(api.recordDecision).mockResolvedValue(decided);
    renderPanel();
    const user = userEvent.setup();

    await user.click(screen.getByRole("radio", { name: /hold/i }));
    await user.type(
      screen.getByRole("textbox", { name: /^reason/i }),
      "holding for the band question",
    );
    await user.click(screen.getByText(/disagree with an assessed band/i));
    await user.type(screen.getByLabelText(/your band for sd/i), "solid");
    await user.type(
      screen.getByLabelText(/why you disagree on sd/i),
      "one incident restated",
    );
    await user.click(screen.getByRole("button", { name: /record decision/i }));

    await waitFor(() =>
      expect(api.recordDecision).toHaveBeenCalledWith("cmp-1", "ses-1", {
        decision: "hold",
        reason: "holding for the band question",
        overrides: [
          {
            competency_id: "sd",
            band: "solid",
            rationale: "one incident restated",
          },
        ],
      }),
    );
  });

  it("surfaces the server's refusal in its own words", async () => {
    vi.mocked(api.recordDecision).mockRejectedValue(
      new ApiError({
        status: 409,
        code: "OVERRIDE_INCOMPLETE",
        message:
          "An override needs the band you read and why. A disagreement without its reasoning is not recordable.",
      }),
    );
    renderPanel();
    const user = userEvent.setup();

    await user.click(screen.getByRole("radio", { name: /advance/i }));
    await user.type(screen.getByRole("textbox", { name: /reason/i }), "r");
    await user.click(screen.getByRole("button", { name: /record decision/i }));

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/without its reasoning is not recordable/i);
  });

  it("renders the history with each true actor, reason and override", async () => {
    renderPanel([decided]);

    expect(
      await screen.findByText(/waiting on the take-home/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/00000000-0000-7000-8000-00000000ab01/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        /read sd as solid where the evaluation assessed strong/i,
      ),
    ).toBeInTheDocument();
    expect(screen.getByText(/append-only/i)).toBeInTheDocument();
  });
});
