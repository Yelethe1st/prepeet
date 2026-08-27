import { afterEach, describe, expect, it, vi } from "vitest";

import { apiFetch } from "@/lib/api/client";

import { createInterview, fetchCatalogue, fetchPracticeConsent } from "./api";

vi.mock("@/lib/api/client", () => ({ apiFetch: vi.fn() }));

/**
 * The wizard's calls. The catalogue is four reads assembled into one shape,
 * so what matters is that all four are asked for and each collection lands
 * in its own field: a mis-assembled catalogue would offer roles as shapes.
 */

afterEach(() => {
  vi.mocked(apiFetch).mockReset();
});

describe("fetchCatalogue", () => {
  it("asks for every collection and keeps each in its own field", async () => {
    vi.mocked(apiFetch).mockImplementation(async (path: string) => {
      switch (path) {
        case "/catalog/disciplines":
          return { disciplines: [{ id: "d" }] } as never;
        case "/catalog/roles":
          return { roles: [{ id: "r" }] } as never;
        case "/catalog/interview-shapes":
          return { shapes: [{ id: "s" }] } as never;
        default:
          return { personas: [{ id: "p" }] } as never;
      }
    });

    const catalogue = await fetchCatalogue();

    expect(
      vi
        .mocked(apiFetch)
        .mock.calls.map((call) => call[0])
        .sort(),
    ).toEqual([
      "/catalog/disciplines",
      "/catalog/interview-shapes",
      "/catalog/personas",
      "/catalog/roles",
    ]);
    expect(catalogue.disciplines[0]?.id).toBe("d");
    expect(catalogue.roles[0]?.id).toBe("r");
    expect(catalogue.shapes[0]?.id).toBe("s");
    expect(catalogue.personas[0]?.id).toBe("p");
  });
});

describe("createInterview", () => {
  it("posts the selection as the body", async () => {
    vi.mocked(apiFetch).mockResolvedValue({ id: "ses-1" } as never);
    const request = {
      mode: "practice" as const,
      discipline: "software-engineering",
      role: "rl_swe",
      shape: "shape_technical",
      minutes: 40,
      persona: "per_ama",
      recording: {
        preference: "transcript_only" as const,
        consent_version: "1.0.0",
      },
    };

    await createInterview(request);

    expect(apiFetch).toHaveBeenCalledWith("/interviews", {
      method: "POST",
      body: request,
    });
  });
});

describe("fetchPracticeConsent", () => {
  it("asks for the currently published consent text", async () => {
    vi.mocked(apiFetch).mockResolvedValue({} as never);

    await fetchPracticeConsent();

    expect(apiFetch).toHaveBeenCalledWith("/interviews/practice-consent");
  });
});
