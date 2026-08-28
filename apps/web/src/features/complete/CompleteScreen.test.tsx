import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { QueryProvider } from "@/lib/api/QueryProvider";

import { CompleteScreen } from "./CompleteScreen";
import * as api from "./api";
import type { InterviewSession } from "./api";

vi.mock("./api");

/**
 * SES-08 from the outside: the receipt is the server's durable session
 * read, so it survives leaving and returning; delayed and failed are
 * different states with different roles; a failure names its next action;
 * and no copy promises a completion time.
 */

function session(overrides: Partial<InterviewSession>): InterviewSession {
  return {
    id: "00000000-0000-7000-8000-0000000000e1",
    mode: "practice",
    state: "evaluating",
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
    ...overrides,
  };
}

function renderComplete(value: InterviewSession) {
  vi.mocked(api.getInterview).mockResolvedValue(value);
  return render(
    <CompleteScreen sessionId="00000000-0000-7000-8000-0000000000e1" />,
    {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryProvider>{children}</QueryProvider>
      ),
    },
  );
}

afterEach(() => {
  vi.mocked(api.getInterview).mockReset();
});

describe("processing", () => {
  it("shows the honest stages without promising a completion time", async () => {
    renderComplete(
      session({
        state: "evaluating",
        seal: {
          sealed_at: "2026-08-27T10:40:00Z",
          media_status: "finalized",
          warnings: [],
        },
      }),
    );

    expect(await screen.findByText(/being evaluated/i)).toBeInTheDocument();
    expect(screen.getByText(/transcript sealed/i)).toBeInTheDocument();
    // Leaving is safe and said so; no minutes or seconds are promised.
    expect(screen.getByText(/safe to leave this page/i)).toBeInTheDocument();
    expect(document.body.textContent).not.toMatch(
      /\b(usually|within) .*(minute|second)/i,
    );
  });
});

describe("the receipt", () => {
  it("survives returning later: the durable seal renders from the session read", async () => {
    renderComplete(
      session({
        state: "review_ready",
        seal: {
          sealed_at: "2026-08-27T10:40:00Z",
          media_status: "none_by_choice",
          warnings: [],
        },
      }),
    );

    expect(
      await screen.findByText(/your results are ready/i),
    ).toBeInTheDocument();
    // The transcript-only choice reads as the choice it was.
    expect(screen.getByText(/no audio, by your choice/i)).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /outcome and evidence/i }),
    ).toHaveAttribute(
      "href",
      "/session/00000000-0000-7000-8000-0000000000e1/results",
    );
    expect(
      screen.getByRole("link", { name: /coaching review/i }),
    ).toHaveAttribute(
      "href",
      "/session/00000000-0000-7000-8000-0000000000e1/review",
    );
  });

  it("a missing recording is stated on the receipt, never pretended", async () => {
    renderComplete(
      session({
        state: "review_ready",
        seal: {
          sealed_at: "2026-08-27T10:40:00Z",
          media_status: "missing",
          warnings: ["MEDIA_MISSING"],
        },
      }),
    );

    expect(
      await screen.findByText(/recording did not arrive/i),
    ).toBeInTheDocument();
  });
});

describe("failure", () => {
  it("is an alert naming the code and a next action, distinct from delay", async () => {
    renderComplete(
      session({
        state: "evaluation_failed",
        failure_code: "FAILURE_CODE_ARTIFACT_NOT_FOUND",
        seal: {
          sealed_at: "2026-08-27T10:40:00Z",
          media_status: "finalized",
          warnings: [],
        },
      }),
    );

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/FAILURE_CODE_ARTIFACT_NOT_FOUND/);
    expect(alert).toHaveTextContent(/transcript and evidence are safe/i);
    // The next action is concrete: the reference to quote.
    expect(alert).toHaveTextContent(/00000000-0000-7000-8000-0000000000e1/);
  });

  it("a session still live reads as live, not as a failure", async () => {
    renderComplete(session({ state: "in_progress" }));

    expect(await screen.findByText(/still in progress/i)).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
