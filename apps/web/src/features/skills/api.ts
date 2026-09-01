import type { components } from "@contracts";

import { apiFetch } from "@/lib/api/client";

/** The candidate's own competency history, as the contract describes it. */

export type SkillHistory = components["schemas"]["SkillHistory"];
export type SkillStanding = components["schemas"]["SkillStanding"];
export type SkillEvidence = components["schemas"]["SkillEvidence"];
export type ReadinessByRole = components["schemas"]["ReadinessByRole"];
export type RoleReadiness = components["schemas"]["RoleReadiness"];

/** Every competency with the readings behind it, newest first. */
export async function getSkills(): Promise<SkillHistory> {
  return apiFetch<SkillHistory>("/me/progression/skills");
}

/** One readiness per role, each naming the standard that produced it. */
export async function getReadiness(): Promise<ReadinessByRole> {
  return apiFetch<ReadinessByRole>("/me/progression/readiness");
}
