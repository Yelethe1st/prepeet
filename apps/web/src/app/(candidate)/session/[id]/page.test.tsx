import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import LivePage from "./page";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
}));

/**
 * The live destination: without a stashed grant it lands on the named
 * recovery, which is also the page's whole safety story for a stale URL.
 */
describe("LivePage", () => {
  it("has one first-level heading and names the missing-pass recovery", async () => {
    sessionStorage.clear();
    const page = await LivePage({ params: Promise.resolve({ id: "ses-9" }) });
    render(page);

    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
    expect(await screen.findByText(/no live pass/i)).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /prepare screen/i }),
    ).toBeInTheDocument();
  });
});
