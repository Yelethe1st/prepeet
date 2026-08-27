import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import DeliveryPage from "./page";

vi.mock("@/features/delivery/DeliveryScreen", () => ({
  DeliveryScreen: ({ sessionId }: { sessionId: string }) => (
    <p>delivery for {sessionId}</p>
  ),
}));

describe("the delivery page", () => {
  it("hands the routed session id to the screen", async () => {
    render(await DeliveryPage({ params: Promise.resolve({ id: "ses-d" }) }));
    expect(
      screen.getByRole("heading", { name: /^delivery$/i }),
    ).toBeInTheDocument();
    expect(screen.getByText("delivery for ses-d")).toBeInTheDocument();
  });
});
