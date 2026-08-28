import { ArrowRight, FileSearch } from "lucide-react";

import { ButtonLink } from "@/shared/components";
import { Icon } from "@/shared/components/Icon";

import { evidence } from "./content";

/** The border colour that says which of the four types a span or a legend entry is. */
const evidenceTone: Record<
  "supporting" | "contradiction" | "claim" | "gap",
  string
> = {
  supporting: "border-ev-supporting bg-ev-supporting/20",
  contradiction: "border-ev-contradiction bg-ev-contradiction/20",
  claim: "border-ev-claim bg-ev-claim/20",
  gap: "border-ev-gap bg-ev-gap/20",
};

/**
 * Evidence and feedback, ported from the seventh section.
 *
 * Two worked examples sit beside the claim: a competency with enough evidence
 * to publish, and one without, reported as a gap rather than as a low number.
 * The second is the one that matters, because withholding a score is the
 * behaviour nobody believes until they see it.
 *
 * Two recorded deviations.
 *
 * The prototype explains a tagged span in a `data-tooltip`, which needs a
 * pointer: it cannot be reached by touch and appears for a keyboard only while
 * the span holds focus. The explanation is text here, next to the span it
 * explains and available to everybody.
 *
 * The prototype's "insufficient evidence" band is drawn in the grey of the
 * score palette. It uses the neutral surface here, because a withheld score is
 * not a bad score and the states this product ships already refuse to render it
 * as a failure.
 */
export function Evidence() {
  return (
    <section
      id="evidence"
      aria-labelledby="evidence-h"
      className="mx-auto max-w-[1180px] px-5 py-16 md:px-6 md:py-24"
    >
      <div className="grid grid-cols-1 items-center gap-7 lg:grid-cols-2 lg:gap-12">
        <div className="lg:order-2">
          <p className="text-2xs font-bold tracking-[0.1em] text-primary uppercase">
            {evidence.eyebrow}
          </p>
          <h2
            id="evidence-h"
            className="mt-2.5 font-display text-[clamp(1.6rem,2.8vw,2.25rem)] leading-[1.15] font-medium tracking-[-0.02em]"
          >
            {evidence.heading}
          </h2>
          <p className="mt-3.5 leading-relaxed text-fg-2">
            {evidence.leadBefore}
            <strong className="font-semibold text-fg">
              {evidence.leadEmphasis}
            </strong>
            {evidence.leadAfter}
          </p>

          <ul className="mt-4.5 flex flex-col gap-2.5">
            {evidence.points.map((point) => (
              <li
                key={point.lead}
                className="flex items-start gap-2.5 text-sm leading-normal text-fg-2"
              >
                <span className="mt-0.5">
                  <Icon glyph={point.glyph} size="sm" tone="text-primary" />
                </span>
                <span>
                  <strong className="font-semibold text-fg">
                    {point.lead}
                  </strong>{" "}
                  {point.body}
                </span>
              </li>
            ))}
          </ul>

          <div className="mt-6 flex flex-wrap gap-2">
            <ButtonLink href="#how" variant="secondary">
              <Icon glyph={FileSearch} size="sm" />
              See who is shown what
            </ButtonLink>
            <ButtonLink href="#faq" variant="ghost">
              How bias is managed
              <Icon glyph={ArrowRight} size="sm" />
            </ButtonLink>
          </div>
        </div>

        <div className="flex flex-col gap-4 lg:order-1">
          <article className="rounded-md border border-border border-l-[3px] border-l-ev-supporting bg-surface px-3.5 py-3">
            <div className="mb-1.5 flex flex-wrap items-center justify-between gap-2">
              <h3 className="text-sm font-semibold">
                {evidence.supportingCard.title}
              </h3>
              <span className="inline-flex items-center rounded-pill bg-success-soft px-2.5 py-1 text-2xs font-semibold text-success-fg">
                {evidence.supportingCard.band}
              </span>
            </div>
            <blockquote className="my-1.5 border-l-2 border-border-strong pl-2.5 text-sm leading-relaxed text-fg-2 italic">
              {evidence.supportingCard.parts.map((part, index) =>
                "kind" in part ? (
                  <span key={index}>
                    <mark
                      className={
                        "rounded-[3px] border-b-2 px-[3px] py-px text-fg " +
                        (part.kind === "supporting"
                          ? "border-ev-supporting bg-ev-supporting/15"
                          : "border-ev-claim bg-ev-claim/15")
                      }
                    >
                      {part.text}
                    </mark>
                    <span className="sr-only"> ({part.meaning}) </span>
                  </span>
                ) : (
                  <span key={index}>{part.text}</span>
                ),
              )}
            </blockquote>
            <p className="mt-2.5 flex flex-wrap gap-x-3 gap-y-1 font-mono text-2xs text-fg-3">
              {evidence.supportingCard.meta.map((item) => (
                <span key={item}>{item}</span>
              ))}
            </p>
          </article>

          <article className="rounded-md border border-border border-l-[3px] border-l-ev-gap bg-surface px-3.5 py-3">
            <div className="mb-1.5 flex flex-wrap items-center justify-between gap-2">
              <h3 className="text-sm font-semibold">
                {evidence.gapCard.title}
              </h3>
              <span className="inline-flex items-center rounded-pill bg-neutral-soft px-2.5 py-1 text-2xs font-semibold text-neutral-fg">
                {evidence.gapCard.band}
              </span>
            </div>
            <blockquote className="my-1.5 border-l-2 border-border-strong pl-2.5 text-sm leading-relaxed text-fg-2 italic">
              {evidence.gapCard.quote}
            </blockquote>
            <p className="mt-1.5 text-sm leading-normal text-fg-2">
              {evidence.gapCard.note}
            </p>
            <p className="mt-2.5 flex flex-wrap gap-x-3 gap-y-1 font-mono text-2xs text-fg-3">
              {evidence.gapCard.meta.map((item) => (
                <span key={item}>{item}</span>
              ))}
            </p>
          </article>

          <article className="rounded-lg border border-border bg-surface-2 p-5">
            <h3 className="mb-3 text-sm font-semibold">
              {evidence.legendHeading}
            </h3>
            <ul className="flex flex-col gap-2 text-xs text-fg-2">
              {evidence.legend.map((entry) => (
                <li key={entry.kind} className="flex items-center gap-2">
                  <span
                    aria-hidden="true"
                    className={
                      "size-3 shrink-0 rounded-[3px] border-b-2 " +
                      evidenceTone[entry.kind]
                    }
                  />
                  {entry.text}
                </li>
              ))}
            </ul>
          </article>

          <article className="rounded-lg border border-border bg-surface p-5">
            <h3 className="mb-3.5 text-sm font-semibold">
              {evidence.signalHeading}
            </h3>
            <ul className="flex flex-col gap-5">
              {evidence.competencies.map((competency) => (
                <li key={competency.name} className="flex items-center gap-3">
                  <span className="min-w-0 flex-1 text-xs font-semibold">
                    {competency.name}
                    <span className="mt-0.5 block font-normal text-fg-3">
                      {competency.detail}
                    </span>
                  </span>
                  {/*
                    The range sits in its own lane below the bar rather than
                    over the fill. Drawn on top, any marker light enough to read
                    against the track hides the value it is qualifying.
                  */}
                  <span
                    aria-hidden="true"
                    className="relative w-[38%] shrink-0"
                  >
                    <span className="relative block h-2.5 rounded-pill bg-surface-3">
                      <span
                        className={
                          "absolute inset-y-0 left-0 rounded-pill " +
                          (competency.score === null
                            ? "bg-score-insufficient"
                            : "bg-score-solid")
                        }
                        style={{ width: `${competency.fill}%` }}
                      />
                    </span>
                    {competency.intervalWidth === 0 ? null : (
                      <span
                        className="absolute top-full mt-1 flex h-1.5 items-center border-x-2 border-fg-muted"
                        style={{
                          left: `${competency.intervalStart}%`,
                          width: `${competency.intervalWidth}%`,
                        }}
                      >
                        <span className="h-0.5 w-full rounded-pill bg-fg-muted" />
                      </span>
                    )}
                  </span>
                  <span className="w-7 shrink-0 text-right text-sm font-bold tabular-nums">
                    {competency.score === null ? (
                      <>
                        <span aria-hidden="true">-</span>
                        <span className="sr-only">
                          No score: insufficient evidence
                        </span>
                      </>
                    ) : (
                      competency.score
                    )}
                  </span>
                </li>
              ))}
            </ul>
            <p className="sr-only">{evidence.chartSummary}</p>
          </article>
        </div>
      </div>
    </section>
  );
}
