import type { components } from "@contracts";

import { apiFetch } from "@/lib/api/client";

/** The session history's read. */

export type InterviewSession = components["schemas"]["InterviewSession"];

/** Every session the caller owns, newest first, every state included. */
export async function listSessions(): Promise<InterviewSession[]> {
  const body = await apiFetch<{ sessions: InterviewSession[] }>("/me/sessions");
  return body.sessions;
}
