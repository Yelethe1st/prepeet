import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { QueryProvider } from "@/lib/api/QueryProvider";
import { ApiError } from "@/lib/api/client";

import { RosterScreen } from "./RosterScreen";
import * as api from "./api";

vi.mock("./api");

/**
 * REV-01's roster from the screen's side. What is pinned: standings render
 * as words in the server's own order, insufficient evidence is a named
 * standing in place rather than a bottom-of-list, the filter asks the
 * server, no column offers a quality sort, and the pending count stays the
 * campaign's truth under any filter.
 */

const roster: api.CampaignRoster = {
  pending_review: 2,
  candidates: [
    {
      invitation_id: "00000000-0000-7000-8000-00000000e001",
      recipient: "amara@example.com",
      standing: "awaiting_review",
      invited_at: "2026-09-01T10:00:00Z",
      session_id: "00000000-0000-7000-8000-00000000a001",
      submitted_at: "2026-09-04T09:00:00Z",
    },
    {
      invitation_id: "00000000-0000-7000-8000-00000000e002",
      recipient: "bela@example.com",
      standing: "insufficient_evidence",
      invited_at: "2026-09-01T11:00:00Z",
      session_id: "00000000-0000-7000-8000-00000000a002",
      submitted_at: "2026-09-04T10:00:00Z",
    },
    {
      invitation_id: "00000000-0000-7000-8000-00000000e003",
      recipient: "chen@example.com",
      standing: "invited",
      invited_at: "2026-09-02T10:00:00Z",
    },
  ],
};

function renderRoster(data: api.CampaignRoster = roster) {
  vi.mocked(api.fetchRoster).mockResolvedValue(data);
  return render(<RosterScreen campaignId="cmp-1" />, {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryProvider>{children}</QueryProvider>
    ),
  });
}

afterEach(() => {
  vi.mocked(api.fetchRoster).mockReset();
});

describe("the roster", () => {
  it("renders standings as words, in the server's own order, with the pending count", async () => {
    renderRoster();

    const table = await screen.findByRole("table");
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows).toHaveLength(3);

    // The server's order (invitation recency) survives untouched: the
    // insufficient-evidence candidate sits in place, named as exactly
    // that, never sorted to the bottom as a low scorer.
    expect(rows[0]).toHaveTextContent("amara@example.com");
    expect(rows[0]).toHaveTextContent("Awaiting review");
    expect(rows[1]).toHaveTextContent("bela@example.com");
    expect(rows[1]).toHaveTextContent("Insufficient evidence, awaiting review");
    expect(rows[2]).toHaveTextContent("chen@example.com");
    expect(rows[2]).toHaveTextContent("Invited");

    expect(screen.getByRole("status")).toHaveTextContent(
      /2 completed screenings await a human review/i,
    );
    // The roster is not a ranking, and the page says so in words.
    expect(
      screen.getByText(/not a ranking, and prepeet does not recommend/i),
    ).toBeInTheDocument();
  });

  it("offers no sortable column and no score anywhere", async () => {
    renderRoster();
    const table = await screen.findByRole("table");

    // The prototype's competency-signal sort is the one thing deliberately
    // not carried over: no header is a button, and no aria-sort exists.
    for (const header of within(table).getAllByRole("columnheader")) {
      expect(header).not.toHaveAttribute("aria-sort");
      expect(within(header).queryByRole("button")).not.toBeInTheDocument();
    }
    expect(
      screen.queryByText(/score|band|signal|confidence/i),
    ).not.toBeInTheDocument();
  });

  it("filters by asking the server, and the pending count stays the campaign's", async () => {
    renderRoster();
    await screen.findByRole("table");

    vi.mocked(api.fetchRoster).mockResolvedValue({
      pending_review: 2,
      candidates: [roster.candidates[1]!],
    });
    const user = userEvent.setup();
    await user.selectOptions(
      screen.getByRole("combobox", { name: /standing/i }),
      "insufficient_evidence",
    );

    await waitFor(() =>
      expect(api.fetchRoster).toHaveBeenLastCalledWith(
        "cmp-1",
        "insufficient_evidence",
      ),
    );
    await waitFor(() => expect(screen.getAllByRole("row")).toHaveLength(2));
    // Filtered to one row, the campaign still owes two reviews.
    expect(screen.getByRole("status")).toHaveTextContent(/2 completed/i);
  });

  it("an empty standing is the campaign's truth, said plainly", async () => {
    renderRoster({ pending_review: 0, candidates: [] });

    expect(
      await screen.findByText(/no candidates have been invited/i),
    ).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent(
      /nothing awaits review/i,
    );
  });

  it("a failed load names what failed and what is safe", async () => {
    vi.mocked(api.fetchRoster).mockRejectedValue(
      new ApiError({
        status: 400,
        message: "boom",
        requestId: "req-77",
      }),
    );
    render(<RosterScreen campaignId="cmp-1" />, {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryProvider>{children}</QueryProvider>
      ),
    });

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/roster could not be loaded/i);
    expect(alert).toHaveTextContent(/unaffected/i);
    expect(alert).toHaveTextContent("req-77");
  });
});
