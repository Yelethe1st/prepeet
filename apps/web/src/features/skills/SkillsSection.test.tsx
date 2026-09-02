import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { SkillsSection } from "./SkillsSection";

/**
 * The three states a person actually meets: waiting, failed, and the reading.
 *
 * SkillsScreen's own tests cover what the reading says. These cover the two
 * states that are easy to leave until a real outage produces them, and the
 * failure wording in particular: a candidate who cannot load this screen needs
 * to know their practice history is intact, because the obvious fear is that it
 * is not.
 */

const fetchMock = vi.fn();

beforeEach(() => {
  fetchMock.mockReset();
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function renderSection() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return render(<SkillsSection />, { wrapper });
}

describe("SkillsSection", () => {
  it("says what it is waiting for", async () => {
    fetchMock.mockImplementation(() => new Promise(() => {}));

    renderSection();

    expect(await screen.findByText(/your competencies/i)).toBeInTheDocument();
  });

  it("renders the reading once it arrives", async () => {
    fetchMock.mockResolvedValue(
      new Response(
        JSON.stringify({
          competencies: [
            {
              competency_id: "systems-design",
              name: "Systems design",
              discipline: "engineering",
              role: "backend",
              standing: "fresh",
              band: "strong",
              evidence: [],
            },
          ],
        }),
        { status: 200, headers: { "content-type": "application/json" } },
      ),
    );

    renderSection();

    expect(
      await screen.findByRole("button", { name: /Systems design/ }),
    ).toBeInTheDocument();
  });

  it("reassures the reader that the failure is only the view", async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ code: "INTERNAL", message: "nope" }), {
        status: 500,
        headers: { "content-type": "application/json" },
      }),
    );

    renderSection();

    // The fear on this screen is that the history is gone, not that a request
    // failed. Saying so is the difference between an error and an alarm.
    expect(
      await screen.findByText(
        /nothing about your practice history has changed/i,
      ),
    ).toBeInTheDocument();
  });

  it("offers a way to try again", async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ code: "INTERNAL", message: "nope" }), {
        status: 500,
        headers: { "content-type": "application/json" },
      }),
    );

    renderSection();

    expect(
      await screen.findByRole("button", { name: /try again/i }),
    ).toBeInTheDocument();
  });
});

describe("SkillsSection recovery", () => {
  it("asks again when told to", async () => {
    // The retry is the only thing on the failure state a person can do, so it
    // is the one part of it worth proving works.
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ code: "INTERNAL", message: "nope" }), {
        status: 500,
        headers: { "content-type": "application/json" },
      }),
    );

    renderSection();
    const retry = await screen.findByRole("button", { name: /try again/i });
    const before = fetchMock.mock.calls.length;
    retry.click();

    await waitFor(() =>
      expect(fetchMock.mock.calls.length).toBeGreaterThan(before),
    );
  });
});
