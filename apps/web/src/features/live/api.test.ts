import { afterEach, describe, expect, it, vi } from "vitest";

import { apiFetch } from "@/lib/api/client";

import {
  completeInterview,
  getInterview,
  resumeInterview,
  sendEvents,
} from "./api";

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

  it("resumes with a bodyless post to the resume route", async () => {
    vi.mocked(apiFetch).mockResolvedValue({} as never);

    await resumeInterview("ses-6");

    expect(apiFetch).toHaveBeenCalledWith("/interviews/ses-6/resume", {
      method: "POST",
    });
  });

  it("sends a control batch in the contract's envelope and narrows the answer", async () => {
    vi.mocked(apiFetch).mockResolvedValue({
      connection_epoch: 2,
      accepted_sequence: 3,
      missing: [],
      outcomes: [{ event_id: "evt-1", status: "accepted" }],
    } as never);

    const events = [
      {
        event_id: "evt-1",
        sequence: 1,
        type: "connection.established",
        occurred_at: "2026-09-04T12:00:00.000Z",
      },
    ];
    const ack = await sendEvents("ses-6", 2, events);

    expect(apiFetch).toHaveBeenLastCalledWith("/interviews/ses-6/events", {
      method: "POST",
      body: { connection_epoch: 2, events },
    });
    // Exactly what the timeline settles against: the cursor and the
    // outcomes, nothing else to drift on.
    expect(ack).toEqual({
      accepted_sequence: 3,
      outcomes: [{ event_id: "evt-1", status: "accepted" }],
    });
  });
});
