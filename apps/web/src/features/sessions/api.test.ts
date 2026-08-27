import { afterEach, describe, expect, it, vi } from "vitest";

import { apiFetch } from "@/lib/api/client";

import { listSessions } from "./api";

vi.mock("@/lib/api/client", () => ({ apiFetch: vi.fn() }));

/** The history read, unwrapped from its envelope. */

afterEach(() => {
  vi.mocked(apiFetch).mockReset();
});

describe("listSessions", () => {
  it("asks for the caller's own sessions and unwraps the envelope", async () => {
    const sessions = [{ id: "ses-7" }, { id: "ses-8" }];
    vi.mocked(apiFetch).mockResolvedValue({ sessions } as never);

    const answered = await listSessions();

    expect(apiFetch).toHaveBeenCalledWith("/me/sessions");
    expect(answered).toBe(sessions);
  });
});
