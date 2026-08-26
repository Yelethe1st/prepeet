import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { QueryProvider } from "@/lib/api/QueryProvider";
import { ApiError } from "@/lib/api/client";

import { PrepareScreen } from "./PrepareScreen";
import * as api from "./api";
import type { CheckRunners } from "./checks";
import type { CheckStatus } from "./gate";

vi.mock("./api");

const push = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push, replace: push }),
}));

/**
 * The prepare screen, from the outside: SES-03's three boxes. Start is
 * disabled until the microphone passes and required consent is given; the
 * blocked state names the one missing thing and moves focus to it; and the
 * optional model-improvement consent is separate, off, and never dragged
 * along by the required one. The check runners are faked - their hardware
 * behaviour is checks.ts's own concern - so what is tested is the screen's
 * behaviour around their answers.
 */

const session = {
  id: "00000000-0000-7000-8000-0000000000e1",
  mode: "practice" as const,
  state: "ready",
  config: {
    discipline: "software-engineering",
    role: "rl_swe",
    shape: "shape_technical",
    minutes: 40,
    persona: "per_ravi",
  },
  recording_preference: "audio_and_transcript" as const,
  consent_version: "1.0.0",
  created_at: "2026-08-26T10:00:00Z",
};

const catalogue = {
  disciplines: [{ id: "software-engineering", name: "Software engineering" }],
  roles: [
    {
      id: "rl_swe",
      discipline: "software-engineering",
      title: "Senior Backend Engineer",
      organisation: "Product company",
      competencies: ["Systems design", "Debugging"],
      shapes: ["shape_technical"],
    },
  ],
  shapes: [
    {
      id: "shape_technical",
      name: "Technical deep-dive",
      description: "Verbal reasoning.",
      minutes: [25, 40],
    },
  ],
  personas: [
    {
      id: "per_ravi",
      name: "Ravi",
      style: "Direct and technical",
      voice: "Brisk, precise",
      description: "Moves quickly, probes trade-offs.",
      best_for: "Senior loops",
      shapes: [],
    },
  ],
};

const profile = {
  disciplines: [],
  target_roles: [],
  seniority: "",
  career_context: "",
  default_duration_minutes: 0,
  default_style: "",
  default_pressure: undefined,
  extended_time: true,
  captions: true,
  reduced_motion: false,
  accessibility_notes: "",
  notify_product_updates: false,
  notify_practice_reminders: false,
};

function runners(
  overrides?: Partial<CheckRunners> & { micResult?: CheckStatus },
): CheckRunners {
  return {
    mic:
      overrides?.mic ?? (() => Promise.resolve(overrides?.micResult ?? "pass")),
    speaker: overrides?.speaker ?? (() => Promise.resolve()),
    net: overrides?.net ?? (() => Promise.resolve("pass")),
    browser: overrides?.browser ?? (() => "pass"),
  };
}

function renderPrepare(overrides?: Parameters<typeof runners>[0]) {
  vi.mocked(api.getInterview).mockResolvedValue(session);
  vi.mocked(api.getProfile).mockResolvedValue(profile);
  vi.mocked(api.fetchCatalogue).mockResolvedValue(catalogue);
  return render(
    <PrepareScreen sessionId={session.id} runners={runners(overrides)} />,
    {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryProvider>{children}</QueryProvider>
      ),
    },
  );
}

afterEach(() => {
  vi.mocked(api.getInterview).mockReset();
  vi.mocked(api.getProfile).mockReset();
  vi.mocked(api.fetchCatalogue).mockReset();
  vi.mocked(api.startInterview).mockReset();
  push.mockReset();
  sessionStorage.clear();
});

describe("the brief", () => {
  it("says who is interviewing and what for, and that nothing is recorded yet", async () => {
    renderPrepare();

    expect(
      await screen.findByText(/senior backend engineer/i),
    ).toBeInTheDocument();
    expect(screen.getByText(/technical deep-dive/i)).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: /who is interviewing you/i }),
    ).toBeInTheDocument();
    expect(screen.getByText(/ravi/i)).toBeInTheDocument();
    expect(
      screen.getAllByText(/nothing is recorded until/i).length,
    ).toBeGreaterThan(0);
  });
});

describe("the gate", () => {
  it("start is disabled until the microphone passes and consent is given, in that order", async () => {
    const user = userEvent.setup();
    renderPrepare();

    const start = await screen.findByRole("button", {
      name: /start interview/i,
    });
    expect(start).toBeDisabled();
    expect(screen.getByText(/run the microphone check/i)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /test microphone/i }));
    await waitFor(() =>
      expect(screen.getByText(/agree to recording above/i)).toBeInTheDocument(),
    );
    expect(start).toBeDisabled();

    await user.click(
      screen.getByRole("checkbox", { name: /record and transcribe/i }),
    );
    expect(start).toBeEnabled();
    expect(screen.getByText(/all required checks passed/i)).toBeInTheDocument();
  });

  it("a failed microphone blocks by name and offers recovery", async () => {
    const user = userEvent.setup();
    renderPrepare({ micResult: "fail" });

    await user.click(
      await screen.findByRole("button", { name: /test microphone/i }),
    );

    await waitFor(() =>
      expect(
        screen.getByText(/until the microphone check passes/i),
      ).toBeInTheDocument(),
    );
    expect(
      screen.getByRole("button", { name: /start interview/i }),
    ).toBeDisabled();
  });

  it("take me to what is missing moves focus to the one problem", async () => {
    const user = userEvent.setup();
    renderPrepare();

    await user.click(
      await screen.findByRole("button", {
        name: /take me to what is missing/i,
      }),
    );
    expect(
      screen.getByRole("button", { name: /test microphone/i }),
    ).toHaveFocus();

    await user.click(screen.getByRole("button", { name: /test microphone/i }));
    await waitFor(() =>
      expect(screen.getByText(/agree to recording above/i)).toBeInTheDocument(),
    );
    await user.click(
      screen.getByRole("button", { name: /take me to what is missing/i }),
    );
    expect(
      screen.getByRole("checkbox", { name: /record and transcribe/i }),
    ).toHaveFocus();
  });

  it("an unsupported browser blocks everything and hides the fix-me hand", async () => {
    renderPrepare({ browser: () => "fail" });

    expect(await screen.findByText(/supported browser/i)).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /start interview/i }),
    ).toBeDisabled();
    expect(
      screen.queryByRole("button", { name: /take me to what is missing/i }),
    ).not.toBeInTheDocument();
  });

  it("the recommended checks never gate", async () => {
    const user = userEvent.setup();
    renderPrepare({ net: () => Promise.resolve("fail") });

    await user.click(
      await screen.findByRole("button", { name: /test microphone/i }),
    );
    await user.click(
      await screen.findByRole("checkbox", { name: /record and transcribe/i }),
    );

    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: /start interview/i }),
      ).toBeEnabled(),
    );
  });
});

describe("consent", () => {
  it("model-improvement consent is separate, off by default, and never bundled", async () => {
    const user = userEvent.setup();
    renderPrepare();

    const optional = await screen.findByRole("checkbox", {
      name: /improve its interviewing/i,
    });
    expect(optional).not.toBeChecked();
    expect(screen.getByText(/optional/i)).toBeInTheDocument();

    // Giving the required consent does not drag the optional one along.
    await user.click(
      screen.getByRole("checkbox", { name: /record and transcribe/i }),
    );
    expect(optional).not.toBeChecked();
    // And the optional one alone opens nothing.
    await user.click(
      screen.getByRole("checkbox", { name: /record and transcribe/i }),
    );
    await user.click(optional);
    expect(
      screen.getByRole("button", { name: /start interview/i }),
    ).toBeDisabled();
  });
});

describe("accessibility defaults", () => {
  it("arrives pre-set from the profile: captions and extra thinking time", async () => {
    renderPrepare();

    expect(
      await screen.findByRole("checkbox", { name: /live captions/i }),
    ).toBeChecked();
    expect(
      screen.getByRole("checkbox", { name: /extra thinking time/i }),
    ).toBeChecked();
    expect(
      screen.getByRole("checkbox", { name: /push to talk/i }),
    ).not.toBeChecked();
  });
});

describe("session states", () => {
  it("a composing session says so and offers no start", async () => {
    vi.mocked(api.getInterview).mockResolvedValue({
      ...session,
      state: "composing",
    });
    vi.mocked(api.getProfile).mockResolvedValue(profile);
    vi.mocked(api.fetchCatalogue).mockResolvedValue(catalogue);
    render(<PrepareScreen sessionId={session.id} runners={runners()} />, {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryProvider>{children}</QueryProvider>
      ),
    });

    expect(
      await screen.findByText(/still being composed/i),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /start interview/i }),
    ).not.toBeInTheDocument();
  });

  it("a failed composition is an honest error, not a dead page", async () => {
    vi.mocked(api.getInterview).mockResolvedValue({
      ...session,
      state: "composition_failed",
      failure_code: "FAILURE_CODE_ARTIFACT_NOT_FOUND",
    });
    vi.mocked(api.getProfile).mockResolvedValue(profile);
    vi.mocked(api.fetchCatalogue).mockResolvedValue(catalogue);
    render(<PrepareScreen sessionId={session.id} runners={runners()} />, {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryProvider>{children}</QueryProvider>
      ),
    });

    expect(await screen.findByRole("alert")).toHaveTextContent(
      /could not be composed/i,
    );
    expect(
      screen.getByText("FAILURE_CODE_ARTIFACT_NOT_FOUND"),
    ).toBeInTheDocument();
  });
});

describe("starting", () => {
  async function openTheGate(user: ReturnType<typeof userEvent.setup>) {
    await user.click(
      await screen.findByRole("button", { name: /test microphone/i }),
    );
    await waitFor(() =>
      expect(screen.getByText(/agree to recording above/i)).toBeInTheDocument(),
    );
    await user.click(
      screen.getByRole("checkbox", { name: /record and transcribe/i }),
    );
  }

  it("start calls the endpoint, stashes the one-use grant and navigates to the live route", async () => {
    vi.mocked(api.startInterview).mockResolvedValue({
      session: {
        id: session.id,
        mode: "practice",
        state: "connecting",
        config: session.config,
        recording_preference: "audio_and_transcript",
        consent_version: "1.0.0",
        created_at: session.created_at,
      },
      realtime: {
        url: "wss://rtc.test",
        room: session.id,
        token: "tok-live",
        expires_at: new Date(Date.now() + 120_000).toISOString(),
      },
    });
    const user = userEvent.setup();
    renderPrepare();

    await openTheGate(user);
    await user.click(screen.getByRole("button", { name: /start interview/i }));

    await waitFor(() =>
      expect(push).toHaveBeenCalledWith(`/session/${session.id}`),
    );
    expect(api.startInterview).toHaveBeenCalledWith(session.id);
    // The grant is in the hand-off, ready for exactly one join.
    const stashed = sessionStorage.getItem(`prepeet.grant.${session.id}`);
    expect(stashed).toContain("tok-live");
  });

  it("a start refusal shows the server's own words and navigates nowhere", async () => {
    vi.mocked(api.startInterview).mockRejectedValue(
      new ApiError({
        status: 409,
        code: "QUOTA_EXHAUSTED",
        message:
          "This workspace is at capacity right now. The hiring team has been told; nothing you did caused this.",
      }),
    );
    const user = userEvent.setup();
    renderPrepare();

    await openTheGate(user);
    await user.click(screen.getByRole("button", { name: /start interview/i }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/at capacity/i);
    expect(push).not.toHaveBeenCalled();
  });
});
