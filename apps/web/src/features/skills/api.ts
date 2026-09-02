import type { components } from "@contracts";

import { apiFetch } from "@/lib/api/client";

/** The candidate's own competency history, as the contract describes it. */

export type SkillHistory = components["schemas"]["SkillHistory"];
export type SkillStanding = components["schemas"]["SkillStanding"];
export type SkillEvidence = components["schemas"]["SkillEvidence"];

/** Every competency with the readings behind it, newest first. */
export async function getSkills(): Promise<SkillHistory> {
  return apiFetch<SkillHistory>("/me/progression/skills");
}

// getReadiness is deliberately absent. The endpoint exists and is served, but
// no screen renders readiness yet, and a client function nothing calls is code
// that cannot be wrong in a way anybody notices. It arrives with the screen.
