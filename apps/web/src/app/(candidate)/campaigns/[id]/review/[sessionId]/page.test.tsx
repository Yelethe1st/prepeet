import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import { QueryProvider } from "@/lib/api/QueryProvider";

import ReviewPage from "./page";
import * as api from "@/features/campaigns/api";

vi.mock("@/features/campaigns/api");

/** The review destination: heading, and the audit promise up front. */
describe("ReviewPage", () => {
  it("has one first-level heading and says the read is recorded", async () => {
    vi.mocked(api.fetchReview).mockReturnValue(new Promise(() => {}));
    const page = await ReviewPage({
      params: Promise.resolve({ id: "cmp-1", sessionId: "ses-1" }),
    });
    render(page, {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryProvider>{children}</QueryProvider>
      ),
    });

    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
    expect(screen.getByText(/recorded in the audit log/i)).toBeInTheDocument();
  });
});
