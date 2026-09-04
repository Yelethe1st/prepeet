"use client";

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { ApiError } from "@/lib/api/client";
import { Button } from "@/shared/components";
import {
  EmptyState,
  ErrorState,
  LoadingSurface,
  SkeletonText,
} from "@/shared/states";

import { fetchRoster, type RosterStanding } from "./api";

/**
 * The candidate roster: REV-01, from the prototype's admin-recruiter
 * screen, with the ticket's own rule where the two disagree. The prototype
 * offered sorting by competency signal; the ticket forbids any ordering by
 * quality, so rows keep the server's order (invitation recency), no column
 * sorts by anything evaluative, and a candidate whose record could not
 * support assessment carries that as their standing, in words, in place -
 * never a low scorer at the bottom of a list.
 *
 * The standing filter is the server's: choosing one refetches, and the
 * pending-review count stays the campaign's truth under any filter.
 */

/** The closed vocabulary, in words a reviewer reads rather than codes. */
const STANDINGS: Record<RosterStanding, string> = {
  invited: "Invited",
  expired: "Invitation expired",
  declined: "Declined",
  revoked: "Revoked",
  accepted: "Accepted, not started",
  in_progress: "Interview in progress",
  processing: "Processing",
  session_expired: "Session expired",
  awaiting_review: "Awaiting review",
  insufficient_evidence: "Insufficient evidence, awaiting review",
};

export function RosterScreen({ campaignId }: { campaignId: string }) {
  const [standing, setStanding] = useState<RosterStanding | "">("");
  const roster = useQuery({
    queryKey: ["roster", campaignId, standing],
    queryFn: () =>
      fetchRoster(campaignId, standing === "" ? undefined : standing),
  });

  if (roster.isPending) {
    return (
      <LoadingSurface label="the candidate roster">
        <SkeletonText />
        <SkeletonText width="75" />
        <SkeletonText width="50" />
      </LoadingSurface>
    );
  }
  if (roster.isError) {
    const failure = roster.error;
    return (
      <ErrorState
        what="The roster could not be loaded"
        safe="The candidates and their screenings are unaffected; only this view failed."
        reference={
          failure instanceof ApiError && failure.requestId
            ? failure.requestId
            : "none"
        }
        action={
          <Button type="button" onClick={() => void roster.refetch()}>
            Retry
          </Button>
        }
      />
    );
  }

  const { candidates, pending_review: pending } = roster.data;

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p role="status" className="text-sm text-fg-2">
          {pending === 0
            ? "Nothing awaits review."
            : `${pending} completed screening${pending === 1 ? "" : "s"} await${pending === 1 ? "s" : ""} a human review, the insufficient-evidence ones included.`}
        </p>
        <label className="flex items-center gap-2 text-sm">
          <span className="text-fg-2">Standing</span>
          <select
            className="rounded-md border border-border-strong bg-surface px-2 py-1.5"
            value={standing}
            onChange={(event) =>
              setStanding(event.target.value as RosterStanding | "")
            }
          >
            <option value="">All</option>
            {Object.entries(STANDINGS).map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
        </label>
      </div>

      {candidates.length === 0 ? (
        <EmptyState title="Nobody at this standing" action={null}>
          {standing === ""
            ? "No candidates have been invited to this campaign yet. Invitations create the roster."
            : "No candidate currently stands there. The filter runs on the server, so this is the campaign's truth, not this page's."}
        </EmptyState>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-border">
          <table className="w-full text-left text-sm">
            <caption className="sr-only">
              Candidates invited to this campaign with where each stands, in
              invitation order. This is not a ranking, and nothing here orders
              candidates by quality.
            </caption>
            <thead className="border-b border-border text-xs text-fg-3">
              <tr>
                <th scope="col" className="px-4 py-3">
                  Candidate
                </th>
                <th scope="col" className="px-4 py-3">
                  Invited
                </th>
                <th scope="col" className="px-4 py-3">
                  Submitted
                </th>
                <th scope="col" className="px-4 py-3">
                  Standing
                </th>
              </tr>
            </thead>
            <tbody>
              {candidates.map((entry) => (
                <tr
                  key={entry.invitation_id}
                  className="border-b border-border last:border-0"
                >
                  <td className="px-4 py-3 font-semibold">{entry.recipient}</td>
                  <td className="px-4 py-3 text-fg-2">
                    {day(entry.invited_at)}
                  </td>
                  <td className="px-4 py-3 text-fg-2">
                    {entry.submitted_at ? day(entry.submitted_at) : "–"}
                  </td>
                  <td className="px-4 py-3 text-fg-2">
                    {STANDINGS[entry.standing] ?? entry.standing}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <p className="text-sm text-fg-3">
        Each row says where one candidate stands. It is not a ranking, and
        Prepeet does not recommend who to hire: the review, and the decision,
        belong to a named person.
      </p>
    </div>
  );
}

function day(timestamp: string): string {
  return new Date(timestamp).toLocaleDateString("en-GB", {
    day: "numeric",
    month: "short",
    year: "numeric",
  });
}
