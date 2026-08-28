import { ArrowRight, CirclePlay } from "lucide-react";

import { ButtonLink } from "@/shared/components";
import { Icon } from "@/shared/components/Icon";

import { realtime } from "./content";

/**
 * The live conversation, ported from the sixth section.
 *
 * The transcript beside the copy is the argument: the follow-up at 05:16 exists
 * because two answers disagreed, and the note under it says so in as many words.
 * That is the section's claim and the evidence for it in the same picture.
 *
 * The transcript is real text rather than a picture, because it is prose worth
 * reading and worth finding. It is a definition list of who said what, which is
 * what a transcript is.
 */
export function Realtime() {
  return (
    <section
      id="realtime"
      aria-labelledby="realtime-h"
      className="mx-auto max-w-[1180px] px-5 py-16 md:px-6 md:py-24"
    >
      <div className="grid grid-cols-1 items-center gap-7 lg:grid-cols-2 lg:gap-12">
        <div>
          <p className="text-2xs font-bold tracking-[0.1em] text-primary uppercase">
            {realtime.eyebrow}
          </p>
          <h2
            id="realtime-h"
            className="mt-2.5 font-display text-[clamp(1.6rem,2.8vw,2.25rem)] leading-[1.15] font-medium tracking-[-0.02em]"
          >
            {realtime.heading}
          </h2>
          <p className="mt-3.5 leading-relaxed text-fg-2">{realtime.lead}</p>

          <ul className="mt-4.5 flex flex-col gap-2.5">
            {realtime.points.map((point) => (
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
            <ButtonLink href="/register">
              <Icon glyph={CirclePlay} size="sm" />
              Try a practice interview
            </ButtonLink>
            <ButtonLink href="#evidence" variant="ghost">
              See how the evidence is built
              <Icon glyph={ArrowRight} size="sm" />
            </ButtonLink>
          </div>
        </div>

        <div className="overflow-hidden rounded-xl border border-border bg-surface">
          <div className="flex items-start justify-between gap-3 border-b border-border px-5 py-4">
            <div>
              <h3 className="text-base font-semibold">
                {realtime.transcriptTitle}
              </h3>
              <p className="mt-0.5 text-xs text-fg-3">
                {realtime.transcriptMeta}
              </p>
            </div>
            <span className="inline-flex shrink-0 items-center rounded-pill bg-primary-soft px-2.5 py-1 font-mono text-2xs tracking-[0.08em] text-primary-soft-fg uppercase">
              Practice
            </span>
          </div>

          <ol className="flex flex-col gap-4 px-5 py-4">
            {realtime.turns.map((turn) => (
              <li
                key={turn.seconds}
                className={
                  "flex items-start gap-2.5 rounded-md " +
                  (turn.current === true ? "bg-warning-soft p-2.5" : "")
                }
              >
                <span
                  aria-hidden="true"
                  className={
                    "grid size-7 shrink-0 place-items-center rounded-full text-2xs font-semibold " +
                    (turn.interviewer
                      ? "bg-primary-soft text-primary-soft-fg"
                      : "bg-surface-3 text-fg-2")
                  }
                >
                  {turn.initials}
                </span>
                <div className="min-w-0">
                  <p className="flex flex-wrap items-center gap-2 text-2xs text-fg-3">
                    <span className="font-semibold text-fg-2">
                      {turn.speaker}
                    </span>
                    <time dateTime={turn.seconds}>{turn.at}</time>
                    {turn.note === undefined ? null : (
                      <span
                        className={
                          "inline-flex items-center rounded-pill px-2 py-0.5 " +
                          (turn.noteTone === "warning"
                            ? "bg-warning-soft text-warning-fg"
                            : "border border-border-strong text-fg-2")
                        }
                      >
                        {turn.note}
                      </span>
                    )}
                  </p>
                  <p className="mt-1 text-sm leading-normal text-fg-2">
                    {turn.text}
                  </p>
                </div>
              </li>
            ))}
          </ol>

          <p className="border-t border-border px-5 py-4 text-xs leading-normal text-fg-3">
            {realtime.transcriptFootnote}
          </p>
        </div>
      </div>
    </section>
  );
}
