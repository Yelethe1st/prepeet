import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { QueryProvider } from "@/lib/api/QueryProvider";
import { ApiError } from "@/lib/api/client";

import { AppealsPanel } from "./AppealsPanel";
import * as api from "./api";

vi.mock("./api");

/**
 * REV-06 from the panel's side: raising needs a reason and carries no
 * requester for a body to forge, the freeze and the independence rule are
 * on the card in words, a resolution is whole before the button enables,
 * and the server's refusals surface as themselves.
 */

const open: api.ReReview = {
  id: "00000000-0000-7000-8000-00000000ee11",
  session_id: "00000000-0000-7000-8000-00000000a001",
  requested_by: "00000000-0000-7000-8000-00000000ab02",
  reason: "the second competency reads misjudged",
  appealed_decision: "00000000-0000-7000-8000-00000000d001",
  original_reviewer: "00000000-0000-7000-8000-00000000ab01",
  frozen: {
    evaluation_id: "00000000-0000-7000-8000-00000000ee01",
    result_digest: "sha256:result",
    rubric_digest: "sha256:rubric",
    bundle_digest: "sha256:bundle",
  },
  raised_at: "2026-09-05T09:00:00Z",
  due_at: "2026-09-12T09:00:00Z",
};

function renderPanel(appeals: api.ReReview[] = []) {
  vi.mocked(api.fetchAppeals).mockResolvedValue(appeals);
  return render(<AppealsPanel campaignId="cmp-1" sessionId="ses-1" />, {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryProvider>{children}</QueryProvider>
    ),
  });
}

afterEach(() => {
  vi.mocked(api.fetchAppeals).mockReset();
  vi.mocked(api.raiseAppeal).mockReset();
  vi.mocked(api.assignAppeal).mockReset();
  vi.mocked(api.resolveAppeal).mockReset();
});

describe("the re-review panel", () => {
  it("raises with a reason and nothing else", async () => {
    vi.mocked(api.raiseAppeal).mockResolvedValue(open);
    renderPanel();
    const user = userEvent.setup();

    const raise = screen.getByRole("button", { name: /raise re-review/i });
    expect(raise).toBeDisabled();
    await user.type(
      screen.getByRole("textbox", { name: /raise a re-review/i }),
      "please look again at the second competency",
    );
    await user.click(raise);

    await waitFor(() =>
      expect(api.raiseAppeal).toHaveBeenCalledWith(
        "cmp-1",
        "ses-1",
        "please look again at the second competency",
      ),
    );
  });

  it("shows the freeze and the independence rule on the card, in words", async () => {
    renderPanel([open]);

    expect(
      await screen.findByText(/frozen at raise: result sha256:result/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/original reviewer cannot answer this appeal/i),
    ).toBeInTheDocument();
    expect(screen.getByText(/awaiting assignment/i)).toBeInTheDocument();
    expect(screen.getByText(/answer due/i)).toBeInTheDocument();
  });

  it("resolves whole or not at all, and names the new-decision rule", async () => {
    const assigned = {
      ...open,
      assigned_to: "00000000-0000-7000-8000-00000000cafe",
    };
    vi.mocked(api.resolveAppeal).mockResolvedValue(assigned);
    renderPanel([assigned]);
    const user = userEvent.setup();

    const resolve = await screen.findByRole("button", { name: /^resolve$/i });
    expect(resolve).toBeDisabled();
    await user.selectOptions(
      screen.getByRole("combobox", { name: /outcome/i }),
      "revised",
    );
    expect(resolve).toBeDisabled();
    await user.type(
      screen.getByRole("textbox", { name: /resolution rationale/i }),
      "the misread span changes the band",
    );
    await user.type(
      screen.getByRole("textbox", { name: /candidate disclosure/i }),
      "Your re-review is complete; the outcome changed.",
    );
    expect(resolve).toBeEnabled();
    await user.click(resolve);

    await waitFor(() =>
      expect(api.resolveAppeal).toHaveBeenCalledWith("cmp-1", open.id, {
        outcome: "revised",
        rationale: "the misread span changes the band",
        disclosure: "Your re-review is complete; the outcome changed.",
      }),
    );
    expect(
      screen.getByText(/recorded as a new decision above, never an edit/i),
    ).toBeInTheDocument();
  });

  it("renders a resolved appeal with its recorded disclosure", async () => {
    renderPanel([
      {
        ...open,
        assigned_to: "00000000-0000-7000-8000-00000000cafe",
        resolution: {
          outcome: "upheld",
          rationale: "the evidence reads as recorded",
          candidate_disclosure:
            "Your re-review is complete; the outcome stands.",
          resolved_by: "00000000-0000-7000-8000-00000000cafe",
          resolved_at: "2026-09-06T09:00:00Z",
        },
      },
    ]);

    expect(
      await screen.findByText(/resolved: outcome upheld/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/candidate disclosure, recorded for delivery/i),
    ).toBeInTheDocument();
    // A resolved appeal offers no further controls: answered is answered.
    expect(
      screen.queryByRole("button", { name: /^resolve$/i }),
    ).not.toBeInTheDocument();
  });

  it("surfaces the server's refusal as itself", async () => {
    vi.mocked(api.assignAppeal).mockRejectedValue(
      new ApiError({
        status: 409,
        code: "SELF_REVIEW_FORBIDDEN",
        message: "The original reviewer cannot re-review their own decision.",
      }),
    );
    renderPanel([open]);
    const user = userEvent.setup();

    await user.type(
      await screen.findByLabelText(/assign to/i),
      "00000000-0000-7000-8000-00000000ab01",
    );
    await user.click(screen.getByRole("button", { name: /^assign$/i }));

    const card = await screen.findByRole("listitem");
    expect(within(card).getByRole("alert")).toHaveTextContent(
      /cannot re-review their own decision/i,
    );
  });
});
