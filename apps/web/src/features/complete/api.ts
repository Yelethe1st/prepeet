import type { components } from "@contracts";

import { apiFetch } from "@/lib/api/client";

/** The completion flow's reads and its one write. */

export type InterviewSession = components["schemas"]["InterviewSession"];
export type CompletionReceipt = components["schemas"]["CompletionReceipt"];

/** One session with its cursor and, once sealed, its durable receipt. */
export async function getInterview(
  sessionId: string,
): Promise<InterviewSession> {
  return apiFetch<InterviewSession>(`/interviews/${sessionId}`);
}

/**
 * Seal the session at the timeline's accepted cursor. Idempotent to the
 * receipt: completing twice with the same cursor answers the same seal.
 */
export async function completeInterview(
  sessionId: string,
  connectionEpoch: number,
  finalSequence: number,
): Promise<CompletionReceipt> {
  return apiFetch<CompletionReceipt>(`/interviews/${sessionId}/complete`, {
    method: "POST",
    body: JSON.stringify({
      connection_epoch: connectionEpoch,
      final_sequence: finalSequence,
    }),
  });
}
