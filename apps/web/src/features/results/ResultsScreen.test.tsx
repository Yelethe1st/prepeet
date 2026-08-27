import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { QueryProvider } from "@/lib/api/QueryProvider";
import { ApiError } from "@/lib/api/client";

import { ResultsScreen } from "./ResultsScreen";
import * as api from "./api";
import type { EvaluationResult, TranscriptView } from "./api";

vi.mock("./api");

/**
 * PRC-01 from the outside: every score expands to the exact sentences that
 * produced it, insufficient reads as insufficient and never as a low band,
 * evidence timestamps jump focus into the transcript by keyboard, the
 * server's framing copy is shown, and no numeric score exists anywhere
 * (ADR-0015: qualitative only until calibration).
 */

const result: EvaluationResult = {
  session_id: "00000000-0000-7000-8000-0000000000e1",
  rubric: {
    reference: "rubric/practice-default",
    version: "1.1.0",
    digest: "sha256:abc",
  },
  aggregation_version: "aggregate-1",
  extraction_version: "evidence-1",
  model_version: "none",
  policy_version: "none",
  competencies: [
    {
      competency_id: "systems-design",
      status: "assessed",
      confidence: "high",
      band: "strong",
      evidence_count: 4,
      supporting: 4,
      contradictory: 0,
      unverified: 0,
      gaps: 0,
      evidence_ids: ["sp-1", "sp-2", "sp-3", "sp-4"],
      reason_codes: [],
    },
    {
      competency_id: "debugging",
      status: "unassessed",
      confidence: "not_assessable",
      evidence_count: 1,
      supporting: 1,
      contradictory: 0,
      unverified: 0,
      gaps: 0,
      evidence_ids: ["sp-5"],
      reason_codes: ["INSUFFICIENT_EVIDENCE"],
    },
    {
      competency_id: "prioritisation",
      status: "unassessed",
      confidence: "not_assessable",
      evidence_count: 0,
      supporting: 0,
      contradictory: 0,
      unverified: 0,
      gaps: 0,
      evidence_ids: [],
      reason_codes: ["NOT_DISCUSSED"],
    },
  ],
  evidence: [
    {
      id: "sp-1",
      competency_id: "systems-design",
      kind: "supporting",
      quote: "We sharded the checkout by region and latency fell 40 percent.",
      segment_sequence: 3,
      start_ms: 61_000,
      end_ms: 68_000,
    },
    {
      id: "sp-5",
      competency_id: "debugging",
      kind: "claim_unverified",
      quote: "I usually find the root cause quickly.",
      segment_sequence: 5,
      start_ms: 130_000,
      end_ms: 133_000,
    },
  ],
  contradictions: [
    {
      topic: ["payments", "team"],
      side_a: {
        segment_sequence: 3,
        quote: "I led the payments migration team of 5 engineers.",
        start_ms: 61_000,
        end_ms: 64_000,
      },
      side_b: {
        segment_sequence: 5,
        quote: "The payments migration team I led was 12 people.",
        start_ms: 130_000,
        end_ms: 133_000,
      },
    },
  ],
  framing: {
    unverified:
      "Unverified means nobody checked this claim during the session. It does not mean the claim is untrue.",
    contradictions:
      "These statements appear to conflict. Treat this as something to ask about, not as a conclusion about the person.",
    confidence:
      "Confidence describes how much verifiable evidence this session produced for each competency. It is not a prediction of performance in any role.",
  },
  coverage: {
    reached: ["debugging", "systems-design"],
    not_reached: ["prioritisation"],
  },
  omissions: [
    {
      stage: "articulation",
      reason: "BUDGET_EXHAUSTED",
      retryable: false,
      note: "Delivery measurement is not part of this session's evaluation. Your results above are complete and unaffected.",
    },
  ],
  delivery: { status: "pending", warnings: [], note: "" },
  covered_competencies: 2,
  total_competencies: 3,
  result_digest: "sha256:def",
  warnings: [],
  created_at: "2026-08-26T19:15:00Z",
};

const transcript: TranscriptView = {
  segments: [
    {
      epoch: 1,
      sequence: 2,
      type: "transcript.segment.final",
      speaker: "interviewer",
      text: "Tell me about a systems design decision you made.",
      start_ms: 55_000,
      end_ms: 60_000,
      confidence: 0.99,
      superseded: false,
    },
    {
      epoch: 1,
      sequence: 3,
      type: "transcript.segment.final",
      speaker: "candidate",
      text: "We sharded the checkout by region and latency fell 40 percent. I led the payments migration team of 5 engineers.",
      start_ms: 61_000,
      end_ms: 70_000,
      confidence: 0.95,
      superseded: false,
    },
    {
      epoch: 1,
      sequence: 5,
      type: "transcript.segment.final",
      speaker: "candidate",
      text: "I usually find the root cause quickly. The payments migration team I led was 12 people.",
      start_ms: 130_000,
      end_ms: 140_000,
      confidence: 0.92,
      superseded: false,
    },
  ],
  orphan_corrections: [],
};

function renderResults() {
  vi.mocked(api.getResults).mockResolvedValue(result);
  vi.mocked(api.getTranscript).mockResolvedValue(transcript);
  return render(
    <ResultsScreen sessionId="00000000-0000-7000-8000-0000000000e1" />,
    {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryProvider>{children}</QueryProvider>
      ),
    },
  );
}

afterEach(() => {
  vi.mocked(api.getResults).mockReset();
  vi.mocked(api.getTranscript).mockReset();
});

describe("the outcome", () => {
  it("shows bands and confidence with the server's framing, and no numeric score anywhere", async () => {
    renderResults();

    expect(
      await screen.findByRole("heading", { name: /outcome/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/not a prediction of performance/i),
    ).toBeInTheDocument();

    const assessed = screen.getByTestId("competency-systems-design");
    expect(within(assessed).getByText(/strong/i)).toBeInTheDocument();
    expect(within(assessed).getByText(/high confidence/i)).toBeInTheDocument();

    // ADR-0015: nothing numeric-looking, no "out of 100", no percentage
    // score. The deliberate deviation from the prototype's score ring.
    expect(screen.queryByText(/out of 100/i)).not.toBeInTheDocument();
    expect(document.body.textContent).not.toMatch(/\d+\s*\/\s*100/);
  });

  it("renders insufficient as its own outcome, never as a low band", async () => {
    renderResults();

    const thin = await screen.findByTestId("competency-debugging");
    expect(
      within(thin).getByText("Debugging: insufficient evidence"),
    ).toBeInTheDocument();
    expect(within(thin).getByText(/not a verdict on you/i)).toBeInTheDocument();
    expect(within(thin).queryByText(/developing/i)).not.toBeInTheDocument();

    // Never reached is its own reason, distinct from thin.
    const silent = screen.getByTestId("competency-prioritisation");
    expect(within(silent).getByText(/never came up/i)).toBeInTheDocument();
  });
});

describe("the evidence", () => {
  it("expands a competency to the exact sentences that produced it", async () => {
    renderResults();
    const user = userEvent.setup();

    const toggle = await screen.findByRole("button", {
      name: /evidence for systems design/i,
    });
    await user.click(toggle);

    expect(
      screen.getByText(
        "We sharded the checkout by region and latency fell 40 percent.",
      ),
    ).toBeInTheDocument();
  });

  it("jumps focus to the transcript segment from an evidence timestamp, by keyboard", async () => {
    renderResults();
    const user = userEvent.setup();

    const accordion = (
      await screen.findByRole("button", {
        name: /evidence for systems design/i,
      })
    ).closest("div");
    await user.click(
      screen.getByRole("button", { name: /evidence for systems design/i }),
    );
    const jump = within(accordion as HTMLElement).getByRole("button", {
      name: /01:01/i,
    });
    jump.focus();
    await user.keyboard("{Enter}");

    const segment = screen.getByTestId("segment-3");
    await waitFor(() => expect(segment).toHaveFocus());
  });

  it("shows both sides of a contradiction with timestamps and the neutral framing", async () => {
    renderResults();

    expect(
      await screen.findByText(/something to ask about, not as a conclusion/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText("I led the payments migration team of 5 engineers."),
    ).toBeInTheDocument();
    expect(
      screen.getByText("The payments migration team I led was 12 people."),
    ).toBeInTheDocument();
    expect(screen.getAllByText(/02:10/).length).toBeGreaterThan(0);
  });
});

describe("the waiting and failure states", () => {
  it("renders RESULT_NOT_READY as the processing state, not an error", async () => {
    vi.mocked(api.getResults).mockRejectedValue(
      new ApiError({
        status: 409,
        code: "RESULT_NOT_READY",
        message: "Evaluation has not finished for this session.",
      }),
    );
    vi.mocked(api.getTranscript).mockResolvedValue(transcript);
    render(<ResultsScreen sessionId="00000000-0000-7000-8000-0000000000e1" />, {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryProvider>{children}</QueryProvider>
      ),
    });

    expect(
      await screen.findByText(/still being evaluated/i),
    ).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("renders a real failure as the error state with its reference", async () => {
    vi.mocked(api.getResults).mockRejectedValue(
      new ApiError({
        status: 500,
        code: "INTERNAL",
        message: "boom",
        requestId: "req_777",
      }),
    );
    vi.mocked(api.getTranscript).mockResolvedValue(transcript);
    render(<ResultsScreen sessionId="00000000-0000-7000-8000-0000000000e1" />, {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryProvider>{children}</QueryProvider>
      ),
    });

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/req_777/);
  });
});

describe("what is not here", () => {
  it("shows the server's words for an omission, and the results still stand", async () => {
    renderResults();

    const omissions = await screen.findByTestId("omissions");
    expect(omissions).toHaveTextContent(/delivery measurement is not part/i);
    expect(omissions).toHaveTextContent(/complete and unaffected/i);
    // The competency results above are unaffected by the absence.
    expect(screen.getByTestId("competency-systems-design")).toHaveTextContent(
      /strong/i,
    );
  });

  it("says nothing when nothing is missing", async () => {
    vi.mocked(api.getResults).mockResolvedValue({ ...result, omissions: [] });
    vi.mocked(api.getTranscript).mockResolvedValue(transcript);
    render(<ResultsScreen sessionId="00000000-0000-7000-8000-0000000000e1" />, {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryProvider>{children}</QueryProvider>
      ),
    });

    await screen.findByRole("heading", { name: /outcome/i });
    expect(screen.queryByTestId("omissions")).not.toBeInTheDocument();
  });
});
