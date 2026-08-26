import type { components } from "@contracts";

import { apiFetch } from "@/lib/api/client";

/** The coaching review's read, typed from the contract. */

export type ReviewView = components["schemas"]["ReviewView"];
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
