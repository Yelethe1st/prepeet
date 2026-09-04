import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import { QueryProvider } from "@/lib/api/QueryProvider";

import RosterPage from "./page";
import * as api from "@/features/campaigns/api";

vi.mock("@/features/campaigns/api");

/** The roster destination: heading and the not-a-ranking promise. */
describe("RosterPage", () => {
  it("has one first-level heading and disclaims ranking in its own words", async () => {
    vi.mocked(api.fetchRoster).mockReturnValue(new Promise(() => {}));
    const page = await RosterPage({
      params: Promise.resolve({ id: "cmp-1" }),
    });
    render(page, {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryProvider>{children}</QueryProvider>
      ),
    });

    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
    expect(
      screen.getByText(/not a ranking, and prepeet does not recommend/i),
    ).toBeInTheDocument();
  });
});
