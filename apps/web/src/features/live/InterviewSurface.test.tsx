import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Speaker } from "@/lib/rtc/speakers";

import { InterviewSurface } from "./InterviewSurface";
import * as liveApi from "./api";

vi.mock("./api");

/**
 * RTC-06's boxes from the surface's side. Every state is words, not colour:
 * who speaks, what the microphone does, how the clock stands. Push-to-talk
 * works by pointer and by keyboard and announces itself. And nothing on
 * this screen scores anything.
 */

const session = {
  id: "00000000-0000-7000-8000-0000000000e1",
  mode: "practice",
  state: "in_progress",
  config: {
    discipline: "software-engineering",
    role: "rl_swe",
    shape: "shape_technical",
    minutes: 40,
    persona: "per_ravi",
  },
  recording_preference: "audio_and_transcript",
  consent_version: "1.0.0",
  created_at: "2026-09-05T10:00:00Z",
} as unknown as liveApi.InterviewSession;

const context = {
  personas: [
    {
      id: "per_ravi",
      name: "Ravi",
      style: "Direct and technical",
      voice: "",
      description: "",
      best_for: "",
      shapes: [],
    },
  ],
  roles: [
    {
      id: "rl_swe",
      discipline: "software-engineering",
      title: "Senior Backend Engineer",
      organisation: "",
      competencies: [],
      shapes: [],
    },
  ],
  shapes: [
    {
      id: "shape_technical",
      name: "Systems design deep-dive",
      description: "",
      minutes: [40],
    },
  ],
};

function finalSegment(sequence: number, speaker: string, text: string) {
  return {
    event_id: `evt-${sequence}`,
    connection_epoch: 1,
    sequence,
    type: "transcript.segment.final",
    payload: { speaker, text },
    occurred_at: "2026-09-05T10:00:00Z",
  };
}

function renderSurface(
  overrides: {
    mode?: "practice" | "screening";
    paused?: boolean;
    onEndConfirmed?: () => void;
  } = {},
) {
  const micCalls: boolean[] = [];
  let emitSpeaker: (speaker: Speaker) => void = () => undefined;
  const rendered = render(
    <InterviewSurface
      sessionId="ses-1"
      session={
        {
          ...(session as object),
          mode: overrides.mode ?? "practice",
        } as liveApi.InterviewSession
      }
      mic={{
        setMicrophoneEnabled: async (enabled) => {
          micCalls.push(enabled);
        },
      }}
      subscribeSpeakers={(onChange) => {
        emitSpeaker = onChange;
        return () => undefined;
      }}
      paused={overrides.paused ?? false}
      onEndConfirmed={overrides.onEndConfirmed ?? (() => undefined)}
    />,
  );
  return {
    rendered,
    micCalls,
    speak: (speaker: Speaker) => act(() => emitSpeaker(speaker)),
  };
}

afterEach(() => {
  vi.mocked(liveApi.fetchLiveContext).mockReset();
  vi.mocked(liveApi.replayEvents).mockReset();
  vi.useRealTimers();
});

describe("the framing", () => {
  it("names the shape, the role, the persona and the mode, resolved from the catalogue", async () => {
    vi.mocked(liveApi.fetchLiveContext).mockResolvedValue(context);
    vi.mocked(liveApi.replayEvents).mockResolvedValue([]);

    renderSurface();

    expect(
      await screen.findByText(/systems design deep-dive/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/senior backend engineer · 40 minutes planned/i),
    ).toBeInTheDocument();
    expect(screen.getByText("Ravi")).toBeInTheDocument();
    expect(
      screen.getByText(/practice interview · not shared with employers/i),
    ).toBeInTheDocument();
  });

  it("scores nothing, and the help panel says so in words", async () => {
    vi.mocked(liveApi.fetchLiveContext).mockResolvedValue(context);
    vi.mocked(liveApi.replayEvents).mockResolvedValue([]);
    const user = userEvent.setup();

    renderSurface();
    await user.click(
      screen.getByRole("button", { name: /help and shortcuts/i }),
    );

    expect(
      screen.getByText(/there is no score on this screen/i),
    ).toBeInTheDocument();
    // The third box, asserted flatly: nothing on the surface mentions
    // articulation, filler words or a correction of the candidate.
    expect(screen.queryByText(/articulation|filler/i)).not.toBeInTheDocument();
  });
});

describe("every state is words", () => {
  it("speaking states are text on both pills and the activity note", async () => {
    vi.mocked(liveApi.fetchLiveContext).mockResolvedValue(context);
    vi.mocked(liveApi.replayEvents).mockResolvedValue([]);

    const { speak } = renderSurface();
    await screen.findByText("Ravi");

    expect(screen.getByText(/ravi is waiting/i)).toBeInTheDocument();
    expect(screen.getByText(/you are not speaking/i)).toBeInTheDocument();

    speak("ai");
    expect(screen.getByText("Ravi is speaking")).toBeInTheDocument();
    expect(
      screen.getByText(/voice activity: ravi is speaking/i),
    ).toBeInTheDocument();

    speak("user");
    expect(screen.getByText(/ravi is listening/i)).toBeInTheDocument();
    expect(screen.getByText("You are speaking")).toBeInTheDocument();
  });

  it("speaking while not transmitting is a warning with the way out", async () => {
    vi.mocked(liveApi.fetchLiveContext).mockResolvedValue(context);
    vi.mocked(liveApi.replayEvents).mockResolvedValue([]);
    const user = userEvent.setup();

    const { speak, micCalls } = renderSurface();
    await screen.findByText("Ravi");

    await user.click(
      screen.getByRole("button", { name: /mute your microphone/i }),
    );
    speak("user");

    const warning = await screen.findByRole("alert");
    expect(warning).toHaveTextContent(/ravi cannot hear you/i);

    await user.click(within(warning).getByRole("button", { name: /unmute/i }));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    // The room was told each time: off at mute, on again at unmute.
    expect(micCalls.at(-2)).toBe(false);
    expect(micCalls.at(-1)).toBe(true);
  });

  it("the clock is text and stops while the connection is being recovered", async () => {
    vi.useFakeTimers();
    vi.mocked(liveApi.fetchLiveContext).mockResolvedValue(context);
    vi.mocked(liveApi.replayEvents).mockResolvedValue([]);

    renderSurface({ paused: true });

    act(() => {
      vi.advanceTimersByTime(5_000);
    });
    expect(screen.getByRole("timer")).toHaveTextContent("00:00 / 40:00");
  });
});

describe("push-to-talk, by pointer and by keyboard", () => {
  it("switching announces the change and closes the microphone until held", async () => {
    vi.mocked(liveApi.fetchLiveContext).mockResolvedValue(context);
    vi.mocked(liveApi.replayEvents).mockResolvedValue([]);
    const user = userEvent.setup();

    const { micCalls } = renderSurface();
    await screen.findByText("Ravi");

    await user.click(
      screen.getByRole("button", { name: /switch to push-to-talk/i }),
    );

    expect(screen.getByTestId("announcer")).toHaveTextContent(
      /push-to-talk enabled/i,
    );
    expect(
      screen.getByText(/hold space, or press and hold the microphone/i),
    ).toBeInTheDocument();
    expect(micCalls.at(-1)).toBe(false);
  });

  it("holding the microphone button by pointer opens the microphone; releasing closes it", async () => {
    vi.mocked(liveApi.fetchLiveContext).mockResolvedValue(context);
    vi.mocked(liveApi.replayEvents).mockResolvedValue([]);
    const user = userEvent.setup();

    const { micCalls } = renderSurface();
    await screen.findByText("Ravi");
    await user.click(
      screen.getByRole("button", { name: /switch to push-to-talk/i }),
    );

    const talk = screen.getByRole("button", {
      name: /press and hold to talk/i,
    });
    await user.pointer([{ keys: "[MouseLeft>]", target: talk }]);
    expect(micCalls.at(-1)).toBe(true);
    expect(
      screen.getByRole("button", { name: /talking\. release to stop/i }),
    ).toBeInTheDocument();

    await user.pointer(["[/MouseLeft]"]);
    expect(micCalls.at(-1)).toBe(false);
  });

  it("holding Space opens the microphone; releasing closes it; M and C keep their jobs", async () => {
    vi.mocked(liveApi.fetchLiveContext).mockResolvedValue(context);
    vi.mocked(liveApi.replayEvents).mockResolvedValue([]);
    const user = userEvent.setup();

    const { micCalls } = renderSurface();
    await screen.findByText("Ravi");
    await user.click(
      screen.getByRole("button", { name: /switch to push-to-talk/i }),
    );

    await user.keyboard("{ >}");
    expect(micCalls.at(-1)).toBe(true);
    await user.keyboard("{/ }");
    expect(micCalls.at(-1)).toBe(false);

    // C hides the caption box; M is push-to-talk-proof (no mute toggle).
    expect(screen.getByTestId("caption")).toBeInTheDocument();
    await user.keyboard("c");
    expect(screen.queryByTestId("caption")).not.toBeInTheDocument();
  });
});

describe("captions from the timeline", () => {
  it("shows the latest line and the whole history, speaker named", async () => {
    vi.mocked(liveApi.fetchLiveContext).mockResolvedValue(context);
    vi.mocked(liveApi.replayEvents).mockResolvedValue([
      finalSegment(2, "interviewer", "Tell me about a system you built."),
      finalSegment(3, "candidate", "I led the booking rewrite."),
    ]);
    const user = userEvent.setup();

    renderSurface();

    await waitFor(() =>
      expect(screen.getByTestId("caption")).toHaveTextContent(
        /I led the booking rewrite/,
      ),
    );

    await user.click(
      screen.getByRole("button", { name: /caption history \(2\)/i }),
    );
    const drawer = screen.getByRole("dialog", { name: /caption history/i });
    expect(drawer).toHaveTextContent(/tell me about a system you built/i);
    expect(drawer).toHaveTextContent(/2 lines so far/i);
  });
});

describe("ending is an explicit, mode-aware decision", () => {
  it("screening names the finality; confirming is what actually ends", async () => {
    vi.mocked(liveApi.fetchLiveContext).mockResolvedValue(context);
    vi.mocked(liveApi.replayEvents).mockResolvedValue([]);
    const ended = vi.fn();
    const user = userEvent.setup();

    renderSurface({ mode: "screening", onEndConfirmed: ended });
    await screen.findByText("Ravi");

    await user.click(screen.getByRole("button", { name: /end interview/i }));
    const dialog = screen.getByRole("alertdialog", {
      name: /end this interview early/i,
    });
    expect(dialog).toHaveTextContent(/cannot restart or retake/i);

    await user.click(
      within(dialog).getByRole("button", { name: /cancel, keep going/i }),
    );
    expect(ended).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: /end interview/i }));
    await user.click(
      within(
        screen.getByRole("alertdialog", { name: /end this interview early/i }),
      ).getByRole("button", { name: /^end interview$/i }),
    );
    expect(ended).toHaveBeenCalledTimes(1);
  });
});
