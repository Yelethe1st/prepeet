import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { QueryProvider } from "@/lib/api/QueryProvider";
import { ApiError } from "@/lib/api/client";

import { ReviewScreen } from "./ReviewScreen";
import * as api from "./api";

vi.mock("./api");

/**
 * REV-02's boxes from the screen's side: conclusions sit above their own
 * evidence, uncertainty travels with every band, the decision statement
 * opens the page, and nothing anywhere reads as a recommendation.
 */

const review: api.ScreeningReview = {
  session_id: "00000000-0000-7000-8000-00000000a001",
  pinned: {
    bundle_digest: "sha256:bundle",
    rubric: { reference: "rubric/icu", version: "2.0.0", digest: "sha256:rub" },
    aggregation_version: "aggregate-1",
    extraction_version: "evidence-1",
    model_version: "claude-sonnet-5",
    policy_version: "runtime-policy-v1",
  },
  competencies: [
    {
      competency_id: "sd",
      status: "assessed",
      band: "solid",
      confidence: "medium",
      evidence_count: 2,
      supporting: 2,
      contradictory: 1,
      unverified: 0,
      gaps: 0,
      evidence_ids: ["sp-1"],
      reason_codes: [],
    },
    {
      competency_id: "comm",
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
      competency_id: "sd",
      kind: "supporting",
      quote: "sharded by clinic",
      segment_sequence: 4,
      start_ms: 65000,
      end_ms: 70000,
    },
  ],
  coverage: { reached: ["sd"], not_reached: ["comm"], covered: 1, total: 2 },
  contradictions: [
    {
      topic: ["queue"],
      side_a: {
        segment_sequence: 4,
        quote: "we shard by clinic",
        start_ms: 65000,
        end_ms: 70000,
      },
      side_b: {
        segment_sequence: 9,
        quote: "one global queue",
        start_ms: 120000,
        end_ms: 125000,
      },
    },
  ],
  requirements: {
    map_version: "requirement-map-1",
    requirements: [
      {
        requirement_id: "00000000-0000-7000-8000-00000000f001",
        text: "Communication with stakeholders",
        status: "not_discussed",
        competencies: ["comm"],
        evidence_ids: [],
        follow_up:
          'The interview never reached "Communication with stakeholders". Ask about it in a follow-up conversation.',
      },
    ],
  },
};

function renderReview(data: api.ScreeningReview = review) {
  vi.mocked(api.fetchReview).mockResolvedValue(data);
  return render(<ReviewScreen campaignId="cmp-1" sessionId="ses-1" />, {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryProvider>{children}</QueryProvider>
    ),
  });
}

afterEach(() => {
  vi.mocked(api.fetchReview).mockReset();
});

describe("the review screen", () => {
  it("opens by saying whose decision this is, and recommends nothing", async () => {
    renderReview();

    expect(
      await screen.findByText(/decision on this candidate belongs to you/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/prepeet does not recommend an outcome/i),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(/we recommend|suggested band/i),
    ).not.toBeInTheDocument();
  });

  it("keeps uncertainty in the same card as every band", async () => {
    renderReview();

    const card = await screen.findByRole("article", {
      name: /competency sd/i,
    });
    // Band, confidence and the sufficiency counts, together: never a
    // footnote away from the number they qualify.
    expect(card).toHaveTextContent("solid");
    expect(card).toHaveTextContent(/confidence medium/i);
    expect(card).toHaveTextContent(/2 supporting, 1 contradictory/i);

    // Unassessed is its own outcome, said in words, never a low score.
    const unassessed = screen.getByRole("article", {
      name: /competency comm/i,
    });
    expect(unassessed).toHaveTextContent(/unassessed/i);
    expect(unassessed).toHaveTextContent(/not a low score/i);
  });

  it("puts the evidence itself under the conclusion it supports", async () => {
    renderReview();

    const card = await screen.findByRole("article", {
      name: /competency sd/i,
    });
    expect(card).toHaveTextContent(/sharded by clinic/i);
    expect(card).toHaveTextContent("01:05");
  });

  it("states contradictions neutrally, both sides quoted", async () => {
    renderReview();

    expect(await screen.findByText(/“we shard by clinic”/)).toBeInTheDocument();
    expect(screen.getByText(/“one global queue”/)).toBeInTheDocument();
    expect(
      screen.getByText(/whether they conflict is yours to weigh/i),
    ).toBeInTheDocument();
  });

  it("reports each requirement on its own with the follow-up, never a percentage", async () => {
    renderReview();

    const section = await screen.findByRole("region", {
      name: /job requirements/i,
    });
    expect(section).toHaveTextContent(
      /each on its own, never a match percentage/i,
    );
    expect(section).toHaveTextContent(/communication with stakeholders/i);
    expect(section).toHaveTextContent(/^((?!%).)*$/s);
    expect(
      within(section).getByText(/ask about it in a follow-up conversation/i),
    ).toBeInTheDocument();
  });

  it("a review with nothing contested and no requirements says so plainly", async () => {
    renderReview({
      ...review,
      coverage: {
        reached: ["sd", "comm"],
        not_reached: [],
        covered: 2,
        total: 2,
      },
      contradictions: [],
      requirements: { map_version: "requirement-map-1", requirements: [] },
      competencies: [
        {
          ...review.competencies[0]!,
          // One id the evidence list does not carry: the card renders the
          // spans it can resolve and invents nothing for the ghost.
          evidence_ids: ["sp-1", "sp-ghost"],
        },
      ],
    });

    expect(
      await screen.findByText(/every competency was reached/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/this campaign froze no requirements/i),
    ).toBeInTheDocument();
    expect(screen.queryByText(/contradictions/i)).not.toBeInTheDocument();
  });

  it("a failed load names what failed with its reference", async () => {
    vi.mocked(api.fetchReview).mockRejectedValue(
      new ApiError({ status: 400, message: "boom", requestId: "req-9" }),
    );
    render(<ReviewScreen campaignId="cmp-1" sessionId="ses-1" />, {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryProvider>{children}</QueryProvider>
      ),
    });

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/review could not be loaded/i);
    expect(alert).toHaveTextContent("req-9");

    // Retry asks again and the document replaces the alert.
    vi.mocked(api.fetchReview).mockResolvedValue(review);
    await userEvent
      .setup()
      .click(within(alert).getByRole("button", { name: /retry/i }));
    expect(
      await screen.findByText(/decision on this candidate belongs to you/i),
    ).toBeInTheDocument();
  });

  it("an unpublished evaluation is a wait, not an error", async () => {
    vi.mocked(api.fetchReview).mockRejectedValue(
      new ApiError({ status: 409, code: "REVIEW_NOT_READY", message: "m" }),
    );
    render(<ReviewScreen campaignId="cmp-1" sessionId="ses-1" />, {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryProvider>{children}</QueryProvider>
      ),
    });

    expect(
      await screen.findByText(/has not published its evaluation yet/i),
    ).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
