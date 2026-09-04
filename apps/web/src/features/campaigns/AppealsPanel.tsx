"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { ApiError } from "@/lib/api/client";
import { Button } from "@/shared/components";

import {
  assignAppeal,
  fetchAppeals,
  raiseAppeal,
  resolveAppeal,
  type ReReview,
} from "./api";

/**
 * The re-review panel: REV-06's screen half. An appeal is raised against
 * the latest decision with a required reason and freezes, at that moment,
 * exactly what the decision was informed by. It is answered by somebody
 * other than the original reviewer - the server refuses the alternative by
 * name and by schema - and answered once, whole: outcome, rationale, and
 * the disclosure the candidate is permitted, which REV-07's candidate
 * surface delivers. A revised outcome is a new decision through the
 * append-only decision panel above, never an edit of anything.
 */

export function AppealsPanel({
  campaignId,
  sessionId,
}: {
  campaignId: string;
  sessionId: string;
}) {
  const client = useQueryClient();
  const [reason, setReason] = useState("");

  const appeals = useQuery({
    queryKey: ["appeals", campaignId, sessionId],
    queryFn: () => fetchAppeals(campaignId, sessionId),
  });

  const refresh = (): void => {
    void client.invalidateQueries({
      queryKey: ["appeals", campaignId, sessionId],
    });
  };

  const raise = useMutation({
    mutationFn: () => raiseAppeal(campaignId, sessionId, reason),
    onSuccess: () => {
      setReason("");
      refresh();
    },
  });

  return (
    <section aria-labelledby="appeals-h" className="space-y-4">
      <h2 id="appeals-h" className="text-base font-semibold">
        Re-review
      </h2>

      {appeals.data && appeals.data.length > 0 ? (
        <ol className="space-y-3">
          {appeals.data.map((appeal) => (
            <AppealCard
              key={appeal.id}
              campaignId={campaignId}
              appeal={appeal}
              onChanged={refresh}
            />
          ))}
        </ol>
      ) : (
        <p className="text-sm text-fg-3">
          No re-review has been raised on this screening.
        </p>
      )}

      <div className="space-y-2">
        <label className="block text-sm">
          <span className="font-semibold">Raise a re-review</span>
          <span className="text-fg-3">
            {" "}
            · appeals the latest decision and freezes the evidence it read, as
            it stands right now.
          </span>
          <textarea
            className="mt-1 w-full rounded-md border border-border-strong bg-surface p-2"
            rows={2}
            value={reason}
            onChange={(event) => setReason(event.target.value)}
          />
        </label>
        {raise.isError ? (
          <p role="alert" className="text-sm font-semibold text-danger">
            {raise.error instanceof ApiError
              ? raise.error.message
              : "The re-review was not raised. Nothing was lost; try again."}
          </p>
        ) : null}
        <Button
          type="button"
          variant="secondary"
          disabled={reason.trim() === ""}
          busy={raise.isPending}
          onClick={() => raise.mutate()}
        >
          Raise re-review
        </Button>
      </div>
    </section>
  );
}

function AppealCard({
  campaignId,
  appeal,
  onChanged,
}: {
  campaignId: string;
  appeal: ReReview;
  onChanged: () => void;
}) {
  const [assignee, setAssignee] = useState("");
  const [outcome, setOutcome] = useState<"upheld" | "revised" | "">("");
  const [rationale, setRationale] = useState("");
  const [disclosure, setDisclosure] = useState("");

  const assign = useMutation({
    mutationFn: () => assignAppeal(campaignId, appeal.id, assignee),
    onSuccess: onChanged,
  });
  const resolve = useMutation({
    mutationFn: () => {
      if (outcome === "") {
        return Promise.reject(new Error("no outcome"));
      }
      return resolveAppeal(campaignId, appeal.id, {
        outcome,
        rationale,
        disclosure,
      });
    },
    onSuccess: onChanged,
  });

  const failure = assign.error ?? resolve.error;

  return (
    <li className="space-y-2 rounded-md border border-border p-3 text-sm">
      <p>
        <span className="font-semibold">
          {appeal.resolution
            ? appeal.resolution.outcome === "upheld"
              ? "Resolved: outcome upheld"
              : "Resolved: outcome revised"
            : appeal.assigned_to
              ? "Awaiting the assigned reviewer"
              : "Awaiting assignment"}
        </span>{" "}
        <span className="text-fg-3">
          · raised {day(appeal.raised_at)} · answer due {day(appeal.due_at)}
        </span>
      </p>
      <p className="text-fg-2">{appeal.reason}</p>
      <p className="font-mono text-xs text-fg-3">
        Frozen at raise: result {appeal.frozen.result_digest} · rubric{" "}
        {appeal.frozen.rubric_digest} · bundle {appeal.frozen.bundle_digest}
      </p>
      <p className="text-fg-3">
        The original reviewer cannot answer this appeal.
      </p>

      {appeal.resolution ? (
        <div className="rounded-md bg-surface-2 p-2">
          <p className="text-fg-2">{appeal.resolution.rationale}</p>
          <p className="mt-1 text-fg-3">
            Candidate disclosure, recorded for delivery:{" "}
            {appeal.resolution.candidate_disclosure}
          </p>
        </div>
      ) : (
        <div className="space-y-2">
          {!appeal.assigned_to ? (
            <div className="flex flex-wrap items-center gap-2">
              <input
                aria-label="Assign to"
                className="min-w-64 rounded-md border border-border-strong bg-surface p-2"
                placeholder="Reviewer's user id"
                value={assignee}
                onChange={(event) => setAssignee(event.target.value)}
              />
              <Button
                type="button"
                size="sm"
                variant="secondary"
                disabled={assignee.trim() === ""}
                busy={assign.isPending}
                onClick={() => assign.mutate()}
              >
                Assign
              </Button>
            </div>
          ) : null}

          <div className="space-y-1">
            <label className="flex items-center gap-2">
              <span className="font-semibold">Outcome</span>
              <select
                className="rounded-md border border-border-strong bg-surface px-2 py-1.5"
                value={outcome}
                onChange={(event) =>
                  setOutcome(event.target.value as "upheld" | "revised" | "")
                }
              >
                <option value="">Choose</option>
                <option value="upheld">Upheld</option>
                <option value="revised">Revised</option>
              </select>
            </label>
            <textarea
              aria-label="Resolution rationale"
              className="w-full rounded-md border border-border-strong bg-surface p-2"
              placeholder="Rationale"
              rows={2}
              value={rationale}
              onChange={(event) => setRationale(event.target.value)}
            />
            <textarea
              aria-label="Candidate disclosure"
              className="w-full rounded-md border border-border-strong bg-surface p-2"
              placeholder="What the candidate is permitted to be told"
              rows={2}
              value={disclosure}
              onChange={(event) => setDisclosure(event.target.value)}
            />
            <Button
              type="button"
              size="sm"
              disabled={
                outcome === "" ||
                rationale.trim() === "" ||
                disclosure.trim() === ""
              }
              busy={resolve.isPending}
              onClick={() => resolve.mutate()}
            >
              Resolve
            </Button>
            <p className="text-fg-3">
              A revised outcome is recorded as a new decision above, never an
              edit.
            </p>
          </div>
        </div>
      )}

      {failure ? (
        <p role="alert" className="text-sm font-semibold text-danger">
          {failure instanceof ApiError
            ? failure.message
            : "That did not go through. Nothing was lost; try again."}
        </p>
      ) : null}
    </li>
  );
}

function day(timestamp: string): string {
  return new Date(timestamp).toLocaleDateString("en-GB", {
    day: "numeric",
    month: "short",
    year: "numeric",
  });
}
