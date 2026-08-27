"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { useRef } from "react";

import { ApiError } from "@/lib/api/client";
import {
  DelayedState,
  ErrorState,
  LoadingSurface,
  SkeletonBlock,
  SkeletonText,
} from "@/shared/states";

import {
  DRILLS,
  clock,
  paceSummary,
  titleOf,
  type Analysis,
  type Priority,
  type ShapePart,
  type TurnFeatures,
} from "./analysis";
import {
  getBaseline,
  getDelivery,
  getInterview,
  getTranscript,
  type DeliveryBaseline,
  type DeliveryView,
} from "./api";

/**
 * The delivery screen - ART-05, from the prototype's
 * candidate-session-articulation screen, with the deviations the ticket
 * records: no audio player yet (the recording is not served, so every
 * observation jumps to its transcript moment instead, keyboard included);
 * no personal ranges yet (ART-07 draws a baseline only after enough
 * sessions, and the copy says so rather than showing a target). Every
 * number is the calculator's, named on screen; nothing here adds the ten
 * dimensions up.
 */
export function DeliveryScreen({ sessionId }: { sessionId: string }) {
  const delivery = useQuery({
    queryKey: ["delivery", sessionId],
    queryFn: () => getDelivery(sessionId),
    retry: false,
    refetchInterval: (query) =>
      query.state.error instanceof ApiError &&
      query.state.error.code === "DELIVERY_NOT_READY"
        ? 5000
        : false,
  });
  const transcript = useQuery({
    queryKey: ["transcript", sessionId],
    queryFn: () => getTranscript(sessionId),
  });
  // The baseline is a convenience beside the numbers: its failure never
  // hides the session's own measurements.
  const baseline = useQuery({
    queryKey: ["delivery-baseline"],
    queryFn: getBaseline,
    retry: false,
  });
  // A retake compares itself with the original: the session says whether
  // it is one, and the original's own analysis supplies the other side.
  const session = useQuery({
    queryKey: ["interview", sessionId],
    queryFn: () => getInterview(sessionId),
  });
  const originalId = session.data?.redo_of?.session_id;
  const original = useQuery({
    queryKey: ["delivery", originalId],
    queryFn: () => getDelivery(originalId as string),
    enabled: Boolean(originalId),
    retry: false,
  });

  if (delivery.isPending || transcript.isPending) {
    return (
      <LoadingSurface label="Loading the delivery analysis">
        <SkeletonBlock className="h-[120px]" />
        <SkeletonText />
        <SkeletonBlock className="h-[240px]" />
      </LoadingSurface>
    );
  }
  if (
    delivery.error instanceof ApiError &&
    delivery.error.code === "DELIVERY_NOT_READY"
  ) {
    return (
      <DelayedState what="Delivery is still being measured">
        <p>
          It runs beside the evaluation and never delays it. This page checks
          again on its own.
        </p>
      </DelayedState>
    );
  }
  if (delivery.error || transcript.error) {
    const failure =
      delivery.error instanceof ApiError
        ? delivery.error
        : transcript.error instanceof ApiError
          ? transcript.error
          : undefined;
    return (
      <ErrorState
        what="The delivery analysis could not be loaded"
        safe="Your evaluation and coaching are stored and unaffected; only this delivery view failed to load."
        action={
          <button
            type="button"
            className="btn btn-secondary"
            onClick={() => void delivery.refetch()}
          >
            Try again
          </button>
        }
        reference={failure?.requestId ?? "no request id"}
      />
    );
  }

  return (
    <DeliveryBody
      sessionId={sessionId}
      delivery={delivery.data}
      baseline={baseline.data}
      segments={transcript.data.segments.filter((s) => !s.superseded)}
      redoOf={session.data?.redo_of}
      original={original.data}
    />
  );
}

function DeliveryBody({
  sessionId,
  delivery,
  baseline,
  segments,
  redoOf,
  original,
}: {
  sessionId: string;
  delivery: DeliveryView;
  baseline: DeliveryBaseline | undefined;
  segments: {
    sequence: number;
    speaker: string;
    text: string;
    start_ms: number;
  }[];
  redoOf?: { session_id: string; sequence: number; question: string };
  original?: DeliveryView;
}) {
  const analysis = delivery.analysis as Analysis;
  const turns = analysis.turns ?? [];
  const dimensions = analysis.profile?.dimensions ?? {};
  const coaching =
    analysis.coaching && !("available" in analysis.coaching)
      ? analysis.coaching
      : undefined;
  const withheld =
    analysis.coaching && "available" in analysis.coaching
      ? analysis.coaching.note
      : undefined;
  const segmentRefs = useRef(new Map<number, HTMLDivElement>());
  const jumpTo = (sequence: number) => {
    const target = segmentRefs.current.get(sequence);
    if (target) {
      target.scrollIntoView?.({ block: "center" });
      target.focus();
    }
  };
  const selectedDrills = new Set(
    (coaching?.priorities ?? []).map((p) => p.drill),
  );
  const drills = [...DRILLS].sort(
    (a, b) =>
      Number(selectedDrills.has(b.key)) - Number(selectedDrills.has(a.key)),
  );

  return (
    <div className="space-y-6">
      {redoOf && (
        <Comparison redoOf={redoOf} redo={analysis} original={original} />
      )}
      {delivery.status === "not_assessable" && (
        <section
          role="status"
          aria-label="Delivery status"
          className="rounded-lg border border-border bg-surface p-5"
        >
          <h2 className="text-lg font-semibold">Delivery was not assessable</h2>
          <p className="mt-2 text-sm text-fg-2">{delivery.note}</p>
          {delivery.warnings.length > 0 && (
            <p className="mt-2 font-mono text-xs text-fg-3">
              {delivery.warnings.join(" · ")}
            </p>
          )}
        </section>
      )}

      <section
        aria-labelledby="measured-heading"
        className="rounded-lg border border-border bg-surface p-5"
      >
        <div className="flex items-center justify-between">
          <h2 id="measured-heading" className="text-lg font-semibold">
            What we measured
          </h2>
          <span className="font-mono text-xs text-fg-3">
            {delivery.calculation_version}
          </span>
        </div>
        <p className="mt-1 text-sm text-fg-2">
          Counted from the transcript&apos;s word timings. A model cannot invent
          these numbers, and it did not produce them.
        </p>
        <dl className="mt-4 grid grid-cols-2 gap-4 sm:grid-cols-4">
          <Metric
            label="Words per minute"
            value={analysis.metrics?.words_per_minute}
            range={
              baseline?.ready ? baseline.ranges["words_per_minute"] : undefined
            }
          />
          <Metric
            label="Fillers per 100 words"
            value={analysis.metrics?.fillers_per_100_words}
            range={
              baseline?.ready
                ? baseline.ranges["fillers_per_100_words"]
                : undefined
            }
          />
          <Metric
            label="Pauses over 700 ms"
            value={analysis.metrics?.long_pause_count}
            range={
              baseline?.ready ? baseline.ranges["long_pause_count"] : undefined
            }
          />
          <Metric label="Words measured" value={analysis.metrics?.words} />
        </dl>
        <p className="mt-4 text-sm text-fg-2" data-testid="baseline-note">
          {baseline?.ready
            ? baseline.note
            : baseline
              ? `There is no correct speaking rate. Your own range appears here after ${baseline.minimum_sessions} measured practice sessions; ${Math.max(0, baseline.minimum_sessions - baseline.sessions_measured)} more to go.`
              : "There is no correct speaking rate. Your own range appears here once enough practice sessions have been measured."}
        </p>
      </section>

      <section
        aria-labelledby="pace-heading"
        className="rounded-lg border border-border bg-surface p-5"
      >
        <h2 id="pace-heading" className="text-lg font-semibold">
          Pace and pauses, answer by answer
        </h2>
        <p id="pace-summary" className="mt-2 text-sm text-fg-2">
          {paceSummary(turns)}
        </p>
        <div
          role="img"
          aria-labelledby="pace-summary"
          className="mt-4 space-y-2"
        >
          {turns.map((turn) => (
            <div
              key={turn.sequence}
              className="flex items-center gap-3 text-xs"
            >
              <span className="w-16 font-mono">turn {turn.sequence}</span>
              <span
                className="h-3 rounded bg-accent-soft"
                style={{
                  width: `${Math.min(100, Math.round(turn.words_per_minute / 2.5))}%`,
                }}
              />
              <span className="text-fg-3">
                {turn.status === "assessable"
                  ? `${turn.words_per_minute} wpm`
                  : "not measured"}
              </span>
            </div>
          ))}
        </div>
        <details className="mt-4">
          <summary className="cursor-pointer text-sm underline">
            Read this as a table
          </summary>
          <table className="mt-3 w-full text-left text-sm">
            <caption className="sr-only">
              Speaking rate, long pauses and length for each answer
            </caption>
            <thead>
              <tr>
                <th scope="col">Answer</th>
                <th scope="col">Words per minute</th>
                <th scope="col">Pauses over 700 ms</th>
                <th scope="col">Words</th>
                <th scope="col">Measured</th>
              </tr>
            </thead>
            <tbody>
              {turns.map((turn) => (
                <PaceRow key={turn.sequence} turn={turn} />
              ))}
            </tbody>
          </table>
        </details>
      </section>

      <section
        aria-labelledby="dimensions-heading"
        className="rounded-lg border border-border bg-surface p-5"
      >
        <h2 id="dimensions-heading" className="text-lg font-semibold">
          Ten things a listener notices
        </h2>
        <p className="mt-1 text-sm text-fg-2">
          Each one is judged on its own. We do not add them up: a single
          delivery percentage would hide exactly the thing you need to see.
        </p>
        <ul className="mt-4 space-y-3">
          {Object.entries(dimensions).map(([name, dimension]) => (
            <li
              key={name}
              data-testid={`dimension-${name}`}
              className="rounded-md border border-border bg-surface-2 p-3 text-sm"
            >
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-semibold">{titleOf(name)}</span>
                <span className="rounded-full border border-border px-2 py-0.5 text-xs capitalize">
                  {dimension.level.replaceAll("_", " ")}
                </span>
              </div>
              <p className="mt-1 text-fg-2">{dimension.reason}</p>
              {dimension.evidence_sequences.length > 0 && (
                <p className="mt-1 flex flex-wrap gap-2 text-xs">
                  {dimension.evidence_sequences.map((sequence) => (
                    <JumpButton
                      key={sequence}
                      sequence={sequence}
                      segments={segments}
                      onJump={jumpTo}
                    />
                  ))}
                </p>
              )}
            </li>
          ))}
        </ul>
      </section>

      <section
        aria-labelledby="priorities-heading"
        className="rounded-lg border border-border bg-surface p-5"
      >
        <h2 id="priorities-heading" className="text-lg font-semibold">
          What to change next
        </h2>
        <p className="mt-1 text-sm text-fg-2">Not ten. At most two.</p>
        {withheld && (
          <p className="mt-3 text-sm text-fg-2">
            Coaching was withheld for this session: {withheld}
          </p>
        )}
        {coaching &&
          coaching.priorities &&
          coaching.priorities.length === 0 && (
            <p className="mt-3 text-sm text-fg-2">
              Nothing measurable needs changing in this session.
            </p>
          )}
        <ol className="mt-4 space-y-4">
          {(coaching?.priorities ?? []).map((priority) => (
            <PriorityCard
              key={priority.dimension}
              priority={priority}
              segments={segments}
              onJump={jumpTo}
            />
          ))}
        </ol>
        {coaching?.suggested_shape && coaching.suggested_shape.length > 0 && (
          <SuggestedShape parts={coaching.suggested_shape} />
        )}
      </section>

      <section
        aria-labelledby="replay-heading"
        className="rounded-lg border border-border bg-surface p-5"
      >
        <h2 id="replay-heading" className="text-lg font-semibold">
          Hear the moment
        </h2>
        <p className="mt-2 text-sm text-fg-2">
          Audio playback is not available yet: the recording is stored but not
          yet served. Every timestamp on this page jumps to the same moment in
          the transcript, and will play the recording when it arrives.
        </p>
      </section>

      <section
        aria-labelledby="drills-heading"
        className="rounded-lg border border-border bg-surface p-5"
      >
        <h2 id="drills-heading" className="text-lg font-semibold">
          Delivery drills
        </h2>
        <p className="mt-1 text-sm text-fg-2">
          Short and spoken out loud. The ones selected from this session sit at
          the top; the rest are always available.
        </p>
        <ul className="mt-4 space-y-2">
          {drills.map((drill) => (
            <li
              key={drill.key}
              data-testid={`drill-${drill.key}`}
              className="rounded-md border border-border bg-surface-2 p-3 text-sm"
            >
              <div className="flex items-center gap-2">
                <span className="font-semibold">{drill.title}</span>
                <span className="text-xs text-fg-3">
                  about {drill.minutes} minutes
                </span>
                {selectedDrills.has(drill.key) && (
                  <span className="rounded-full bg-accent-soft px-2 py-0.5 text-xs font-semibold">
                    selected for you
                  </span>
                )}
              </div>
              <p className="mt-1 text-fg-2">{drill.how}</p>
            </li>
          ))}
        </ul>
      </section>

      <section
        aria-labelledby="transcript-heading"
        className="rounded-lg border border-border bg-surface p-5"
      >
        <h2 id="transcript-heading" className="text-lg font-semibold">
          Transcript
        </h2>
        <div className="mt-4 space-y-3">
          {segments.map((segment) => (
            <div
              key={segment.sequence}
              data-testid={`segment-${segment.sequence}`}
              ref={(node) => {
                if (node) segmentRefs.current.set(segment.sequence, node);
              }}
              tabIndex={-1}
              className="rounded-md border border-border bg-surface-2 p-3 focus:outline focus:outline-2 focus:outline-accent"
            >
              <p className="flex items-center gap-2 text-xs text-fg-3">
                <span className="font-semibold uppercase">
                  {segment.speaker}
                </span>
                <span className="font-mono">{clock(segment.start_ms)}</span>
              </p>
              <p className="mt-1 text-sm">{segment.text}</p>
            </div>
          ))}
        </div>
        <p className="mt-4 text-sm">
          <Link className="underline" href={`/session/${sessionId}/results`}>
            Outcome and evidence
          </Link>
          {" · "}
          <Link className="underline" href={`/session/${sessionId}/review`}>
            Coaching review
          </Link>
        </p>
      </section>
    </div>
  );
}

/**
 * Original versus redo: the question the retake answered, and the
 * measured deltas. The original is another session's own analysis, so
 * nothing here can overwrite it; a missing original is said, not hidden.
 */
function Comparison({
  redoOf,
  redo,
  original,
}: {
  redoOf: { session_id: string; sequence: number; question: string };
  redo: Analysis;
  original: DeliveryView | undefined;
}) {
  const before = (original?.analysis as Analysis | undefined)?.metrics;
  const after = redo.metrics;
  const rows: {
    label: string;
    key: "words_per_minute" | "fillers_per_100_words" | "long_pause_count";
  }[] = [
    { label: "Words per minute", key: "words_per_minute" },
    { label: "Fillers per 100 words", key: "fillers_per_100_words" },
    { label: "Pauses over 700 ms", key: "long_pause_count" },
  ];
  return (
    <section
      data-testid="comparison"
      aria-labelledby="comparison-heading"
      className="rounded-lg border border-border bg-surface p-5"
    >
      <h2 id="comparison-heading" className="text-lg font-semibold">
        Your redo, next to the original
      </h2>
      <p className="mt-2 text-sm text-fg-2">
        This session retook one answer. The question it answered:{" "}
        <q>{redoOf.question}</q>
      </p>
      {before ? (
        <table className="mt-4 w-full text-left text-sm">
          <caption className="sr-only">
            Original and redo measurements with the change between them
          </caption>
          <thead>
            <tr>
              <th scope="col">Measurement</th>
              <th scope="col">Original</th>
              <th scope="col">Redo</th>
              <th scope="col">Change</th>
            </tr>
          </thead>
          <tbody>
            {rows.map(({ label, key }) => {
              const a = before[key];
              const b = after?.[key];
              const delta =
                a !== undefined && b !== undefined
                  ? Math.round((b - a) * 10) / 10
                  : undefined;
              return (
                <tr key={key} data-testid={`delta-${key}`}>
                  <th scope="row">{label}</th>
                  <td>{a ?? "not measured"}</td>
                  <td>{b ?? "not measured"}</td>
                  <td>
                    {delta === undefined
                      ? "n/a"
                      : delta > 0
                        ? `+${delta}`
                        : `${delta}`}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      ) : (
        <p className="mt-3 text-sm text-fg-2">
          The original session&apos;s delivery analysis is not available to
          compare yet.
        </p>
      )}
      <p className="mt-3 text-sm">
        <Link
          className="underline"
          href={`/session/${redoOf.session_id}/delivery`}
        >
          The original session&apos;s delivery
        </Link>
      </p>
    </section>
  );
}

function Metric({
  label,
  value,
  range,
}: {
  label: string;
  value: number | undefined;
  range?: { low: number; high: number };
}) {
  return (
    <div>
      <dt className="text-xs text-fg-3">{label}</dt>
      <dd className="text-xl font-semibold">
        {value === undefined ? "not measured" : value}
      </dd>
      {range && (
        <dd className="text-xs text-fg-3">
          your usual range {range.low} to {range.high}
        </dd>
      )}
    </div>
  );
}

function PaceRow({ turn }: { turn: TurnFeatures }) {
  const measured = turn.status === "assessable";
  return (
    <tr>
      <th scope="row" className="font-mono">
        turn {turn.sequence}
      </th>
      <td>{measured ? turn.words_per_minute : "not measured"}</td>
      <td>{measured ? turn.long_pause_count : "not measured"}</td>
      <td>{turn.words}</td>
      <td>
        {measured
          ? "yes"
          : turn.warnings
              .filter((w) => w !== "AUDIO_QUALITY_NOT_COMPUTED")
              .join(", ") || "no"}
      </td>
    </tr>
  );
}

function JumpButton({
  sequence,
  segments,
  onJump,
}: {
  sequence: number;
  segments: { sequence: number; start_ms: number }[];
  onJump: (sequence: number) => void;
}) {
  const segment = segments.find((s) => s.sequence === sequence);
  const label = segment ? clock(segment.start_ms) : `turn ${sequence}`;
  return (
    <button
      type="button"
      className="font-mono underline"
      onClick={() => onJump(sequence)}
      aria-label={`Jump to ${label} in the transcript`}
    >
      {label}
    </button>
  );
}

function PriorityCard({
  priority,
  segments,
  onJump,
}: {
  priority: Priority;
  segments: { sequence: number; start_ms: number }[];
  onJump: (sequence: number) => void;
}) {
  return (
    <li
      data-testid={`priority-${priority.dimension}`}
      className="rounded-md border border-border bg-surface-2 p-4 text-sm"
    >
      <h3 className="font-semibold">{titleOf(priority.dimension)}</h3>
      <p className="mt-1 text-fg-2">
        <span className="font-semibold">Listener impact: </span>
        {priority.listener_impact}
      </p>
      <p className="mt-1">
        <span className="font-semibold">One action: </span>
        {priority.action}
      </p>
      <p className="mt-2 flex flex-wrap gap-2 text-xs">
        {priority.evidence_sequences.map((sequence) => (
          <JumpButton
            key={sequence}
            sequence={sequence}
            segments={segments}
            onJump={onJump}
          />
        ))}
      </p>
    </li>
  );
}

function SuggestedShape({ parts }: { parts: ShapePart[] }) {
  return (
    <div className="mt-5">
      <h3 className="text-xs font-semibold uppercase text-fg-3">
        A shape built from your own sentences
      </h3>
      <ol className="mt-2 space-y-2 text-sm">
        {parts.map((part) => (
          <li
            key={part.slot}
            className="rounded-md border border-border bg-surface-2 p-3"
          >
            <span className="mr-2 text-xs uppercase text-fg-3">
              {part.slot}
            </span>
            {part.kind === "quote" ? (
              <span data-part="quote">{part.text}</span>
            ) : (
              <mark
                data-part="placeholder"
                className="rounded bg-accent-soft px-1 font-medium"
              >
                {part.text}
              </mark>
            )}
          </li>
        ))}
      </ol>
      <p className="mt-1 text-xs text-fg-3">
        Highlighted brackets are questions to answer, not facts: nothing here is
        invented for you.
      </p>
    </div>
  );
}
