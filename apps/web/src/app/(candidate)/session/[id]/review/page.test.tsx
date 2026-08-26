import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import ReviewPage from "./page";

vi.mock("@/features/review/ReviewScreen", () => ({
  ReviewScreen: ({ sessionId }: { sessionId: string }) => (
    <p>review for {sessionId}</p>
  ),
}));

describe("the review page", () => {
  it("hands the routed session id to the screen", async () => {
    render(await ReviewPage({ params: Promise.resolve({ id: "ses-5" }) }));

    expect(
      screen.getByRole("heading", { name: /coaching review/i }),
    ).toBeInTheDocument();
    expect(screen.getByText("review for ses-5")).toBeInTheDocument();
  });
});
