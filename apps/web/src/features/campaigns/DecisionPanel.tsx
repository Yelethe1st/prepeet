"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { ApiError } from "@/lib/api/client";
import { Button } from "@/shared/components";

import {
  fetchDecisions,
  recordDecision,
  type DecisionRequest,
  type ReviewDecision,
} from "./api";

/**
 * The decision panel: REV-03's screen half, below the evidence it is about.
 *
 * A decision is the reviewer's own: advance, hold or decline, with a
 * required reason, recorded under the session's user - the form has no
 * decider field because the contract has none. Where the reviewer
 * disagrees with an assessed band, the disagreement travels with a
 * required rationale, and the band disagreed with is captured server-side
 * from the stored result. The history above the form is append-only:
 * changing a decision is a new decision, and every earlier one stays,
 * with its true actor - which is exactly what an appeal reads.
 */

const DECISION_WORDS: Record<string, string> = {
  advance: "Advance",
  hold: "Hold",
  reject: "Decline",
};

export function DecisionPanel({
  campaignId,
  sessionId,
  assessed,
}: {
  campaignId: string;
  sessionId: string;
  /** The assessed competencies and their bands, for override affordances. */
  assessed: { competencyID: string; band: string }[];
}) {
  const client = useQueryClient();
  const [decision, setDecision] = useState<DecisionRequest["decision"] | "">(
    "",
  );
  const [reason, setReason] = useState("");
  const [overrides, setOverrides] = useState<
    Record<string, { band: string; rationale: string }>
  >({});

  const history = useQuery({
    queryKey: ["decisions", campaignId, sessionId],
    queryFn: () => fetchDecisions(campaignId, sessionId),
  });

  const record = useMutation({
    mutationFn: (request: DecisionRequest) =>
      recordDecision(campaignId, sessionId, request),
    onSuccess: () => {
      setDecision("");
      setReason("");
      setOverrides({});
      void client.invalidateQueries({
        queryKey: ["decisions", campaignId, sessionId],
      });
      void client.invalidateQueries({ queryKey: ["roster", campaignId] });
    },
  });

  const submit = (): void => {
    if (decision === "") {
      return;
    }
    const stated = Object.entries(overrides)
      .filter(
        ([, entry]) =>
          entry.band.trim() !== "" || entry.rationale.trim() !== "",
      )
      .map(([competencyID, entry]) => ({
        competency_id: competencyID,
        band: entry.band,
        rationale: entry.rationale,
      }));
    record.mutate({
      decision,
      reason,
      ...(stated.length > 0 ? { overrides: stated } : {}),
    });
  };

  return (
    <section aria-labelledby="decision-h" className="space-y-4">
      <h2 id="decision-h" className="text-base font-semibold">
        Your decision
      </h2>

      {history.data && history.data.length > 0 ? (
        <div className="space-y-2">
          <p className="text-sm text-fg-3">
            Every decision recorded on this screening, oldest first. The history
            is append-only; changing a decision adds to it.
          </p>
          <ol className="space-y-2">
            {history.data.map((decided) => (
              <li
                key={decided.id}
                className="rounded-md border border-border p-3 text-sm"
              >
                <p>
                  <span className="font-semibold">
                    {DECISION_WORDS[decided.decision] ?? decided.decision}
                  </span>{" "}
                  <span className="text-fg-3">
                    · {day(decided.decided_at)} · decided by{" "}
                    <span className="font-mono text-xs">
                      {decided.decided_by}
                    </span>
                  </span>
                </p>
                <p className="mt-1 text-fg-2">{decided.reason}</p>
                {decided.overrides.map((override) => (
                  <p key={override.competency_id} className="mt-1 text-fg-3">
                    Read {override.competency_id} as {override.override_band}{" "}
                    where the evaluation assessed {override.recorded_band}:{" "}
                    {override.rationale}
                  </p>
                ))}
              </li>
            ))}
          </ol>
        </div>
      ) : null}

      <fieldset className="space-y-2">
        <legend className="text-sm font-semibold">Outcome</legend>
        <div className="flex flex-wrap gap-4 text-sm">
          {(["advance", "hold", "reject"] as const).map((choice) => (
            <label key={choice} className="flex items-center gap-2">
              <input
                type="radio"
                name="decision"
                value={choice}
                checked={decision === choice}
                onChange={() => setDecision(choice)}
              />
              {DECISION_WORDS[choice]}
            </label>
          ))}
        </div>
      </fieldset>

      <label className="block text-sm">
        <span className="font-semibold">Reason</span>
        <span className="text-fg-3">
          {" "}
          · required. What you write is part of the record an appeal reads.
        </span>
        <textarea
          className="mt-1 w-full rounded-md border border-border-strong bg-surface p-2"
          rows={3}
          value={reason}
          onChange={(event) => setReason(event.target.value)}
        />
      </label>

      {assessed.length > 0 ? (
        <details className="text-sm">
          <summary className="cursor-pointer font-semibold">
            Disagree with an assessed band
          </summary>
          <p className="mt-1 text-fg-3">
            A disagreement needs your reading and why. The band you disagree
            with is recorded from the evaluation itself.
          </p>
          <div className="mt-2 space-y-3">
            {assessed.map((competency) => (
              <div key={competency.competencyID} className="space-y-1">
                <p>
                  {competency.competencyID}
                  <span className="text-fg-3">
                    {" "}
                    · assessed {competency.band}
                  </span>
                </p>
                <input
                  aria-label={`Your band for ${competency.competencyID}`}
                  className="w-full rounded-md border border-border-strong bg-surface p-2"
                  placeholder="Your band"
                  value={overrides[competency.competencyID]?.band ?? ""}
                  onChange={(event) =>
                    setOverrides((current) => ({
                      ...current,
                      [competency.competencyID]: {
                        band: event.target.value,
                        rationale:
                          current[competency.competencyID]?.rationale ?? "",
                      },
                    }))
                  }
                />
                <textarea
                  aria-label={`Why you disagree on ${competency.competencyID}`}
                  className="w-full rounded-md border border-border-strong bg-surface p-2"
                  placeholder="Why"
                  rows={2}
                  value={overrides[competency.competencyID]?.rationale ?? ""}
                  onChange={(event) =>
                    setOverrides((current) => ({
                      ...current,
                      [competency.competencyID]: {
                        band: current[competency.competencyID]?.band ?? "",
                        rationale: event.target.value,
                      },
                    }))
                  }
                />
              </div>
            ))}
          </div>
        </details>
      ) : null}

      {record.isError ? (
        <p role="alert" className="text-sm font-semibold text-danger">
          {record.error instanceof ApiError
            ? record.error.message
            : "The decision was not recorded. Nothing was lost; try again."}
        </p>
      ) : null}

      <Button
        type="button"
        size="lg"
        disabled={decision === "" || reason.trim() === ""}
        busy={record.isPending}
        onClick={submit}
      >
        Record decision
      </Button>
      <p className="text-sm text-fg-3">
        Recorded under your own name, with the evidence version above. This is
        your decision; Prepeet made no part of it.
      </p>
    </section>
  );
}

function day(timestamp: string): string {
  return new Date(timestamp).toLocaleDateString("en-GB", {
    day: "numeric",
    month: "short",
    year: "numeric",
  });
}
