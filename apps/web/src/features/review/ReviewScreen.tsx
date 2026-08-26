"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";

import { ApiError } from "@/lib/api/client";
import {
  DelayedState,
  ErrorState,
  LoadingSurface,
  SkeletonBlock,
  SkeletonText,
} from "@/shared/states";

import { getReview, type AnswerCoaching, type CoachingPoint } from "./api";

/**
 * The coaching review - PRC-02, from the prototype's
 * candidate-session-review screen at the deterministic floor.
 *
 * Everything shown is derived server-side from what the candidate actually
 * said and gated for fact preservation before it is served: every
 * statement sits beside its exact quote, and a suggested rewrite is the
 * candidate's own sentences plus bracketed questions where information is
 * missing. A placeholder renders as the question it is - marked in the
 * DOM, not just styled - so it can never read as a fact. When coaching
 * cannot be derived, the screen says the one thing that matters: the
 * evaluation is complete and unaffected.
 */
export function ReviewScreen({ sessionId }: { sessionId: string }) {
  const review = useQuery({
    queryKey: ["review", sessionId],
    queryFn: () => getReview(sessionId),
    retry: false,
    refetchInterval: (query) =>
      query.state.error instanceof ApiError &&
      query.state.error.code === "RESULT_NOT_READY"
        ? 5000
        : false,
  });

  if (review.isPending) {
    return (
      <LoadingSurface label="Loading the coaching review">
        <SkeletonBlock className="h-[120px]" />
        <SkeletonText />
        <SkeletonBlock className="h-[240px]" />
      </LoadingSurface>
    );
  }

  if (
    review.error instanceof ApiError &&
    review.error.code === "RESULT_NOT_READY"
  ) {
    return (
      <DelayedState what="This session is still being evaluated">
        <p>
          Coaching follows the evaluation. This page checks again on its own.
        </p>
      </DelayedState>
    );
  }

  if (review.error) {
    const failure =
      review.error instanceof ApiError ? review.error : undefined;
    return (
      <ErrorState
        what="The coaching review could not be loaded"
        safe="Your evaluation is stored and unaffected; only this coaching view failed to load."
        action={
          <button
            type="button"
            className="btn btn-secondary"
            onClick={() => void review.refetch()}
          >
            Try again
          </button>
        }
        reference={failure?.requestId ?? "no request id"}
      />
    );
  }

  if (!review.data.coaching_available) {
    return (
      <section
        aria-label="Coaching unavailable"
        className="rounded-lg border border-border bg-surface p-5"
      >
        <h2 className="text-lg font-semibold">No coaching for this session</h2>
        <p className="mt-2 text-sm text-fg-2">
          {review.data.note ??
            "Coaching could not be derived for this session. Your evaluation is complete and unaffected."}
        </p>
        <Link
          className="btn btn-secondary mt-4 inline-block"
          href={`/session/${review.data.session_id}/results`}
        >
          Outcome and evidence
        </Link>
      </section>
    );
  }

  return (
    <div className="space-y-6">
      <p className="text-sm text-fg-2">
        Answer by answer: what worked, what a listener could not check, and a
        suggested shape built only from your own words. Where information is
        missing you get the question to answer, never an invented fact.
      </p>
      {review.data.answers.map((answer) => (
        <AnswerCard key={answer.sequence} answer={answer} />
      ))}
      <p className="text-xs text-fg-3">
        Derived by {review.data.coaching_version} from the session&apos;s own
        evidence. The outcome this coaching is based on never changes:{" "}
        <Link className="underline" href={`/session/${sessionId}/results`}>
          outcome and evidence
        </Link>
        .
      </p>
    </div>
  );
}

/** One answer's coaching: strengths, gaps, and the rewrite when one helps. */
function AnswerCard({ answer }: { answer: AnswerCoaching }) {
  return (
    <section
      data-testid={`answer-${answer.sequence}`}
      aria-label={`Coaching for answer at turn ${answer.sequence}`}
      className="rounded-lg border border-border bg-surface p-5"
    >
      <h2 className="text-sm font-semibold">Turn {answer.sequence}</h2>

      {answer.strengths.length > 0 && (
        <PointList title="What worked" points={answer.strengths} />
      )}
      {answer.gaps.length > 0 && (
        <PointList title="What to tighten" points={answer.gaps} />
      )}

      {answer.rewrite.length > 0 && (
        <div className="mt-4">
          <h3 className="text-xs font-semibold uppercase text-fg-3">
            Suggested rewrite
          </h3>
          <p className="mt-2 rounded-md border border-border bg-surface-2 p-3 text-sm leading-7">
            {answer.rewrite.map((part, index) =>
              part.kind === "quote" ? (
                <span key={index} data-part="quote">
                  {part.text}{" "}
                </span>
              ) : (
                <mark
                  key={index}
                  data-part="placeholder"
                  className="rounded bg-accent-soft px-1 font-medium not-italic"
                >
                  {part.text}{" "}
                </mark>
              ),
            )}
          </p>
          <p className="mt-1 text-xs text-fg-3">
            Highlighted brackets are questions to answer, not facts: nothing
            in a rewrite is ever invented for you.
          </p>
        </div>
      )}
    </section>
  );
}

function PointList({
  title,
  points,
}: {
  title: string;
  points: CoachingPoint[];
}) {
  return (
    <div className="mt-4">
      <h3 className="text-xs font-semibold uppercase text-fg-3">{title}</h3>
      <ul className="mt-2 space-y-3">
        {points.map((point, index) => (
          <li key={index} className="text-sm">
            <p>{point.statement}</p>
            <blockquote className="mt-1 border-l border-accent pl-3 text-fg-2">
              {point.quote}
            </blockquote>
          </li>
        ))}
      </ul>
    </div>
  );
}
