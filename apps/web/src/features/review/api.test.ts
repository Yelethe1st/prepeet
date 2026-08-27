import { afterEach, describe, expect, it, vi } from "vitest";

import { apiFetch } from "@/lib/api/client";

import { createRedo, getReview, getTranscript } from "./api";

vi.mock("@/lib/api/client", () => ({ apiFetch: vi.fn() }));

/** The coaching review's reads, and the one write that creates a retake. */

afterEach(() => {
  vi.mocked(apiFetch).mockReset();
});

describe("the review reads", () => {
  it("asks for this session's review and transcript", async () => {
    vi.mocked(apiFetch).mockResolvedValue({} as never);

    await getReview("ses-2");
    await getTranscript("ses-2");

    expect(vi.mocked(apiFetch).mock.calls.map((call) => call[0])).toEqual([
      "/interviews/ses-2/review",
      "/interviews/ses-2/transcript",
    ]);
  });
});

describe("createRedo", () => {
  it("posts to the turn's own redos collection", async () => {
    vi.mocked(apiFetch).mockResolvedValue({ id: "ses-3" } as never);

    const created = await createRedo("ses-2", 5);

    expect(apiFetch).toHaveBeenCalledWith("/interviews/ses-2/turns/5/redos", {
      method: "POST",
    });
    expect(created).toEqual({ id: "ses-3" });
  });

  it("lets a refusal through untouched, so the screen can name it", async () => {
    const refusal = new Error("REDO_EXISTS");
    vi.mocked(apiFetch).mockRejectedValue(refusal);

    await expect(createRedo("ses-2", 5)).rejects.toBe(refusal);
  });
});
