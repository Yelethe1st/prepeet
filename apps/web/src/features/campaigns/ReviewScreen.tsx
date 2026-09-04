"use client";

import { useQuery } from "@tanstack/react-query";

import { ApiError } from "@/lib/api/client";
import { Button } from "@/shared/components";
import { ErrorState, LoadingSurface, SkeletonText } from "@/shared/states";

import { DecisionPanel } from "./DecisionPanel";
import { fetchReview, type ScreeningReview } from "./api";

/**
 * The evidence-first review screen: REV-02, from the prototype's
 * admin-recruiter-detail, held to the ticket's boxes. Every conclusion sits
 * above the evidence behind it; every band travels with its confidence,
 * sufficiency counts and reason codes in the same card rather than a
 * footnote; and the screen says, in its own words at the top, that the
 * decision belongs to the reviewer - it renders no recommendation because
 * the server sends none, by contract.
 */

const REQUIREMENT_WORDS: Record<string, string> = {
  evidenced: "Evidenced",
  partial: "Partial",
  not_discussed: "Not discussed",
  not_assessable: "Not assessable in this interview",
};

export function ReviewScreen({
  campaignId,
  sessionId,
}: {
  campaignId: string;
  sessionId: string;
}) {
  const review = useQuery({
    queryKey: ["review", campaignId, sessionId],
    queryFn: () => fetchReview(campaignId, sessionId),
    retry: false,
  });

  if (review.isPending) {
    return (
      <LoadingSurface label="the screening review">
        <SkeletonText />
        <SkeletonText width="75" />
        <SkeletonText width="50" />
      </LoadingSurface>
    );
  }
  if (review.isError) {
    const failure = review.error;
    if (failure instanceof ApiError && failure.code === "REVIEW_NOT_READY") {
      return (
        <div className="rounded-md border border-border bg-surface-2 px-4 py-3 text-sm">
          <p className="font-semibold">
            This screening has not published its evaluation yet
          </p>
          <p className="mt-1 text-fg-2">
            It appears here the moment it does. Nothing needs doing; the
            candidate has finished their part.
          </p>
        </div>
      );
    }
    return (
      <ErrorState
        what="The review could not be loaded"
        safe="The evaluation and its evidence are unaffected; only this view failed."
        reference={
          failure instanceof ApiError && failure.requestId
            ? failure.requestId
            : "none"
        }
        action={
          <Button type="button" onClick={() => void review.refetch()}>
            Retry
          </Button>
        }
      />
    );
  }

  return (
    <div className="space-y-8">
      <ReviewDocument review={review.data} />
      <DecisionPanel
        campaignId={campaignId}
        sessionId={sessionId}
        assessed={review.data.competencies
          .filter(
            (competency) => competency.status === "assessed" && competency.band,
          )
          .map((competency) => ({
            competencyID: competency.competency_id,
            band: competency.band ?? "",
          }))}
      />
    </div>
  );
}

function ReviewDocument({ review }: { review: ScreeningReview }) {
  const spansByID = new Map(review.evidence.map((span) => [span.id, span]));

  return (
    <div className="space-y-6">
      {/* The frame: whose decision this is, stated before any evidence. */}
      <p className="rounded-md border border-border bg-surface-2 px-4 py-3 text-sm text-fg-2">
        What follows is evidence, not a verdict. The decision on this candidate
        belongs to you, recorded under your own name; Prepeet does not recommend
        an outcome and nothing on this page is one.
      </p>

      <section aria-labelledby="pinned-h">
        <h2 id="pinned-h" className="text-base font-semibold">
          What this session ran
        </h2>
        <dl className="mt-2 grid gap-x-6 gap-y-1 text-sm sm:grid-cols-2">
          <Pinned label="Bundle" value={review.pinned.bundle_digest} />
          <Pinned
            label={`Rubric ${review.pinned.rubric.reference} v${review.pinned.rubric.version}`}
            value={review.pinned.rubric.digest}
          />
          <Pinned
            label="Aggregation"
            value={review.pinned.aggregation_version}
          />
          <Pinned label="Extraction" value={review.pinned.extraction_version} />
          <Pinned label="Model" value={review.pinned.model_version} />
          <Pinned label="Runtime policy" value={review.pinned.policy_version} />
        </dl>
      </section>

      <section aria-labelledby="coverage-h">
        <h2 id="coverage-h" className="text-base font-semibold">
          Coverage
        </h2>
        <p className="mt-1 text-sm text-fg-2">
          The conversation reached {review.coverage.covered} of{" "}
          {review.coverage.total} competencies.
          {review.coverage.not_reached.length > 0
            ? ` Not reached: ${review.coverage.not_reached.join(", ")}. A competency the conversation never reached is the plan's gap, not the candidate's.`
            : " Every competency was reached."}
        </p>
      </section>

      <section aria-labelledby="competencies-h" className="space-y-3">
        <h2 id="competencies-h" className="text-base font-semibold">
          Competencies, with their evidence
        </h2>
        {review.competencies.map((competency) => (
          <article
            key={competency.competency_id}
            className="rounded-lg border border-border p-4"
            aria-label={`Competency ${competency.competency_id}`}
          >
            <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1 text-sm">
              <span className="font-semibold">{competency.competency_id}</span>
              {competency.status === "assessed" ? (
                <span>
                  {competency.band}
                  <span className="text-fg-3">
                    {" "}
                    · confidence {competency.confidence} ·{" "}
                    {competency.supporting} supporting,{" "}
                    {competency.contradictory} contradictory,{" "}
                    {competency.unverified} unverified
                  </span>
                </span>
              ) : (
                <span className="text-fg-2">
                  Unassessed
                  <span className="text-fg-3">
                    {" "}
                    ·{" "}
                    {competency.reason_codes.join(", ").toLowerCase() ||
                      "no evidence"}{" "}
                    · not a low score
                  </span>
                </span>
              )}
            </div>
            <ul className="mt-2 space-y-1 text-sm text-fg-2">
              {competency.evidence_ids.length === 0 ? (
                <li className="text-fg-3">No evidence recorded.</li>
              ) : (
                competency.evidence_ids.map((id) => {
                  const span = spansByID.get(id);
                  if (!span) {
                    return null;
                  }
                  return (
                    <li key={id}>
                      <span className="font-mono text-xs text-fg-3">
                        {timestamp(span.start_ms)} · {span.kind}
                      </span>{" "}
                      “{span.quote}”
                    </li>
                  );
                })
              )}
            </ul>
          </article>
        ))}
      </section>

      {review.contradictions.length > 0 ? (
        <section aria-labelledby="contradictions-h" className="space-y-2">
          <h2 id="contradictions-h" className="text-base font-semibold">
            Contradictions, stated neutrally
          </h2>
          {review.contradictions.map((pair, index) => (
            <div
              key={index}
              className="rounded-md border border-border p-3 text-sm text-fg-2"
            >
              <p>
                <span className="font-mono text-xs text-fg-3">
                  {timestamp(pair.side_a.start_ms)}
                </span>{" "}
                “{pair.side_a.quote}”
              </p>
              <p className="mt-1">
                <span className="font-mono text-xs text-fg-3">
                  {timestamp(pair.side_b.start_ms)}
                </span>{" "}
                “{pair.side_b.quote}”
              </p>
              <p className="mt-1 text-fg-3">
                Both were said. Whether they conflict is yours to weigh.
              </p>
            </div>
          ))}
        </section>
      ) : null}

      <section aria-labelledby="requirements-h" className="space-y-2">
        <h2 id="requirements-h" className="text-base font-semibold">
          Job requirements
        </h2>
        <p className="text-sm text-fg-3">
          Each on its own, never a match percentage.
        </p>
        {review.requirements.requirements.length === 0 ? (
          <p className="text-sm text-fg-2">
            This campaign froze no requirements.
          </p>
        ) : (
          review.requirements.requirements.map((finding) => (
            <div
              key={finding.requirement_id}
              className="rounded-md border border-border p-3 text-sm"
            >
              <p className="font-semibold">{finding.text}</p>
              <p className="mt-1 text-fg-2">
                {REQUIREMENT_WORDS[finding.status] ?? finding.status}
                {finding.evidence_ids.length > 0
                  ? ` · ${finding.evidence_ids.length} evidence span${finding.evidence_ids.length === 1 ? "" : "s"} attached above`
                  : ""}
              </p>
              {finding.follow_up ? (
                <p className="mt-1 text-fg-2">{finding.follow_up}</p>
              ) : null}
            </div>
          ))
        )}
      </section>
    </div>
  );
}

function Pinned({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between gap-4">
      <dt className="text-fg-3">{label}</dt>
      <dd className="truncate font-mono text-xs">{value}</dd>
    </div>
  );
}

function timestamp(ms: number): string {
  const total = Math.floor(ms / 1000);
  const minutes = Math.floor(total / 60);
  const seconds = total % 60;
  return `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
}
