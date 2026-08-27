import { afterEach, describe, expect, it, vi } from "vitest";

import { apiFetch } from "@/lib/api/client";

import { getInterview, getResults, getTranscript } from "./api";

vi.mock("@/lib/api/client", () => ({ apiFetch: vi.fn() }));

/**
 * The outcome screen's reads. What is worth asserting in a thin wrapper is
 * the address: a wrong path or verb here is a real defect that no screen
 * test catches, because every screen test mocks this module.
 */

afterEach(() => {
  vi.mocked(apiFetch).mockReset();
});

describe("the outcome reads", () => {
  it("asks for the session's own results, transcript and session", async () => {
    vi.mocked(apiFetch).mockResolvedValue({} as never);

    await getResults("ses-1");
    await getTranscript("ses-1");
    await getInterview("ses-1");

    expect(vi.mocked(apiFetch).mock.calls.map((call) => call[0])).toEqual([
      "/interviews/ses-1/results",
      "/interviews/ses-1/transcript",
      "/interviews/ses-1",
    ]);
  });

  it("answers exactly what the server said", async () => {
    const result = { session_id: "ses-1" };
    vi.mocked(apiFetch).mockResolvedValue(result as never);

    await expect(getResults("ses-1")).resolves.toBe(result);
  });
});
