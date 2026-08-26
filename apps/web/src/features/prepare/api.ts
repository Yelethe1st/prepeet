import type { components } from "@contracts";

import { apiFetch } from "@/lib/api/client";

/** The prepare screen's reads, typed from the contract. */

export type InterviewSession = components["schemas"]["InterviewSession"];
export type CandidateProfile = components["schemas"]["CandidateProfile"];

/** The catalogue collections the brief names things from. */
export interface Catalogue {
  disciplines: components["schemas"]["Discipline"][];
  roles: components["schemas"]["CatalogRole"][];
  shapes: components["schemas"]["InterviewShape"][];
  personas: components["schemas"]["Persona"][];
}

/** One session, as its owner sees it. Polled while composing. */
export async function getInterview(
  sessionId: string,
): Promise<InterviewSession> {
  return apiFetch<InterviewSession>(`/interviews/${sessionId}`);
}

/**
 * The profile, for the accessibility defaults the prepare screen honours:
 * captions and extended thinking time arrive pre-set from what the person
 * already told us, which is PRO-01's promise kept here.
 */
export async function getProfile(): Promise<CandidateProfile> {
  return apiFetch<CandidateProfile>("/me/profile");
}

/**
 * The catalogue, for naming what the session's config points at. Fetched
 * here rather than borrowed from the wizard's feature: features do not
 * import each other, and the endpoints are shared surface, not feature code.
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

export type StartedInterview = components["schemas"]["StartedInterview"];

/**
 * Starts the session and returns the room grant. Each refusal arrives with
 * its own stable code (SESSION_EXPIRED, SESSION_ALREADY_STARTED,
 * SESSION_NOT_READY, QUOTA_EXHAUSTED) for the screen to answer by name.
 */
export async function startInterview(
  sessionId: string,
): Promise<StartedInterview> {
  return apiFetch<StartedInterview>(`/interviews/${sessionId}/start`, {
    method: "POST",
  });
}
