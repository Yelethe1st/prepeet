import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { QueryProvider } from "@/lib/api/QueryProvider";
import { ApiError } from "@/lib/api/client";

import { SessionsScreen } from "./SessionsScreen";
import { MACHINE_STATES, STATE_ROWS } from "./states";
import * as api from "./api";
import type { InterviewSession } from "./api";

const replace = vi.fn();
let search = "";
vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace, push: replace }),
  useSearchParams: () => new URLSearchParams(search),
  usePathname: () => "/practice",
}));

vi.mock("./api");

/**
 * SES-07 from the outside: every state the machine can reach renders with
 * the one action that applies to it - the completeness the prototype gap
 * was about - and the filter lives in the URL, so a refresh keeps it.
 */

function sessionIn(state: string, id: string): InterviewSession {
  return {
    id,
    mode: "practice",
    state,
    config: {
      discipline: "software-engineering",
      role: "rl_swe",
      shape: "shape_technical",
      minutes: 40,
      persona: "per_ama",
    },
    recording_preference: "audio_and_transcript",
    consent_version: "1.0.0",
    created_at: "2026-08-27T10:00:00Z",
  };
}

function renderSessions(sessions: InterviewSession[]) {
  vi.mocked(api.listSessions).mockResolvedValue(sessions);
  return render(<SessionsScreen />, {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryProvider>{children}</QueryProvider>
    ),
  });
}

afterEach(() => {
  vi.mocked(api.listSessions).mockReset();
  replace.mockReset();
  search = "";
});

describe("completeness", () => {
  it("every lifecycle state the machine can reach renders with its own action", async () => {
    const sessions = MACHINE_STATES.map((state, index) =>
      sessionIn(
        state,
        `00000000-0000-7000-8000-0000000000${index.toString(16).padStart(2, "0")}`,
      ),
    );
    renderSessions(sessions);

    await screen.findByText(STATE_ROWS.review_ready.label);
    for (const state of MACHINE_STATES) {
      const spec = STATE_ROWS[state];
      const row = screen.getByTestId(`session-${state}`);
      expect(within(row).getByText(spec.label)).toBeInTheDocument();
      const action = within(row).getByRole("link", { name: spec.action });
      expect(action).toHaveAttribute(
        "href",
        spec.href(row.dataset.sessionId ?? ""),
      );
    }
  });
});

describe("filters", () => {
  it("reads the filter from the URL, so a refresh keeps it", async () => {
    search = "filter=attention";
    renderSessions([
      sessionIn("review_ready", "00000000-0000-7000-8000-0000000000a1"),
      sessionIn("evaluation_failed", "00000000-0000-7000-8000-0000000000a2"),
    ]);

    await screen.findByText(STATE_ROWS.evaluation_failed.label);
    expect(
      screen.queryByText(STATE_ROWS.review_ready.label),
    ).not.toBeInTheDocument();
  });

  it("choosing a filter writes it to the URL", async () => {
    renderSessions([
      sessionIn("review_ready", "00000000-0000-7000-8000-0000000000a1"),
    ]);
    const user = userEvent.setup();

    await screen.findByText(STATE_ROWS.review_ready.label);
    await user.click(screen.getByRole("tab", { name: /needs attention/i }));

    expect(replace).toHaveBeenCalledWith("/practice?filter=attention", {
      scroll: false,
    });
  });
});

describe("the empty history", () => {
  it("offers the wizard rather than a blank table", async () => {
    renderSessions([]);

    expect(
      await screen.findByText(/no practice sessions yet/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /start a practice interview/i }),
    ).toHaveAttribute("href", "/practice/new");
  });
});

describe("the states around the list", () => {
  it("a filter that matches nothing says so and keeps All available", async () => {
    search = "filter=attention";
    renderSessions([
      sessionIn("review_ready", "00000000-0000-7000-8000-0000000000a1"),
    ]);

    expect(
      await screen.findByText(/nothing under this filter/i),
    ).toHaveTextContent(/still exists under All/i);
  });

  it("choosing All clears the filter from the URL rather than naming it", async () => {
    search = "filter=finished";
    renderSessions([
      sessionIn("review_ready", "00000000-0000-7000-8000-0000000000a1"),
    ]);
    const user = userEvent.setup();

    await user.click(await screen.findByRole("tab", { name: /^all$/i }));

    expect(replace).toHaveBeenCalledWith("/practice", { scroll: false });
  });

  it("a state the machine gains later renders as itself with the safest action", async () => {
    renderSessions([
      sessionIn("quarantined", "00000000-0000-7000-8000-0000000000a3"),
    ]);

    const row = await screen.findByTestId("session-quarantined");
    expect(within(row).getByText("quarantined")).toBeInTheDocument();
    expect(
      within(row).getByRole("link", { name: /see status/i }),
    ).toHaveAttribute(
      "href",
      "/session/00000000-0000-7000-8000-0000000000a3/complete",
    );
  });

  it("a failure names what is safe and offers a retry", async () => {
    vi.mocked(api.listSessions).mockRejectedValue(
      new ApiError({ status: 500, message: "boom", requestId: "req_31" }),
    );
    render(<SessionsScreen />, {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryProvider>{children}</QueryProvider>
      ),
    });

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/req_31/);
    expect(alert).toHaveTextContent(/unaffected/i);
  });

  it("a failure code is shown beside the state that carries one", async () => {
    const failed = {
      ...sessionIn(
        "composition_failed",
        "00000000-0000-7000-8000-0000000000a4",
      ),
      failure_code: "FAILURE_CODE_ARTIFACT_NOT_FOUND",
    };
    renderSessions([failed]);

    const row = await screen.findByTestId("session-composition_failed");
    expect(
      within(row).getByText("FAILURE_CODE_ARTIFACT_NOT_FOUND"),
    ).toBeInTheDocument();
  });
});
