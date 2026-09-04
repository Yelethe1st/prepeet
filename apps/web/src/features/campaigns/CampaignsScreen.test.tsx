import { render, screen, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { QueryProvider } from "@/lib/api/QueryProvider";
import { ApiError } from "@/lib/api/client";

import { CampaignsScreen } from "./CampaignsScreen";
import * as api from "./api";

vi.mock("./api");

/**
 * The campaign list: the doorway to the roster. Statuses are words, each
 * row links to its candidates, and the empty workspace explains itself.
 */

const campaigns: api.Campaign[] = [
  {
    id: "00000000-0000-7000-8000-00000000c123",
    name: "ICU autumn intake",
    status: "open",
    role_reference: "role/icu-nurse",
    jurisdiction: "GB",
    created_at: "2026-08-20T10:00:00Z",
  },
  {
    id: "00000000-0000-7000-8000-00000000c124",
    name: "Backend hiring",
    status: "draft",
    role_reference: "role/backend",
    jurisdiction: "GB",
    created_at: "2026-08-22T10:00:00Z",
  },
];

function renderCampaigns(data: api.Campaign[] = campaigns) {
  vi.mocked(api.listCampaigns).mockResolvedValue(data);
  return render(<CampaignsScreen />, {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryProvider>{children}</QueryProvider>
    ),
  });
}

afterEach(() => {
  vi.mocked(api.listCampaigns).mockReset();
});

describe("the campaign list", () => {
  it("lists every campaign with its status in words and the way into its roster", async () => {
    renderCampaigns();

    const table = await screen.findByRole("table");
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent("ICU autumn intake");
    expect(rows[0]).toHaveTextContent("open");
    expect(
      within(rows[0] as HTMLElement).getByRole("link", {
        name: /candidates/i,
      }),
    ).toHaveAttribute(
      "href",
      "/campaigns/00000000-0000-7000-8000-00000000c123",
    );
  });

  it("an empty workspace explains what a campaign is", async () => {
    renderCampaigns([]);

    expect(await screen.findByText(/no campaigns yet/i)).toBeInTheDocument();
    expect(
      screen.getByText(/fixes one interview configuration/i),
    ).toBeInTheDocument();
  });

  it("a failed load names what failed with its reference", async () => {
    vi.mocked(api.listCampaigns).mockRejectedValue(
      new ApiError({ status: 400, message: "boom", requestId: "req-42" }),
    );
    render(<CampaignsScreen />, {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryProvider>{children}</QueryProvider>
      ),
    });

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/campaigns could not be loaded/i);
    expect(alert).toHaveTextContent("req-42");
  });
});
