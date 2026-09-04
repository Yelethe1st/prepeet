import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import { QueryProvider } from "@/lib/api/QueryProvider";

import CampaignsPage from "./page";
import * as api from "@/features/campaigns/api";

vi.mock("@/features/campaigns/api");

/** The campaigns destination: heading and the fixed-configuration promise. */
describe("CampaignsPage", () => {
  it("has one first-level heading and names what a campaign fixes", () => {
    vi.mocked(api.listCampaigns).mockReturnValue(new Promise(() => {}));
    render(<CampaignsPage />, {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryProvider>{children}</QueryProvider>
      ),
    });

    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
    expect(
      screen.getByText(/every candidate.*sits the same interview/i),
    ).toBeInTheDocument();
  });
});
