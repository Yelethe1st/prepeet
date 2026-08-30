import { afterEach, describe, expect, it, vi } from "vitest";

import { apiFetch } from "@/lib/api/client";

import { completeInterview, getInterview } from "./api";

vi.mock("@/lib/api/client", () => ({ apiFetch: vi.fn() }));

/** The live shell's exit: read the cursor, seal at it. */

afterEach(() => {
  vi.mocked(apiFetch).mockReset();
});

describe("the live shell's exit", () => {
  it("reads the session and seals at the cursor it carries", async () => {
    vi.mocked(apiFetch).mockResolvedValue({} as never);

    await getInterview("ses-6");
    await completeInterview("ses-6", 1, 4);

    expect(vi.mocked(apiFetch).mock.calls[0]?.[0]).toBe("/interviews/ses-6");
    expect(apiFetch).toHaveBeenLastCalledWith("/interviews/ses-6/complete", {
      method: "POST",
      body: { connection_epoch: 1, final_sequence: 4 },
    });
  });
});
