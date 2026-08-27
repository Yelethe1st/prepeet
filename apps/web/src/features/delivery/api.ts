import type { components } from "@contracts";

import { apiFetch } from "@/lib/api/client";

/** The delivery screen's reads, typed from the contract. */

export type DeliveryView = components["schemas"]["DeliveryView"];
export type TranscriptView = components["schemas"]["TranscriptView"];

/** The stored analysis. DELIVERY_NOT_READY while the workflow runs. */
export async function getDelivery(sessionId: string): Promise<DeliveryView> {
  return apiFetch<DeliveryView>(`/interviews/${sessionId}/delivery`);
}

/** The transcript, for jumping observations to the moment they cite. */
export async function getTranscript(
  sessionId: string,
): Promise<TranscriptView> {
  return apiFetch<TranscriptView>(`/interviews/${sessionId}/transcript`);
}
