import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import CompletePage from "./page";

vi.mock("@/features/complete/CompleteScreen", () => ({
  CompleteScreen: ({ sessionId }: { sessionId: string }) => (
    <p>status for {sessionId}</p>
  ),
}));

describe("the complete page", () => {
  it("hands the routed session id to the screen", async () => {
    render(await CompletePage({ params: Promise.resolve({ id: "ses-7" }) }));

    expect(
      screen.getByRole("heading", { name: /session finished/i }),
    ).toBeInTheDocument();
    expect(screen.getByText("status for ses-7")).toBeInTheDocument();
  });
});
