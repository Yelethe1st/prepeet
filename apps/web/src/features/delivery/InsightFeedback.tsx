"use client";

import { ThumbsDown, ThumbsUp } from "lucide-react";

import { Icon } from "@/shared/components";

import type { InsightFeedback as Verdict } from "./api";

/**
 * Two thumbs on one generated insight: ART-09.
 *
 * The one person who knows whether "your opening establishes a clear,
 * defensible position" is true of their own answer is the person who gave it,
 * and nothing in the product carried that back to the people maintaining the
 * coaching.
 *
 * Nothing is asked for. There is no prompt, no modal, no free-text box and no
 * follow-up if it is ignored, which is the ticket's fifth criterion and the
 * difference between a signal and a survey. A verdict is also a report about
 * the coaching rather than a way to edit it: pressing either thumb changes
 * nothing the candidate is shown.
 *
 * "Not helpful" means "this did not describe me". It does not mean "this was
 * harsh", and nothing here treats it as a request to soften anything.
 */
export function InsightFeedback({
  kind,
  insightKey,
  dimension,
  given,
  onVerdict,
}: {
  kind: Verdict["insight_kind"];
  insightKey: string;
  dimension?: string;
  /** What this candidate already said, so the pressed thumb survives a reload. */
  given?: boolean;
  onVerdict: (verdict: Verdict) => void;
}) {
  const label = dimension ?? insightKey;

  return (
    <div className="mt-3 flex items-center gap-1">
      {/*
        A group label rather than a question. "Was this helpful?" is a prompt,
        and the ticket asks for none: the controls are here for somebody who
        wants them and say nothing to anybody who does not.
      */}
      <span className="mr-1 text-2xs text-fg-3">Did this describe you?</span>
      {[true, false].map((helpful) => (
        <button
          key={String(helpful)}
          type="button"
          aria-pressed={given === helpful}
          aria-label={
            helpful
              ? `Yes, this described me: ${label}`
              : `No, this did not describe me: ${label}`
          }
          onClick={() =>
            onVerdict({
              insight_kind: kind,
              insight_key: insightKey,
              ...(dimension === undefined ? {} : { dimension }),
              helpful,
            })
          }
          className={
            "inline-flex size-7 items-center justify-center rounded-sm transition-colors " +
            (given === helpful
              ? "bg-surface-3 text-fg"
              : "text-fg-3 hover:bg-surface-3 hover:text-fg")
          }
        >
          <Icon glyph={helpful ? ThumbsUp : ThumbsDown} size="sm" />
        </button>
      ))}
    </div>
  );
}
