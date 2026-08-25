import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import { QueryProvider } from "@/lib/api/QueryProvider";

import NewPracticeInterviewPage from "./page";
import * as api from "@/features/interview/api";

vi.mock("@/features/interview/api");

/**
 * The wizard's destination: the page decodes the URL into the wizard's
 * starting position, which is the addressable half of CAT-04's first box.
 */
describe("NewPracticeInterviewPage", () => {
  async function renderAt(parameters: Record<string, string>): Promise<void> {
    vi.mocked(api.fetchCatalogue).mockResolvedValue({
      disciplines: [{ id: "d", name: "D" }],
      roles: [
        {
          id: "rl_x",
          discipline: "d",
          title: "Role X",
          organisation: "O",
          competencies: [],
          shapes: ["s"],
        },
      ],
      shapes: [{ id: "s", name: "Shape S", description: "x", minutes: [15] }],
      personas: [
        {
          id: "p",
          name: "Ama",
          style: "warm",
          voice: "v",
          description: "x",
          best_for: "b",
          shapes: [],
        },
      ],
    });
    const page = await NewPracticeInterviewPage({
      searchParams: Promise.resolve(parameters),
    });
    render(page, {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryProvider>{children}</QueryProvider>
      ),
    });
  }

  it("has exactly one first-level heading", async () => {
    await renderAt({});

    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
  });

  it("restores the addressed step from the URL", async () => {
    await renderAt({ step: "2", role: "rl_x" });

    expect(
      await screen.findByRole("heading", { name: /interview shape/i }),
    ).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: /shape s/i })).toBeInTheDocument();
  });
});
