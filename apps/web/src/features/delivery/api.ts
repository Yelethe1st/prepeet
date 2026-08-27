import type { components } from "@contracts";

import { apiFetch } from "@/lib/api/client";

/** The delivery screen's reads, typed from the contract. */

export type DeliveryView = components["schemas"]["DeliveryView"];
export type DeliveryBaseline = components["schemas"]["DeliveryBaseline"];
export type InterviewSession = components["schemas"]["InterviewSession"];
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

/** The caller's own ranges, or the honest absence with its count. */
export async function getBaseline(): Promise<DeliveryBaseline> {
  return apiFetch<DeliveryBaseline>("/me/delivery-baseline");
}

/** The session, for whether it is a retake and of what. */
export async function getInterview(
  sessionId: string,
): Promise<InterviewSession> {
  return apiFetch<InterviewSession>(`/interviews/${sessionId}`);
}
