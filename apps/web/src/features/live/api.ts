import type { components } from "@contracts";

import { apiFetch } from "@/lib/api/client";
import type { AckView, WireEvent } from "@/lib/rtc/timeline";

/**
 * The live surface's server calls: the cursor to seal at, the control
 * events the timeline sends, and RTC-03's resume.
 */

export type InterviewSession = components["schemas"]["InterviewSession"];
export type CompletionReceipt = components["schemas"]["CompletionReceipt"];
export type ResumedInterview = components["schemas"]["ResumedInterview"];
type ControlAcknowledgment = components["schemas"]["ControlAcknowledgment"];

/** The session, for the timeline cursor completion seals at. */
export async function getInterview(
  sessionId: string,
): Promise<InterviewSession> {
  return apiFetch<InterviewSession>(`/interviews/${sessionId}`);
}

/**
 * Resume a dropped session into a fresh connection attempt. Refusals carry
 * their codes on the ApiError: SESSION_NOT_RESUMABLE, GRACE_EXPIRED,
 * EPOCH_STALE.
 */
export async function resumeInterview(
  sessionId: string,
): Promise<ResumedInterview> {
  return apiFetch<ResumedInterview>(`/interviews/${sessionId}/resume`, {
    method: "POST",
  });
}

/**
 * Send one control event batch for the epoch, in the exact shape the
 * timeline buffers. The acknowledgment comes back narrowed to what the
 * buffer settles against.
 */
export async function sendEvents(
  sessionId: string,
  epoch: number,
  events: WireEvent[],
): Promise<AckView> {
  const ack = await apiFetch<ControlAcknowledgment>(
    `/interviews/${sessionId}/events`,
    {
      method: "POST",
      body: { connection_epoch: epoch, events },
    },
  );
  return {
    accepted_sequence: ack.accepted_sequence,
    outcomes: ack.outcomes,
  };
}

/** Seal the session. Idempotent to the receipt. */
export async function completeInterview(
  sessionId: string,
  connectionEpoch: number,
  finalSequence: number,
): Promise<CompletionReceipt> {
  return apiFetch<CompletionReceipt>(`/interviews/${sessionId}/complete`, {
    method: "POST",
    body: {
      connection_epoch: connectionEpoch,
      final_sequence: finalSequence,
    },
  });
}
