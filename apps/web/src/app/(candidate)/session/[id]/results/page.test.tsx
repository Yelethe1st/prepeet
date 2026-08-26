import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import ResultsPage from "./page";

vi.mock("@/features/results/ResultsScreen", () => ({
  ResultsScreen: ({ sessionId }: { sessionId: string }) => (
    <p>screen for {sessionId}</p>
  ),
}));

describe("the results page", () => {
  it("hands the routed session id to the screen", async () => {
    render(await ResultsPage({ params: Promise.resolve({ id: "ses-9" }) }));

    expect(
      screen.getByRole("heading", { name: /outcome and evidence/i }),
    ).toBeInTheDocument();
    expect(screen.getByText("screen for ses-9")).toBeInTheDocument();
  });
});
