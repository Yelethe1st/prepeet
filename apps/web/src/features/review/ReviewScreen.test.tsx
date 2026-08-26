import { render, screen, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { QueryProvider } from "@/lib/api/QueryProvider";
import { ApiError } from "@/lib/api/client";

import { ReviewScreen } from "./ReviewScreen";
import * as api from "./api";
import type { ReviewView } from "./api";

vi.mock("./api");

/**
 * PRC-02 from the outside: every statement shows beside the exact words it
 * is about, a placeholder renders as a visibly distinct question and never
 * as a fact, and a coaching failure says the evaluation is intact.
 */

const review: ReviewView = {
  session_id: "00000000-0000-7000-8000-0000000000e1",
  coaching_version: "coaching-1",
  coaching_available: true,
  answers: [
    {
      sequence: 3,
      strengths: [
        {
          statement:
            "You backed this with a concrete, measured outcome. Keep leading with it.",
          quote: "Latency dropped 40 percent.",
        },
      ],
      gaps: [],
      rewrite: [],
    },
    {
      sequence: 5,
      strengths: [],
      gaps: [
        {
          statement:
            "This is a claim about yourself with nothing a listener could check. Ground it in one specific moment.",
          quote: "I am usually good at systems design tradeoffs.",
        },
      ],
      rewrite: [
        { kind: "quote", text: "I am usually good at systems design tradeoffs." },
        {
          kind: "placeholder",
          text: "[Which project or moment shows this? Name it, and what happened.]",
        },
      ],
    },
  ],
};

function renderReview(value: ReviewView = review) {
  vi.mocked(api.getReview).mockResolvedValue(value);
  return render(
    <ReviewScreen sessionId="00000000-0000-7000-8000-0000000000e1" />,
    {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryProvider>{children}</QueryProvider>
      ),
    },
  );
}

afterEach(() => {
  vi.mocked(api.getReview).mockReset();
});

describe("the coaching", () => {
  it("shows every statement beside the exact words it is about", async () => {
    renderReview();

    const strong = await screen.findByTestId("answer-3");
    expect(
      within(strong).getByText(/keep leading with it/i),
    ).toBeInTheDocument();
    expect(
      within(strong).getByText("Latency dropped 40 percent."),
    ).toBeInTheDocument();
  });

  it("renders a placeholder as a distinct question, never as a fact", async () => {
    renderReview();

    const claim = await screen.findByTestId("answer-5");
    const placeholder = within(claim).getByText(
      /which project or moment shows this/i,
    );
    // The placeholder is marked for what it is, distinguishable by more
    // than styling.
    expect(placeholder.closest("[data-part='placeholder']")).not.toBeNull();
    expect(
      within(claim).getByText(
        "I am usually good at systems design tradeoffs.",
        { selector: "[data-part='quote']" },
      ),
    ).toBeInTheDocument();
  });

  it("a strong answer reads as strength, with no rewrite offered", async () => {
    renderReview();

    const strong = await screen.findByTestId("answer-3");
    expect(
      within(strong).queryByText(/suggested rewrite/i),
    ).not.toBeInTheDocument();
  });
});

describe("the honest states", () => {
  it("a coaching failure says the evaluation is intact and links to it", async () => {
    renderReview({
      session_id: "00000000-0000-7000-8000-0000000000e1",
      coaching_version: "coaching-1",
      coaching_available: false,
      note: "Coaching could not be derived for this session. Your evaluation is complete and unaffected.",
      answers: [],
    });

    expect(
      await screen.findByText(/complete and unaffected/i),
    ).toBeInTheDocument();
    const link = screen.getByRole("link", { name: /outcome and evidence/i });
    expect(link).toHaveAttribute(
      "href",
      "/session/00000000-0000-7000-8000-0000000000e1/results",
    );
  });

  it("renders RESULT_NOT_READY as the processing state", async () => {
    vi.mocked(api.getReview).mockRejectedValue(
      new ApiError({
        status: 409,
        code: "RESULT_NOT_READY",
        message: "Evaluation has not finished.",
      }),
    );
    render(
      <ReviewScreen sessionId="00000000-0000-7000-8000-0000000000e1" />,
      {
        wrapper: ({ children }: { children: ReactNode }) => (
          <QueryProvider>{children}</QueryProvider>
        ),
      },
    );

    expect(
      await screen.findByText(/still being evaluated/i),
    ).toBeInTheDocument();
  });

  it("renders a real failure as the error state with its reference", async () => {
    vi.mocked(api.getReview).mockRejectedValue(
      new ApiError({
        status: 500,
        code: "INTERNAL",
        message: "boom",
        requestId: "req_888",
      }),
    );
    render(
      <ReviewScreen sessionId="00000000-0000-7000-8000-0000000000e1" />,
      {
        wrapper: ({ children }: { children: ReactNode }) => (
          <QueryProvider>{children}</QueryProvider>
        ),
      },
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(/req_888/);
  });
});
