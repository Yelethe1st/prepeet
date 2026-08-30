import { afterEach, describe, expect, it, vi } from "vitest";

import { apiFetch } from "@/lib/api/client";

import { completeInterview, getInterview } from "./api";

vi.mock("@/lib/api/client", () => ({ apiFetch: vi.fn() }));

/**
 * The completion flow's read and its one write. Completion carries the
 * cursor it seals at, so the body is the assertion that matters: sealing
 * at the wrong cursor is SEAL_CONFLICT or a truncated transcript.
 */

afterEach(() => {
  vi.mocked(apiFetch).mockReset();
});

describe("getInterview", () => {
  it("asks for the session", async () => {
    vi.mocked(apiFetch).mockResolvedValue({} as never);

    await getInterview("ses-5");

    expect(apiFetch).toHaveBeenCalledWith("/interviews/ses-5");
  });
});

describe("completeInterview", () => {
  it("seals at the epoch and sequence it was given", async () => {
    vi.mocked(apiFetch).mockResolvedValue({} as never);

    await completeInterview("ses-5", 2, 9);

    expect(apiFetch).toHaveBeenCalledWith("/interviews/ses-5/complete", {
      method: "POST",
      body: { connection_epoch: 2, final_sequence: 9 },
    });
  });
});
