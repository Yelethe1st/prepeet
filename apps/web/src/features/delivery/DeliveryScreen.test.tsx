import { render, screen, waitFor, within } from "@testing-library/react";
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

function renderDelivery(value: DeliveryView = delivery) {
  vi.mocked(api.getDelivery).mockResolvedValue(value);
  vi.mocked(api.getTranscript).mockResolvedValue(transcript);
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
