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

/**
 * Replay the durable timeline after a cursor. The caption fold consumes
 * this: captions are the transcript read back, never a parallel channel
 * that could disagree with the evidence.
 */
export async function replayEvents(
  sessionId: string,
  afterEpoch: number,
  afterSequence: number,
): Promise<components["schemas"]["ControlEventList"]["events"]> {
  const list = await apiFetch<components["schemas"]["ControlEventList"]>(
    `/interviews/${sessionId}/events?after_epoch=${afterEpoch}&after_sequence=${afterSequence}`,
  );
  return list.events;
}

/**
 * What the live surface names: the persona speaking, the role and shape
 * the session was configured from. The catalogue is the resolver because
 * the session's config records ids, and only ids.
 */
export interface LiveContext {
  personas: components["schemas"]["Persona"][];
  roles: components["schemas"]["CatalogRole"][];
  shapes: components["schemas"]["InterviewShape"][];
}

export async function fetchLiveContext(): Promise<LiveContext> {
  const [personas, roles, shapes] = await Promise.all([
    apiFetch<components["schemas"]["PersonaList"]>("/catalog/personas"),
    apiFetch<components["schemas"]["RoleList"]>("/catalog/roles"),
    apiFetch<components["schemas"]["ShapeList"]>("/catalog/interview-shapes"),
  ]);
  return {
    personas: personas.personas,
    roles: roles.roles,
    shapes: shapes.shapes,
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
