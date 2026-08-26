import type { components } from "@contracts";

import { apiFetch } from "@/lib/api/client";

/** The live shell's exit: read the cursor, seal at it. */

export type InterviewSession = components["schemas"]["InterviewSession"];
export type CompletionReceipt = components["schemas"]["CompletionReceipt"];

/** The session, for the timeline cursor completion seals at. */
export async function getInterview(
  sessionId: string,
): Promise<InterviewSession> {
  return apiFetch<InterviewSession>(`/interviews/${sessionId}`);
}

/** Seal the session. Idempotent to the receipt. */
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
