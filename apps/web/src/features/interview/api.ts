import type { components } from "@contracts";

import { apiFetch } from "@/lib/api/client";

import type { Catalogue } from "./rules";

/** The interview feature's calls, typed from the contract. */

export type CreateInterviewRequest =
  components["schemas"]["CreateInterviewRequest"];
export type InterviewSession = components["schemas"]["InterviewSession"];
export type PracticeConsent = components["schemas"]["PracticeConsent"];

/**
 * The four catalogue collections, together: the wizard needs them all
 * before any step renders, and one loading state beats four.
 */
export async function fetchCatalogue(): Promise<Catalogue> {
  const [disciplines, roles, shapes, personas] = await Promise.all([
    apiFetch<components["schemas"]["DisciplineList"]>("/catalog/disciplines"),
    apiFetch<components["schemas"]["RoleList"]>("/catalog/roles"),
    apiFetch<components["schemas"]["ShapeList"]>("/catalog/interview-shapes"),
    apiFetch<components["schemas"]["PersonaList"]>("/catalog/personas"),
  ]);
  return {
    disciplines: disciplines.disciplines,
    roles: roles.roles,
    shapes: shapes.shapes,
    personas: personas.personas,
  };
}

/** Creates the practice session. The server re-validates everything. */
export async function createInterview(
  request: CreateInterviewRequest,
): Promise<InterviewSession> {
  return apiFetch<InterviewSession>("/interviews", {
    method: "POST",
    body: request,
  });
}

/**
 * The current consent text with its version. The version is what creation
 * echoes back, so the stored agreement points at words the person was
 * actually shown; the server refuses a stale one.
 */
export async function fetchPracticeConsent(): Promise<PracticeConsent> {
  return apiFetch<PracticeConsent>("/interviews/practice-consent");
}
