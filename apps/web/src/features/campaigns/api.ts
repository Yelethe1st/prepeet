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

export type ReviewDecision = components["schemas"]["ReviewDecisionView"];
export type DecisionRequest = components["schemas"]["ReviewDecisionRequest"];

/** The decision history, oldest first: what an appeal reads. */
export async function fetchDecisions(
  campaignId: string,
  sessionId: string,
): Promise<ReviewDecision[]> {
  const list = await apiFetch<components["schemas"]["ReviewDecisionList"]>(
    `/campaigns/${campaignId}/sessions/${sessionId}/decisions`,
  );
  return list.decisions;
}

/**
 * Record a named human's decision. The decider is the session server-side;
 * the request cannot even try to supply one.
 */
export async function recordDecision(
  campaignId: string,
  sessionId: string,
  request: DecisionRequest,
): Promise<ReviewDecision> {
  return apiFetch<ReviewDecision>(
    `/campaigns/${campaignId}/sessions/${sessionId}/decisions`,
    { method: "POST", body: request },
  );
}

export type ReReview = components["schemas"]["ReReviewView"];

/** The appeals on one screening, oldest first. */
export async function fetchAppeals(
  campaignId: string,
  sessionId: string,
): Promise<ReReview[]> {
  const list = await apiFetch<components["schemas"]["ReReviewList"]>(
    `/campaigns/${campaignId}/sessions/${sessionId}/re-reviews`,
  );
  return list.re_reviews;
}

/** Raise an appeal against the latest decision. The requester is the session. */
export async function raiseAppeal(
  campaignId: string,
  sessionId: string,
  reason: string,
): Promise<ReReview> {
  return apiFetch<ReReview>(
    `/campaigns/${campaignId}/sessions/${sessionId}/re-reviews`,
    { method: "POST", body: { reason } },
  );
}

/** Seat the reviewer who answers. Never the original reviewer. */
export async function assignAppeal(
  campaignId: string,
  reReviewId: string,
  assignee: string,
): Promise<ReReview> {
  return apiFetch<ReReview>(
    `/campaigns/${campaignId}/re-reviews/${reReviewId}/assignment`,
    { method: "POST", body: { assignee } },
  );
}

/** Answer an open appeal, once, whole. The resolver is the session. */
export async function resolveAppeal(
  campaignId: string,
  reReviewId: string,
  resolution: {
    outcome: "upheld" | "revised";
    rationale: string;
    disclosure: string;
  },
): Promise<ReReview> {
  return apiFetch<ReReview>(
    `/campaigns/${campaignId}/re-reviews/${reReviewId}/resolution`,
    { method: "POST", body: resolution },
  );
}
