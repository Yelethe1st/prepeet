import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import { QueryProvider } from "@/lib/api/QueryProvider";

import PreparePage from "./page";
import * as api from "@/features/prepare/api";

vi.mock("@/features/prepare/api");

/** The prepare destination: heading, promise, and the screen underneath. */
describe("PreparePage", () => {
  it("has one first-level heading and states the not-recording promise", async () => {
    vi.mocked(api.getInterview).mockReturnValue(new Promise(() => {}));
    vi.mocked(api.getProfile).mockReturnValue(new Promise(() => {}));
    vi.mocked(api.fetchCatalogue).mockReturnValue(new Promise(() => {}));

    const page = await PreparePage({
      params: Promise.resolve({ id: "ses-1" }),
    });
    render(page, {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryProvider>{children}</QueryProvider>
      ),
    });

    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
    // The line most likely to be dropped in a redesign, asserted for that
    // reason: nothing is recorded before start.
    expect(
      screen.getByText(/nothing is recorded until you press start/i),
    ).toBeInTheDocument();
  });
});
