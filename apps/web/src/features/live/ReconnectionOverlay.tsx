"use client";

import { useEffect, useRef } from "react";

import { Button } from "@/shared/components";
import type { RecoveryPhase } from "@/lib/rtc/recovery";

/**
 * The reconnection overlay, ported from the prototype's live screen
 * (candidate-session-live.html): an alertdialog over the interview while
 * the chain runs, with the attempt counter in a live region so a screen
 * reader hears each state change without the dialog re-announcing itself.
 * The copy tells the truth the spec requires: the interview is paused, the
 * timer stopped with it, and what was said is already recorded.
 *
 * Focus moves to the retry button when the overlay opens - the one
 * decision the person can act on - and returns to wherever it was when
 * recovery closes the overlay, exactly as the prototype does, so a
 * keyboard or screen-reader user is never stranded on a control that no
 * longer exists (A11Y-02).
 *
 * Only the states with a decision still open render here; the terminal
 * verdicts (expired, superseded, unresumable) replace the live surface
 * entirely, because there is no interview behind them to return to.
 */

export function ReconnectionOverlay({
  phase,
  onRetryNow,
  onEndInterview,
}: {
  phase: Extract<RecoveryPhase, { kind: "reconnecting" | "exhausted" }>;
  onRetryNow: () => void;
  onEndInterview: () => void;
}) {
  const retry = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    const before =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    retry.current?.focus();
    return () => {
      before?.focus();
    };
  }, []);

  const attempts =
    phase.kind === "reconnecting"
      ? `Reconnection attempt ${phase.attempt} of ${phase.maxAttempts}`
      : "Automatic reconnection has stopped. Retry when your connection is back.";

  return (
    <div
      className="fixed inset-0 z-[120] grid place-items-center bg-overlay p-6 backdrop-blur-[3px]"
      role="alertdialog"
      aria-modal="true"
      aria-labelledby="reconnect-h"
      aria-describedby="reconnect-d"
    >
      <div className="flex w-[min(92vw,420px)] flex-col items-center gap-3 rounded-xl border border-border bg-surface px-[22px] py-[26px] text-center shadow-lg">
        <div
          className="size-8 animate-spin rounded-full border-2 border-border border-t-primary"
          aria-hidden="true"
        />
        <h2 id="reconnect-h" className="text-md font-semibold">
          Reconnecting to the interview
        </h2>
        <p id="reconnect-d" className="text-sm leading-relaxed text-fg-2">
          The connection dropped. Your interview is paused and the timer has
          stopped. Everything you have already said is safely recorded.
        </p>
        <p role="status" className="font-mono text-xs text-fg-3">
          {attempts}
        </p>
        <div className="flex flex-wrap items-center justify-center gap-2">
          <Button ref={retry} type="button" size="sm" onClick={onRetryNow}>
            Retry now
          </Button>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={onEndInterview}
          >
            End interview
          </Button>
        </div>
      </div>
    </div>
  );
}
