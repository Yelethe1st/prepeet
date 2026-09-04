"use client";

import { useQuery } from "@tanstack/react-query";

import { ApiError } from "@/lib/api/client";
import { Button, TextLink } from "@/shared/components";
import {
  EmptyState,
  ErrorState,
  LoadingSurface,
  SkeletonText,
} from "@/shared/states";

import { listCampaigns } from "./api";

/**
 * The campaign list: the doorway to REV-01's roster. Every campaign in the
 * workspace with its status in words, newest first as the server answers
 * them; what a recruiter may do inside one is decided per campaign by the
 * join, so this list offers only the way in.
 */
export function CampaignsScreen() {
  const campaigns = useQuery({
    queryKey: ["campaigns"],
    queryFn: listCampaigns,
  });

  if (campaigns.isPending) {
    return (
      <LoadingSurface label="the campaigns in this workspace">
        <SkeletonText />
        <SkeletonText width="75" />
      </LoadingSurface>
    );
  }
  if (campaigns.isError) {
    const failure = campaigns.error;
    return (
      <ErrorState
        what="The campaigns could not be loaded"
        safe="The campaigns themselves are unaffected; only this view failed."
        reference={
          failure instanceof ApiError && failure.requestId
            ? failure.requestId
            : "none"
        }
        action={
          <Button type="button" onClick={() => void campaigns.refetch()}>
            Retry
          </Button>
        }
      />
    );
  }
  if (campaigns.data.length === 0) {
    return (
      <EmptyState title="No campaigns yet" action={null}>
        A campaign fixes one interview configuration and invites candidates to
        sit it. When one exists, it appears here.
      </EmptyState>
    );
  }

  return (
    <div className="overflow-x-auto rounded-lg border border-border">
      <table className="w-full text-left text-sm">
        <caption className="sr-only">
          The campaigns in this workspace, with status and jurisdiction.
        </caption>
        <thead className="border-b border-border text-xs text-fg-3">
          <tr>
            <th scope="col" className="px-4 py-3">
              Campaign
            </th>
            <th scope="col" className="px-4 py-3">
              Role
            </th>
            <th scope="col" className="px-4 py-3">
              Status
            </th>
            <th scope="col" className="px-4 py-3">
              Jurisdiction
            </th>
            <th scope="col" className="px-4 py-3">
              <span className="sr-only">Candidates</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {campaigns.data.map((campaign) => (
            <tr
              key={campaign.id}
              className="border-b border-border last:border-0"
            >
              <td className="px-4 py-3 font-semibold">{campaign.name}</td>
              <td className="px-4 py-3 text-fg-2">{campaign.role_reference}</td>
              <td className="px-4 py-3 text-fg-2">{campaign.status}</td>
              <td className="px-4 py-3 text-fg-2">{campaign.jurisdiction}</td>
              <td className="px-4 py-3">
                <TextLink href={`/campaigns/${campaign.id}`}>
                  Candidates
                </TextLink>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
