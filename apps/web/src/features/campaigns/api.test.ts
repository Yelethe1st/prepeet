import { afterEach, describe, expect, it, vi } from "vitest";

import { apiFetch } from "@/lib/api/client";

import { fetchRoster, listCampaigns } from "./api";

vi.mock("@/lib/api/client", () => ({ apiFetch: vi.fn() }));

/** The recruiter surface's calls: paths and the server-side filter. */

afterEach(() => {
  vi.mocked(apiFetch).mockReset();
});

describe("the recruiter surface's calls", () => {
  it("lists campaigns and unwraps the envelope", async () => {
    vi.mocked(apiFetch).mockResolvedValue({ campaigns: [] } as never);

    const campaigns = await listCampaigns();

    expect(apiFetch).toHaveBeenCalledWith("/campaigns");
    expect(campaigns).toEqual([]);
  });

  it("fetches the roster, with the standing sent to the server when chosen", async () => {
    vi.mocked(apiFetch).mockResolvedValue({
      pending_review: 0,
      candidates: [],
    } as never);

    await fetchRoster("cmp-1");
    expect(apiFetch).toHaveBeenLastCalledWith("/campaigns/cmp-1/candidates");

    await fetchRoster("cmp-1", "insufficient_evidence");
    // The filter is a query the server answers, never a rule this client
    // applies to rows it already holds.
    expect(apiFetch).toHaveBeenLastCalledWith(
      "/campaigns/cmp-1/candidates?standing=insufficient_evidence",
    );
  });
});

it("fetches the review from the audited route", async () => {
  const { fetchReview } = await import("./api");
  vi.mocked(apiFetch).mockResolvedValue({} as never);

  await fetchReview("cmp-1", "ses-1");

  expect(apiFetch).toHaveBeenLastCalledWith(
    "/campaigns/cmp-1/sessions/ses-1/review",
  );
});
