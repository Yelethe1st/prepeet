import type { components } from "@contracts";

import { apiFetch } from "@/lib/api/client";

/** The outcome screen's reads, typed from the contract. */

export type EvaluationResult = components["schemas"]["EvaluationResultView"];
export type CompetencyResult = components["schemas"]["CompetencyResultView"];
export type EvidenceSpan = components["schemas"]["EvidenceSpanView"];
export type Contradiction = components["schemas"]["ContradictionView"];
export type TranscriptView = components["schemas"]["TranscriptView"];
export type TranscriptSegment = components["schemas"]["TranscriptSegment"];
export type InterviewSession = components["schemas"]["InterviewSession"];

/**
 * The stored evaluation. While evaluation runs the server answers 409
 * RESULT_NOT_READY, which the screen renders as its processing state and
 * polls; that state is the honest one, never a spinner pretending.
 */
export async function getResults(sessionId: string): Promise<EvaluationResult> {
  return apiFetch<EvaluationResult>(`/interviews/${sessionId}/results`);
}

/** The assembled transcript, provenance included. */
export async function getTranscript(
  sessionId: string,
): Promise<TranscriptView> {
  return apiFetch<TranscriptView>(`/interviews/${sessionId}/transcript`);
}

/** The session, for the header's names and dates. */
export async function getInterview(
  sessionId: string,
): Promise<InterviewSession> {
  return apiFetch<InterviewSession>(`/interviews/${sessionId}`);
}
