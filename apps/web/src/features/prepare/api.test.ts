import { afterEach, describe, expect, it, vi } from "vitest";

import { apiFetch } from "@/lib/api/client";

import {
  fetchCatalogue,
  getInterview,
  getProfile,
  startInterview,
} from "./api";

vi.mock("@/lib/api/client", () => ({ apiFetch: vi.fn() }));

/** The prepare screen's reads, and the one write that starts the session. */

afterEach(() => {
  vi.mocked(apiFetch).mockReset();
});

describe("the prepare reads", () => {
  it("asks for the session and the caller's own profile", async () => {
    vi.mocked(apiFetch).mockResolvedValue({} as never);

    await getInterview("ses-1");
    await getProfile();

    expect(vi.mocked(apiFetch).mock.calls.map((call) => call[0])).toEqual([
      "/interviews/ses-1",
      "/me/profile",
    ]);
  });

  it("assembles the catalogue from its four collections", async () => {
    vi.mocked(apiFetch).mockImplementation(async (path: string) => {
      if (path.endsWith("disciplines")) return { disciplines: [] } as never;
      if (path.endsWith("roles")) return { roles: [{ id: "r" }] } as never;
      if (path.endsWith("interview-shapes")) return { shapes: [] } as never;
      return { personas: [] } as never;
    });

    const catalogue = await fetchCatalogue();

    expect(catalogue.roles[0]?.id).toBe("r");
    expect(vi.mocked(apiFetch)).toHaveBeenCalledTimes(4);
  });
});

describe("startInterview", () => {
  it("posts to the session's own start", async () => {
    vi.mocked(apiFetch).mockResolvedValue({} as never);

    await startInterview("ses-1");

    expect(apiFetch).toHaveBeenCalledWith("/interviews/ses-1/start", {
      method: "POST",
    });
  });

  it("lets a named refusal through for the screen to answer", async () => {
    const refusal = new Error("QUOTA_EXHAUSTED");
    vi.mocked(apiFetch).mockRejectedValue(refusal);

    await expect(startInterview("ses-1")).rejects.toBe(refusal);
  });
});
