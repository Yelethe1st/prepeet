import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { ApiError } from "@/lib/api/client";
import * as liveApi from "@/features/live/api";

import LivePage from "./page";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
}));
vi.mock("@/features/live/api");

/**
 * The live destination: a stale URL with no stashed grant asks the server
 * to resume (RTC-03's front door back in), and a session with nothing
 * running lands on the named recovery.
 */
describe("LivePage", () => {
  it("has one first-level heading and names the missing-pass recovery", async () => {
    sessionStorage.clear();
    vi.mocked(liveApi.resumeInterview).mockRejectedValue(
      new ApiError({
        status: 409,
        code: "SESSION_NOT_RESUMABLE",
        message: "No interview is in flight to resume.",
      }),
    );

    const page = await LivePage({ params: Promise.resolve({ id: "ses-9" }) });
    render(page);

    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
    expect(await screen.findByText(/no live pass/i)).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /prepare screen/i }),
    ).toBeInTheDocument();
  });
});
