import type { components } from "@contracts";

import { apiFetch } from "@/lib/api/client";

/** The coaching review's read, typed from the contract. */

export type ReviewView = components["schemas"]["ReviewView"];
export type InterviewSession = components["schemas"]["InterviewSession"];
export type TranscriptView = components["schemas"]["TranscriptView"];
export type AnswerCoaching = components["schemas"]["AnswerCoachingView"];
export type CoachingPoint = components["schemas"]["CoachingPointView"];

/**
 * The derived coaching. RESULT_NOT_READY while evaluation runs; a
 * coaching failure arrives as coaching_available false with the
 * evaluation intact, never as an error.
 */
export async function getReview(sessionId: string): Promise<ReviewView> {
  return apiFetch<ReviewView>(`/interviews/${sessionId}/review`);
}

/** The transcript, for which answers already have a retake. */
export async function getTranscript(
  sessionId: string,
): Promise<TranscriptView> {
  return apiFetch<TranscriptView>(`/interviews/${sessionId}/transcript`);
}

/**
 * Retake one answer as a new linked practice session. One per question,
 * practice only, once results exist: the server refuses by name.
 */
export async function createRedo(
  sessionId: string,
  sequence: number,
): Promise<InterviewSession> {
  return apiFetch<InterviewSession>(
    `/interviews/${sessionId}/turns/${sequence}/redos`,
    { method: "POST" },
  );
}
