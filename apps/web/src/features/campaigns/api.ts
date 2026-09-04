import type { components } from "@contracts";

import { apiFetch } from "@/lib/api/client";

/** The recruiter surface's calls, typed from the contract. */

export type Campaign = components["schemas"]["Campaign"];
export type CampaignRoster = components["schemas"]["CampaignRoster"];
export type RosterEntry = components["schemas"]["RosterEntry"];
export type RosterStanding = components["schemas"]["RosterStanding"];

/** Every campaign in the caller's workspace. */
export async function listCampaigns(): Promise<Campaign[]> {
  const list =
    await apiFetch<components["schemas"]["CampaignList"]>("/campaigns");
  return list.campaigns;
}

/**
 * One campaign's roster, filtered by the server when a standing is named:
 * a filtered roster contains only the rows asked for, whoever renders it.
 */
export async function fetchRoster(
  campaignId: string,
  standing?: RosterStanding,
): Promise<CampaignRoster> {
  const query = standing ? `?standing=${standing}` : "";
  return apiFetch<CampaignRoster>(
    `/campaigns/${campaignId}/candidates${query}`,
  );
}

export type ScreeningReview = components["schemas"]["ScreeningReview"];

/**
 * The evidence-first review of one completed screening. Reading it is a
 * recorded event server-side; a read that cannot be recorded is refused.
 */
export async function fetchReview(
  campaignId: string,
  sessionId: string,
): Promise<ScreeningReview> {
  return apiFetch<ScreeningReview>(
    `/campaigns/${campaignId}/sessions/${sessionId}/review`,
  );
}
