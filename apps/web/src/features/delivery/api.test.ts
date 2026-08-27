import { afterEach, describe, expect, it, vi } from "vitest";

import { apiFetch } from "@/lib/api/client";

import { getBaseline, getDelivery, getInterview, getTranscript } from "./api";

vi.mock("@/lib/api/client", () => ({ apiFetch: vi.fn() }));

/**
 * The delivery screen's reads. The baseline is the caller's own and takes
 * no session: asking for it per session would be asking the wrong question.
 */

afterEach(() => {
  vi.mocked(apiFetch).mockReset();
});

describe("the delivery reads", () => {
  it("asks for the session's analysis, transcript and session", async () => {
    vi.mocked(apiFetch).mockResolvedValue({} as never);

    await getDelivery("ses-4");
    await getTranscript("ses-4");
    await getInterview("ses-4");

    expect(vi.mocked(apiFetch).mock.calls.map((call) => call[0])).toEqual([
      "/interviews/ses-4/delivery",
      "/interviews/ses-4/transcript",
      "/interviews/ses-4",
    ]);
  });

  it("asks for the baseline as the caller's own, not the session's", async () => {
    vi.mocked(apiFetch).mockResolvedValue({} as never);

    await getBaseline();

    expect(apiFetch).toHaveBeenCalledWith("/me/delivery-baseline");
  });
});
