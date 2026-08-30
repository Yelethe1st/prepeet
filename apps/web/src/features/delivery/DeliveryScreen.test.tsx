import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { QueryProvider } from "@/lib/api/QueryProvider";
import { ApiError } from "@/lib/api/client";

import { DeliveryScreen } from "./DeliveryScreen";
import { DRILLS, paceSummary } from "./analysis";
import * as api from "./api";
import type { DeliveryView, TranscriptView } from "./api";

vi.mock("./api");

/**
 * ART-05 from the outside: every chart has a text summary and a table,
 * observations jump to their moment by keyboard, the ten dimensions are
 * never summed, and a not-assessable delivery says what it is.
 */

const analysis = {
  assessability: {
    status: "assessable",
    warnings: ["AUDIO_QUALITY_NOT_COMPUTED"],
    note: "",
  },
  metrics: {
    words: 24,
    words_per_minute: 120,
    fillers_per_100_words: 8.3,
    long_pause_count: 1,
  },
  turns: [
    {
      sequence: 3,
      words: 24,
      duration_ms: 12000,
      words_per_minute: 120,
      pause_count: 23,
      long_pause_count: 1,
      max_pause_ms: 900,
      filler_count: 2,
      fillers_per_100_words: 8.3,
      restart_count: 1,
      repeated_phrase_count: 1,
      transcript_confidence: 0.95,
      status: "assessable",
      warnings: ["AUDIO_QUALITY_NOT_COMPUTED"],
    },
    {
      sequence: 5,
      words: 4,
      duration_ms: 2000,
      words_per_minute: 120,
      pause_count: 3,
      long_pause_count: 0,
      max_pause_ms: 100,
      filler_count: 0,
      fillers_per_100_words: 0,
      restart_count: 0,
      repeated_phrase_count: 0,
      transcript_confidence: 0.95,
      status: "not_assessable",
      warnings: ["INSUFFICIENT_SPEECH", "AUDIO_QUALITY_NOT_COMPUTED"],
    },
  ],
  profile: {
    profile_version: "articulation-profile-v1",
    dimensions: {
      pace: {
        level: "solid",
        evidence_sequences: [3],
        reason: "120.0 words per minute over assessable turns",
      },
      fluency: {
        level: "developing",
        evidence_sequences: [3],
        reason: "8.3 fillers per hundred words",
      },
      vocal_delivery: {
        level: "not_assessable",
        evidence_sequences: [],
        reason: "audio not decoded at this floor",
      },
    },
  },
  coaching: {
    coaching_version: "articulation-coaching-v1",
    priorities: [
      {
        dimension: "fluency",
        level: "developing",
        listener_impact:
          "Fillers and restarts pull the listener's attention from the content.",
        action:
          "Pause silently where you would say a filler; the pause reads as thought.",
        evidence_sequences: [3],
        drill: "deliberate_pause",
      },
    ],
    suggested_shape: [
      {
        slot: "headline",
        kind: "quote",
        text: "I led the migration for payments.",
        sequence: 3,
      },
      {
        slot: "result",
        kind: "placeholder",
        text: "[What changed as a result? Give the number or the outcome.]",
        sequence: null,
      },
    ],
  },
};

const delivery: DeliveryView = {
  session_id: "00000000-0000-7000-8000-0000000000e1",
  status: "assessable",
  warnings: ["AUDIO_QUALITY_NOT_COMPUTED"],
  note: "",
  calculation_version: "articulation-features-v1",
  policy_version: "articulation-practice-v1",
  analysis,
  created_at: "2026-08-27T16:00:00Z",
};

const transcript: TranscriptView = {
  segments: [
    {
      epoch: 1,
      sequence: 2,
      type: "transcript.segment.final",
      speaker: "interviewer",
      text: "Tell me about it.",
      start_ms: 0,
      end_ms: 1000,
      confidence: 0.99,
      superseded: false,
    },
    {
      epoch: 1,
      sequence: 3,
      type: "transcript.segment.final",
      speaker: "candidate",
      text: "um I I led the migration for payments.",
      start_ms: 61000,
      end_ms: 73000,
      confidence: 0.95,
      superseded: false,
    },
    {
      epoch: 1,
      sequence: 5,
      type: "transcript.segment.final",
      speaker: "candidate",
      text: "Yes that is right",
      start_ms: 90000,
      end_ms: 92000,
      confidence: 0.95,
      superseded: false,
    },
  ],
  orphan_corrections: [],
};

const absentBaseline = {
  baseline_version: "baseline-1",
  sessions_measured: 2,
  minimum_sessions: 5,
  ready: false,
  ranges: {},
  note: "These ranges are guidance about you, not a target: there is no correct speaking rate.",
};

const plainSession = {
  id: "00000000-0000-7000-8000-0000000000e1",
  mode: "practice" as const,
  state: "review_ready",
  config: { discipline: "d", role: "r", shape: "s", minutes: 40, persona: "p" },
  recording_preference: "transcript_only" as const,
  consent_version: "1.0.0",
  created_at: "2026-08-27T18:00:00Z",
};

function renderDelivery(
  value: DeliveryView = delivery,
  baseline = absentBaseline,
) {
  vi.mocked(api.getDelivery).mockResolvedValue(value);
  vi.mocked(api.getTranscript).mockResolvedValue(transcript);
  vi.mocked(api.getBaseline).mockResolvedValue(baseline);
  vi.mocked(api.getInterview).mockResolvedValue(plainSession);
  return render(
    <DeliveryScreen sessionId="00000000-0000-7000-8000-0000000000e1" />,
    {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryProvider>{children}</QueryProvider>
      ),
    },
  );
}

afterEach(() => {
  vi.mocked(api.getDelivery).mockReset();
  vi.mocked(api.getTranscript).mockReset();
  vi.mocked(api.getBaseline).mockReset();
  vi.mocked(api.getInterview).mockReset();
});

describe("the original versus the redo", () => {
  it("a retake shows the question it answered and the metric deltas, the original untouched", async () => {
    const originalId = "00000000-0000-7000-8000-0000000000e0";
    vi.mocked(api.getInterview).mockResolvedValue({
      ...plainSession,
      redo_of: {
        session_id: originalId,
        sequence: 3,
        question: "Tell me about a migration you led.",
      },
    });
    vi.mocked(api.getBaseline).mockResolvedValue(absentBaseline);
    vi.mocked(api.getTranscript).mockResolvedValue(transcript);
    vi.mocked(api.getDelivery).mockImplementation(async (id: string) =>
      id === originalId
        ? {
            ...delivery,
            session_id: originalId,
            analysis: {
              ...analysis,
              metrics: {
                words: 30,
                words_per_minute: 150,
                fillers_per_100_words: 10.3,
                long_pause_count: 3,
              },
            },
          }
        : delivery,
    );
    render(
      <DeliveryScreen sessionId="00000000-0000-7000-8000-0000000000e1" />,
      {
        wrapper: ({ children }: { children: ReactNode }) => (
          <QueryProvider>{children}</QueryProvider>
        ),
      },
    );

    const comparison = await screen.findByTestId("comparison");
    expect(comparison).toHaveTextContent(/tell me about a migration you led/i);
    await waitFor(() =>
      expect(
        within(comparison).getByTestId("delta-words_per_minute"),
      ).toHaveTextContent(/150.*120.*-30/),
    );
    expect(
      within(comparison).getByTestId("delta-fillers_per_100_words"),
    ).toHaveTextContent(/10.3.*8.3.*-2/);
    expect(
      within(comparison).getByRole("link", { name: /original session/i }),
    ).toHaveAttribute("href", `/session/${originalId}/delivery`);
  });

  it("a session that is not a retake shows no comparison", async () => {
    renderDelivery();
    await screen.findByTestId("dimension-fluency");
    expect(screen.queryByTestId("comparison")).not.toBeInTheDocument();
  });
});

describe("the personal baseline", () => {
  it("says how many sessions remain before a range is drawn", async () => {
    renderDelivery();
    const note = await screen.findByTestId("baseline-note");
    expect(note).toHaveTextContent(/no correct speaking rate/i);
    expect(note).toHaveTextContent(/3 more to go/i);
    expect(screen.queryByText(/your usual range/i)).not.toBeInTheDocument();
  });

  it("shows the ranges as guidance once ready, never as a target", async () => {
    renderDelivery(delivery, {
      ...absentBaseline,
      sessions_measured: 6,
      ready: true,
      ranges: { words_per_minute: { low: 130, high: 170 } },
    });
    expect(
      await screen.findByText(/your usual range 130 to 170/i),
    ).toBeInTheDocument();
    expect(screen.getByTestId("baseline-note")).toHaveTextContent(
      /not a target/i,
    );
    expect(document.body.textContent).not.toMatch(
      /target rate|ideal rate|correct rate is/i,
    );
  });
});

describe("charts", () => {
  it("every chart has a text summary and a table alternative", async () => {
    renderDelivery();
    expect(
      await screen.findByText(
        /you spoke between 120 and 120 words per minute/i,
      ),
    ).toBeInTheDocument();
    const chart = screen.getByRole("img");
    expect(chart).toHaveAttribute("aria-labelledby", "pace-summary");
    expect(
      screen.getByRole("table", { name: /speaking rate, long pauses/i }),
    ).toBeInTheDocument();
  });

  it("the summary is honest about unmeasured answers", () => {
    expect(paceSummary([])).toMatch(/no answer was long enough/i);
  });
});

describe("dimensions", () => {
  it("renders each dimension on its own with its reason, and nothing adds them up", async () => {
    renderDelivery();
    const fluency = await screen.findByTestId("dimension-fluency");
    expect(within(fluency).getByText(/developing/i)).toBeInTheDocument();
    expect(
      within(fluency).getByText(/8.3 fillers per hundred words/i),
    ).toBeInTheDocument();
    const vocal = screen.getByTestId("dimension-vocal_delivery");
    expect(within(vocal).getByText(/not assessable/i)).toBeInTheDocument();
    expect(document.body.textContent).not.toMatch(
      /\b\d+\s*\/\s*100\b|delivery score/i,
    );
  });

  it("an observation jumps to its moment in the transcript by keyboard", async () => {
    renderDelivery();
    const user = userEvent.setup();
    const priority = await screen.findByTestId("priority-fluency");
    const jump = within(priority).getByRole("button", { name: /01:01/i });
    jump.focus();
    await user.keyboard("{Enter}");
    await waitFor(() => expect(screen.getByTestId("segment-3")).toHaveFocus());
  });
});

describe("coaching and drills", () => {
  it("shows the priority with impact and action, the shape with marked placeholders, and the selected drill first", async () => {
    renderDelivery();
    const priority = await screen.findByTestId("priority-fluency");
    expect(
      within(priority).getByText(/pull the listener's attention/i),
    ).toBeInTheDocument();
    expect(within(priority).getByText(/pause silently/i)).toBeInTheDocument();
    expect(
      screen
        .getByText(/what changed as a result/i)
        .closest("[data-part='placeholder']"),
    ).not.toBeNull();
    const drills = screen.getAllByTestId(/^drill-/);
    expect(drills).toHaveLength(DRILLS.length);
    const first = drills[0];
    if (!first) {
      throw new Error("no drills rendered");
    }
    expect(first).toHaveAttribute("data-testid", "drill-deliberate_pause");
    expect(within(first).getByText(/selected for you/i)).toBeInTheDocument();
  });
});

describe("honest states", () => {
  it("a not-assessable delivery says it is not a low result", async () => {
    renderDelivery({
      ...delivery,
      status: "not_assessable",
      warnings: ["AUDIO_CLIPPED"],
      note: "Delivery was not assessable for this session. That is a statement about the recording or the transcript, not about you: it is not a low result, and it has not affected any score.",
    });
    const status = await screen.findByRole("status", {
      name: /delivery status/i,
    });
    expect(status).toHaveTextContent(/not a low result/i);
    expect(status).toHaveTextContent(/AUDIO_CLIPPED/);
  });

  it("renders DELIVERY_NOT_READY as the processing state", async () => {
    vi.mocked(api.getDelivery).mockRejectedValue(
      new ApiError({
        status: 409,
        code: "DELIVERY_NOT_READY",
        message: "not yet",
      }),
    );
    vi.mocked(api.getTranscript).mockResolvedValue(transcript);
    render(
      <DeliveryScreen sessionId="00000000-0000-7000-8000-0000000000e1" />,
      {
        wrapper: ({ children }: { children: ReactNode }) => (
          <QueryProvider>{children}</QueryProvider>
        ),
      },
    );
    expect(
      await screen.findByText(/still being measured/i),
    ).toBeInTheDocument();
  });
});

describe("what the screen does with an absence", () => {
  it("withheld coaching is stated, and nothing invented takes its place", async () => {
    renderDelivery({
      ...delivery,
      analysis: {
        ...analysis,
        coaching: { available: false, note: "coaching withheld: test lie" },
      },
    });

    expect(
      await screen.findByText(/withheld for this session/i),
    ).toBeInTheDocument();
    expect(screen.queryByTestId("priority-fluency")).not.toBeInTheDocument();
  });

  it("nothing measurable to change says so rather than inventing a priority", async () => {
    renderDelivery({
      ...delivery,
      analysis: {
        ...analysis,
        coaching: {
          coaching_version: "articulation-coaching-v1",
          priorities: [],
          suggested_shape: [],
        },
      },
    });

    expect(
      await screen.findByText(/nothing measurable needs changing/i),
    ).toBeInTheDocument();
  });

  it("a metric the calculator did not produce reads as not measured", async () => {
    renderDelivery({
      ...delivery,
      analysis: { ...analysis, metrics: {}, turns: [] },
    });

    await screen.findByRole("heading", { name: /what we measured/i });
    expect(screen.getAllByText(/not measured/i).length).toBeGreaterThan(0);
    expect(screen.getByText(/no answer was long enough/i)).toBeInTheDocument();
  });

  it("a retake whose original has no analysis says so rather than showing an empty table", async () => {
    vi.mocked(api.getInterview).mockResolvedValue({
      ...plainSession,
      redo_of: {
        session_id: "00000000-0000-7000-8000-0000000000e0",
        sequence: 3,
        question: "Again please.",
      },
    });
    vi.mocked(api.getBaseline).mockResolvedValue(absentBaseline);
    vi.mocked(api.getTranscript).mockResolvedValue(transcript);
    vi.mocked(api.getDelivery).mockImplementation(async (id: string) => {
      if (id === "00000000-0000-7000-8000-0000000000e0") {
        throw new ApiError({
          status: 409,
          code: "DELIVERY_NOT_READY",
          message: "not yet",
        });
      }
      return delivery;
    });
    render(
      <DeliveryScreen sessionId="00000000-0000-7000-8000-0000000000e1" />,
      {
        wrapper: ({ children }: { children: ReactNode }) => (
          <QueryProvider>{children}</QueryProvider>
        ),
      },
    );

    const comparison = await screen.findByTestId("comparison");
    expect(comparison).toHaveTextContent(/not available to compare yet/i);
  });
});

/**
 * ART-09. The controls are a courtesy: they take an answer from the one
 * person who knows whether the coaching described them, and they ask for
 * nothing and change nothing.
 */
describe("saying whether a priority described you", () => {
  it("records the verdict against the insight, and shows which thumb is pressed", async () => {
    const user = userEvent.setup();
    vi.mocked(api.recordInsightFeedback).mockResolvedValue();
    renderDelivery();

    const no = await screen.findByRole("button", {
      name: /No, this did not describe me: fluency/i,
    });
    expect(no).toHaveAttribute("aria-pressed", "false");

    await user.click(no);

    expect(api.recordInsightFeedback).toHaveBeenCalledWith(
      "00000000-0000-7000-8000-0000000000e1",
      // No dimension. The server reads it from the stored analysis, because
      // a client-supplied one could name a real key and a fabricated
      // dimension and corrupt per-dimension monitoring.
      {
        insight_kind: "priority",
        insight_key: "fluency",
        helpful: false,
      },
    );
    expect(
      screen.getByRole("button", {
        name: /No, this did not describe me: fluency/i,
      }),
    ).toHaveAttribute("aria-pressed", "true");
  });

  it("lets somebody correct a thumb they did not mean", async () => {
    const user = userEvent.setup();
    vi.mocked(api.recordInsightFeedback).mockResolvedValue();
    renderDelivery();

    await user.click(
      await screen.findByRole("button", {
        name: /No, this did not describe me: fluency/i,
      }),
    );
    await user.click(
      screen.getByRole("button", { name: /Yes, this described me: fluency/i }),
    );

    expect(api.recordInsightFeedback).toHaveBeenLastCalledWith(
      "00000000-0000-7000-8000-0000000000e1",
      expect.objectContaining({ insight_key: "fluency", helpful: true }),
    );
    expect(
      screen.getByRole("button", {
        name: /No, this did not describe me: fluency/i,
      }),
    ).toHaveAttribute("aria-pressed", "false");
  });

  /** A verdict given last week is still pressed this week. */
  it("shows a verdict given before this visit", async () => {
    vi.mocked(api.recordInsightFeedback).mockResolvedValue();
    renderDelivery({
      ...delivery,
      insight_feedback: [
        { insight_kind: "priority", insight_key: "fluency", helpful: true },
      ],
    });

    expect(
      await screen.findByRole("button", {
        name: /Yes, this described me: fluency/i,
      }),
    ).toHaveAttribute("aria-pressed", "true");
  });

  /**
   * The ticket's fifth criterion. A prompt, a modal or a box to type in would
   * make this a survey somebody has to get past to read their own coaching.
   */
  it("asks for nothing", async () => {
    renderDelivery();
    await screen.findByRole("button", {
      name: /No, this did not describe me: fluency/i,
    });

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  });

  /**
   * A rejection is a report about the coaching, not a way to edit it: the
   * priority reads exactly the same afterwards.
   */
  it("changes nothing the candidate is shown", async () => {
    const user = userEvent.setup();
    vi.mocked(api.recordInsightFeedback).mockResolvedValue();
    renderDelivery();

    const card = await screen.findByTestId("priority-fluency");
    const before = card.textContent;

    await user.click(
      screen.getByRole("button", {
        name: /No, this did not describe me: fluency/i,
      }),
    );

    expect(screen.getByTestId("priority-fluency").textContent).toBe(before);
  });

  /**
   * A failed verdict is silent. It is feedback about the product given as a
   * courtesy, and interrupting somebody reading their own coaching to report
   * that our telemetry call failed makes the product's problem theirs.
   */
  /**
   * Silent, and not pretending. Interrupting somebody reading their own
   * coaching to report that our telemetry call failed makes our problem
   * theirs; leaving the thumb pressed when nothing was stored is the screen
   * claiming something it has no reason to believe, which the candidate finds
   * out on reload.
   */
  it("says nothing when the verdict cannot be sent, and does not claim it saved", async () => {
    const user = userEvent.setup();
    vi.mocked(api.recordInsightFeedback).mockRejectedValue(
      new Error("offline"),
    );
    renderDelivery();

    await user.click(
      await screen.findByRole("button", {
        name: /No, this did not describe me: fluency/i,
      }),
    );

    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    await waitFor(() =>
      expect(
        screen.getByRole("button", {
          name: /No, this did not describe me: fluency/i,
        }),
      ).toHaveAttribute("aria-pressed", "false"),
    );
  });

  /**
   * The last press wins, not the last response. Two presses issue two upserts
   * and the server takes them in arrival order, so a slow first request
   * landing after the second would leave the screen showing the verdict the
   * candidate had already changed their mind about.
   */
  it("keeps the last thumb pressed when an earlier request fails late", async () => {
    const user = userEvent.setup();
    let failFirst: (reason: Error) => void = () => {};
    vi.mocked(api.recordInsightFeedback)
      .mockImplementationOnce(
        () =>
          new Promise<void>((_resolve, reject) => {
            failFirst = reject;
          }),
      )
      .mockResolvedValueOnce();
    renderDelivery();

    await user.click(
      await screen.findByRole("button", {
        name: /No, this did not describe me: fluency/i,
      }),
    );
    await user.click(
      screen.getByRole("button", { name: /Yes, this described me: fluency/i }),
    );

    // Flushed inside act, or the assertion below runs on the value that was
    // already true and never observes the late failure at all: the first
    // version of this test passed with the sequencing removed.
    await act(async () => {
      failFirst(new Error("offline"));
      await Promise.resolve();
    });

    expect(
      screen.getByRole("button", {
        name: /Yes, this described me: fluency/i,
      }),
    ).toHaveAttribute("aria-pressed", "true");
  });
});

describe("saying whether a drill was worth doing", () => {
  it("takes a verdict on a drill chosen for this session", async () => {
    const user = userEvent.setup();
    vi.mocked(api.recordInsightFeedback).mockResolvedValue();
    renderDelivery();

    await user.click(
      await screen.findByRole("button", {
        name: /No, this did not describe me: deliberate_pause/i,
      }),
    );

    expect(api.recordInsightFeedback).toHaveBeenCalledWith(
      "00000000-0000-7000-8000-0000000000e1",
      expect.objectContaining({
        insight_kind: "drill",
        insight_key: "deliberate_pause",
        helpful: false,
      }),
    );
  });

  /**
   * An unselected drill is a menu item rather than something generated about
   * this candidate, so there is nothing for them to answer about it.
   */
  it("offers nothing on a drill that was not chosen", async () => {
    renderDelivery();
    await screen.findByTestId("drill-deliberate_pause");

    const drills = screen.getAllByTestId(/^drill-/);
    const unselected = drills.filter(
      (drill) => !drill.textContent?.includes("selected for you"),
    );
    expect(unselected.length).toBeGreaterThan(0);
    for (const drill of unselected) {
      expect(
        within(drill).queryByRole("button", { name: /describe me/i }),
      ).not.toBeInTheDocument();
    }
  });
});

/**
 * Regression from the ART review. A delivery that will never arrive was
 * answered as DELIVERY_NOT_READY, so the screen claimed measurement was still
 * running forever and polled every five seconds for as long as the page was
 * open. The reason had been recorded for the candidate and never shown.
 */
describe("a delivery that will never arrive", () => {
  for (const [code, heading] of [
    ["DELIVERY_OMITTED", /was not measured/i],
    ["DELIVERY_FAILED", /could not be measured/i],
  ] as const) {
    it(`says what happened for ${code} rather than waiting`, async () => {
      vi.mocked(api.getDelivery).mockRejectedValue(
        new ApiError({
          status: 409,
          code,
          message: "Delivery measurement was not run for this session.",
        }),
      );
      vi.mocked(api.getTranscript).mockResolvedValue(transcript);
      vi.mocked(api.getBaseline).mockResolvedValue(absentBaseline);
      vi.mocked(api.getInterview).mockResolvedValue(plainSession);

      render(
        <DeliveryScreen sessionId="00000000-0000-7000-8000-0000000000e1" />,
        {
          wrapper: ({ children }: { children: ReactNode }) => (
            <QueryProvider>{children}</QueryProvider>
          ),
        },
      );

      expect(await screen.findByText(heading)).toBeInTheDocument();
      // Not the pending copy: this one never resolves itself.
      expect(
        screen.queryByText(/still being measured/i),
      ).not.toBeInTheDocument();
      // And it says the evaluation is untouched, because a candidate reading
      // this needs to know it is not a result about them.
      expect(
        screen.getByText(/results and coaching are unaffected/i),
      ).toBeInTheDocument();
    });
  }
});
