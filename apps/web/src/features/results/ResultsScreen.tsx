"use client";

import { useQuery } from "@tanstack/react-query";
import { useRef, useState } from "react";

import { ApiError } from "@/lib/api/client";
import {
  DelayedState,
  ErrorState,
  InsufficientEvidenceState,
  LoadingSurface,
  SkeletonBlock,
  SkeletonText,
} from "@/shared/states";

import {
  getResults,
  getTranscript,
  type CompetencyResult,
  type Contradiction,
  type EvaluationResult,
  type EvidenceSpan,
  type TranscriptSegment,
} from "./api";

/**
 * The outcome and evidence screen - PRC-01, from the prototype's
 * candidate-session-results screen, with two deliberate deviations the
 * tickets record:
 *
 * - No numeric score. The prototype shows a 74/100 ring; ADR-0015 forbids
 *   numeric confidence or score display until QUA-03 calibrates, so the
 *   outcome is the per-competency band and confidence label, with the
 *   server's own framing sentence beside it.
 * - No audio player yet. The recording pipeline (RTC-05) has not shipped,
 *   so replay is an honest absence; every evidence timestamp still jumps
 *   into the transcript, keyboard included, and the same jump will drive
 *   the player when audio exists.
 *
 * Everything shown is the server's: bands, confidence, reasons, framing
 * copy and the exact quoted sentences behind each score. This page is
 * read-only and never changes - the coaching review is a different screen.
 */
export function ResultsScreen({ sessionId }: { sessionId: string }) {
  const results = useQuery({
    queryKey: ["results", sessionId],
    queryFn: () => getResults(sessionId),
    retry: false,
    // While evaluation runs the server answers RESULT_NOT_READY; poll it,
    // because readiness arrives from the workflow, not from the person.
    refetchInterval: (query) =>
      query.state.error instanceof ApiError &&
      query.state.error.code === "RESULT_NOT_READY"
        ? 5000
        : false,
  });
  const transcript = useQuery({
    queryKey: ["transcript", sessionId],
    queryFn: () => getTranscript(sessionId),
  });

  if (results.isPending || transcript.isPending) {
    return (
      <LoadingSurface label="Loading the outcome and its evidence">
        <SkeletonBlock className="h-[140px]" />
        <SkeletonText />
        <SkeletonText width="75" />
        <SkeletonBlock className="h-[280px]" />
      </LoadingSurface>
    );
  }

  if (
    results.error instanceof ApiError &&
    results.error.code === "RESULT_NOT_READY"
  ) {
    return (
      <DelayedState what="This session is still being evaluated">
        <p>
          Evaluation usually finishes within a few minutes of the interview
          ending. This page checks again on its own.
        </p>
      </DelayedState>
    );
  }

  if (results.error || transcript.error) {
    const failure =
      results.error instanceof ApiError
        ? results.error
        : transcript.error instanceof ApiError
          ? transcript.error
          : undefined;
    return (
      <ErrorState
        what="The outcome could not be loaded"
        safe="Nothing about your session was changed; the result is stored and will load once this is resolved."
        action={
          <button
            type="button"
            className="btn btn-secondary"
            onClick={() => {
              void results.refetch();
              void transcript.refetch();
            }}
          >
            Try again
          </button>
        }
        reference={failure?.requestId ?? "no request id"}
      />
    );
  }

  return (
    <ResultsBody
      result={results.data}
      segments={transcript.data.segments.filter((s) => !s.superseded)}
    />
  );
}

function ResultsBody({
  result,
  segments,
}: {
  result: EvaluationResult;
  segments: TranscriptSegment[];
}) {
  const segmentRefs = useRef(new Map<number, HTMLDivElement>());
  const spansById = new Map(result.evidence.map((span) => [span.id, span]));

  /** Jump focus to a transcript segment: the evidence-to-source link. */
  const jumpTo = (sequence: number) => {
    const target = segmentRefs.current.get(sequence);
    if (target) {
      // jsdom has no scrollIntoView; focus alone is the observable contract.
      target.scrollIntoView?.({ block: "center" });
      target.focus();
    }
  };

  const evidencedSequences = new Set(
    result.evidence.map((span) => span.segment_sequence),
  );

  return (
    <div className="space-y-6">
      <section
        aria-labelledby="outcome-heading"
        className="rounded-lg border border-border bg-surface p-5"
      >
        <div className="flex items-center justify-between">
          <h2 id="outcome-heading" className="text-lg font-semibold">
            Outcome
          </h2>
          <span className="rounded-full border border-border px-3 py-1 text-xs">
            Visible only to you
          </span>
        </div>
        <p className="mt-2 text-sm text-fg-2">{result.framing.confidence}</p>
        <p className="mt-2 text-sm text-fg-2">
          {result.covered_competencies} of {result.total_competencies}{" "}
          competencies were reached by this conversation. Scored against{" "}
          <span className="font-mono text-xs">
            {result.rubric.reference} v{result.rubric.version}
          </span>
          , pinned when the session was composed.
        </p>

        <ul className="mt-4 space-y-3">
          {result.competencies.map((competency) => (
            <li
              key={competency.competency_id}
              data-testid={`competency-${competency.competency_id}`}
            >
              <CompetencyRow competency={competency} />
            </li>
          ))}
        </ul>
      </section>

      <section
        aria-labelledby="evidence-heading"
        className="rounded-lg border border-border bg-surface p-5"
      >
        <h2 id="evidence-heading" className="text-lg font-semibold">
          Evidence behind each result
        </h2>
        <p className="mt-1 text-sm text-fg-2">
          Open a competency to read the exact words that produced its result,
          with a control to find the moment in the transcript.
        </p>
        <div className="mt-4 space-y-2">
          {result.competencies
            .filter((competency) => competency.evidence_ids.length > 0)
            .map((competency) => (
              <EvidenceAccordion
                key={competency.competency_id}
                competency={competency}
                spans={competency.evidence_ids
                  .map((id) => spansById.get(id))
                  .filter((span): span is EvidenceSpan => span !== undefined)}
                framingUnverified={result.framing.unverified}
                onJump={jumpTo}
              />
            ))}
        </div>
      </section>

      {result.contradictions.length > 0 && (
        <section
          aria-labelledby="contradictions-heading"
          className="rounded-lg border border-border bg-surface p-5"
        >
          <h2 id="contradictions-heading" className="text-lg font-semibold">
            Statements to clarify
          </h2>
          <p className="mt-1 text-sm text-fg-2">
            {result.framing.contradictions}
          </p>
          <ul className="mt-4 space-y-4">
            {result.contradictions.map((pair, index) => (
              <li key={index}>
                <ContradictionPair pair={pair} onJump={jumpTo} />
              </li>
            ))}
          </ul>
        </section>
      )}

      <section
        aria-labelledby="replay-heading"
        className="rounded-lg border border-border bg-surface p-5"
      >
        <h2 id="replay-heading" className="text-lg font-semibold">
          Replay
        </h2>
        <p className="mt-2 text-sm text-fg-2">
          Audio replay is not available yet: the recording pipeline has not
          shipped. Every evidence timestamp on this page jumps to the same
          moment in the transcript below, and will drive the recording when
          it arrives.
        </p>
      </section>

      <section
        aria-labelledby="transcript-heading"
        className="rounded-lg border border-border bg-surface p-5"
      >
        <h2 id="transcript-heading" className="text-lg font-semibold">
          Transcript
        </h2>
        <p className="mt-1 text-sm text-fg-2">
          Everything said, on the session&apos;s own clock. Turns that carry
          evidence are marked.
        </p>
        <div className="mt-4 space-y-3">
          {segments.map((segment) => (
            <div
              key={`${segment.epoch}-${segment.sequence}`}
              data-testid={`segment-${segment.sequence}`}
              ref={(node) => {
                if (node) {
                  segmentRefs.current.set(segment.sequence, node);
                }
              }}
              tabIndex={-1}
              className="rounded-md border border-border bg-surface-2 p-3 focus:outline focus:outline-2 focus:outline-accent"
            >
              <p className="flex items-center gap-2 text-xs text-fg-3">
                <span className="font-semibold uppercase">
                  {segment.speaker}
                </span>
                <span className="font-mono">{clock(segment.start_ms)}</span>
                {evidencedSequences.has(segment.sequence) && (
                  <span className="rounded-full border border-border px-2">
                    evidence
                  </span>
                )}
              </p>
              <p className="mt-1 text-sm">{segment.text}</p>
            </div>
          ))}
        </div>
      </section>

      {result.coverage.not_reached.length > 0 && (
        <section
          aria-labelledby="coverage-heading"
          className="rounded-lg border border-border bg-surface p-5"
        >
          <h2 id="coverage-heading" className="text-lg font-semibold">
            Coverage and gaps
          </h2>
          <p className="mt-1 text-sm text-fg-2">
            These competencies never came up, so they are absent from the
            outcome rather than scored low. A missing measurement is not a
            low one.
          </p>
          <ul className="mt-3 list-inside list-disc text-sm">
            {result.coverage.not_reached.map((id) => (
              <li key={id}>{titleOf(id)}</li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}

/** One competency's outcome: a band when assessed, the honest state when not. */
function CompetencyRow({ competency }: { competency: CompetencyResult }) {
  if (competency.status === "unassessed") {
    const neverRaised = competency.reason_codes.includes("NOT_DISCUSSED");
    return (
      <InsufficientEvidenceState
        what={
          neverRaised
            ? `${titleOf(competency.competency_id)}: this never came up`
            : `${titleOf(competency.competency_id)}: insufficient evidence`
        }
        remedy={
          neverRaised
            ? "A session whose questions reach this competency would measure it."
            : "One or two more concrete examples with outcomes would be enough to assess this."
        }
      />
    );
  }
  return (
    <div className="flex flex-wrap items-center gap-3 rounded-md border border-border bg-surface-2 px-4 py-3">
      <span className="text-sm font-semibold">
        {titleOf(competency.competency_id)}
      </span>
      <span className="rounded-full bg-accent-soft px-3 py-0.5 text-xs font-semibold capitalize">
        {competency.band}
      </span>
      <span className="rounded-full border border-border px-3 py-0.5 text-xs capitalize">
        {competency.confidence} confidence
      </span>
      <span className="text-xs text-fg-3">
        {competency.evidence_count} evidence span
        {competency.evidence_count === 1 ? "" : "s"}
      </span>
    </div>
  );
}

/** The expandable exact-sentences view behind one competency. */
function EvidenceAccordion({
  competency,
  spans,
  framingUnverified,
  onJump,
}: {
  competency: CompetencyResult;
  spans: EvidenceSpan[];
  framingUnverified: string;
  onJump: (sequence: number) => void;
}) {
  const [open, setOpen] = useState(false);
  const hasUnverified = spans.some((span) => span.kind === "claim_unverified");
  return (
    <div className="rounded-md border border-border">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((current) => !current)}
        className="flex w-full items-center justify-between px-4 py-3 text-left text-sm font-semibold"
      >
        <span>
          Evidence for {titleOf(competency.competency_id)}
          <span className="ml-2 font-normal text-fg-3">
            {spans.length} span{spans.length === 1 ? "" : "s"}
          </span>
        </span>
        <span aria-hidden="true">{open ? "−" : "+"}</span>
      </button>
      {open && (
        <div className="space-y-3 border-t border-border px-4 py-3">
          {spans.map((span) => (
            <figure key={span.id} className="text-sm">
              <blockquote className="border-l border-accent pl-3">
                {span.quote}
              </blockquote>
              <figcaption className="mt-1 flex items-center gap-2 text-xs text-fg-3">
                <span className="capitalize">{kindLabel(span.kind)}</span>
                <button
                  type="button"
                  className="font-mono underline"
                  onClick={() => onJump(span.segment_sequence)}
                  aria-label={`Jump to ${clock(span.start_ms)} in the transcript`}
                >
                  {clock(span.start_ms)}
                </button>
              </figcaption>
            </figure>
          ))}
          {hasUnverified && (
            <p className="text-xs text-fg-3">{framingUnverified}</p>
          )}
        </div>
      )}
    </div>
  );
}

/** Both sides of one contradiction, each with its moment. */
function ContradictionPair({
  pair,
  onJump,
}: {
  pair: Contradiction;
  onJump: (sequence: number) => void;
}) {
  return (
    <div className="rounded-md border border-border bg-surface-2 p-4 text-sm">
      {[pair.side_a, pair.side_b].map((side, index) => (
        <p key={index} className={index === 0 ? "" : "mt-2"}>
          <button
            type="button"
            className="mr-2 font-mono text-xs underline"
            onClick={() => onJump(side.segment_sequence)}
            aria-label={`Jump to ${clock(side.start_ms)} in the transcript`}
          >
            {clock(side.start_ms)}
          </button>
          <q>{side.quote}</q>
        </p>
      ))}
      <p className="mt-2 text-xs text-fg-3">
        About: {pair.topic.join(", ")}
      </p>
    </div>
  );
}

/** mm:ss on the session's own clock. */
function clock(ms: number): string {
  const total = Math.floor(ms / 1000);
  const minutes = Math.floor(total / 60);
  const seconds = total % 60;
  return `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
}

/** A competency id as a human title: hyphens to spaces, sentence case. */
function titleOf(id: string): string {
  const words = id.replaceAll("-", " ");
  return words.charAt(0).toUpperCase() + words.slice(1);
}

/** The legend's words for a span kind, matching the prototype's legend. */
function kindLabel(kind: string): string {
  switch (kind) {
    case "supporting":
      return "supporting";
    case "contradictory":
      return "contradiction";
    case "claim_unverified":
      return "unverified claim";
    case "gap":
      return "acknowledged gap";
    default:
      return kind;
  }
}
