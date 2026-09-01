"use client";

import { useState } from "react";

import type { SkillEvidence, SkillHistory, SkillStanding } from "./api";

/**
 * The candidate's competencies, and the evidence behind each one.
 *
 * PRG-04 asks for three things, and each is a rule rather than a decoration.
 * A competency expands to the readings behind it with their dates, so a
 * standing can be argued with. Stale evidence says it is stale, because a
 * reading from six months ago and one from last week are different claims and
 * a screen that renders both as "solid" has quietly promoted a memory to a
 * measurement. And the chart carries a summary and a table, because a picture
 * is not a reading for everyone.
 *
 * The rule the whole progression context turns on applies hardest here: a
 * competency nobody has been asked about is not one somebody answered badly.
 * The screen is where that difference is most easily lost, so an unassessed
 * competency has no band, no bar and no position in any ordering that implies
 * one.
 */

/** How each standing is described, in words rather than by colour alone. */
const standingLabel: Record<string, string> = {
  fresh: "Fresh",
  aging: "Ageing",
  stale: "Stale",
  none: "Not yet assessed",
};

/**
 * Why a standing matters, said once so every row does not repeat it.
 *
 * Written as what it means for the candidate rather than as what the system
 * did: "worth revisiting" is actionable, "older than 90 days" is trivia.
 */
const standingMeaning: Record<string, string> = {
  fresh: "Measured recently.",
  aging: "Still current, and worth refreshing soon.",
  stale: "Old enough that it may no longer describe how you answer now.",
  none: "No session has covered this yet.",
};

export function SkillsScreen({ history }: { history: SkillHistory }) {
  const [open, setOpen] = useState<string | null>(null);

  if (history.competencies.length === 0) {
    return (
      <section>
        <h1>Skills</h1>
        <p>
          Once you have practised, your competencies appear here with the
          evidence behind each one.
        </p>
      </section>
    );
  }

  return (
    <section>
      <h1>Skills</h1>
      <ChartSummary competencies={history.competencies} />
      <StandingsTable competencies={history.competencies} />

      <h2>Competencies</h2>
      <p>Open a row to read the evidence behind its standing.</p>
      <ul>
        {history.competencies.map((competency) => (
          <CompetencyRow
            key={competency.competency_id}
            competency={competency}
            expanded={open === competency.competency_id}
            onToggle={() =>
              setOpen(
                open === competency.competency_id
                  ? null
                  : competency.competency_id,
              )
            }
          />
        ))}
      </ul>
    </section>
  );
}

/**
 * The chart's text alternative.
 *
 * Rendered whether or not a chart is, and not hidden from anyone. A summary
 * only screen readers receive is one nobody proofreads, and it is the sighted
 * reader in a hurry who most often wants the count rather than the shape.
 */
function ChartSummary({ competencies }: { competencies: SkillStanding[] }) {
  const counts = countByStanding(competencies);
  const described = (["fresh", "aging", "stale", "none"] as const)
    .filter((standing) => counts[standing] > 0)
    .map(
      (standing) =>
        `${counts[standing]} ${(standingLabel[standing] ?? standing).toLowerCase()}`,
    )
    .join(", ");

  return <p>{described}.</p>;
}

/** The same reading as a table, for anyone the chart does not serve. */
function StandingsTable({ competencies }: { competencies: SkillStanding[] }) {
  return (
    <table aria-label="Competency standings">
      <thead>
        <tr>
          <th scope="col">Competency</th>
          <th scope="col">Standing</th>
          <th scope="col">Evidence</th>
        </tr>
      </thead>
      <tbody>
        {competencies.map((competency) => (
          <tr key={competency.competency_id}>
            <th scope="row">{competency.name}</th>
            <td>{standingLabel[competency.standing]}</td>
            {/* An em dash would be the wrong character and a zero would be the
                wrong claim, so the count of readings is given as a count. */}
            <td>{competency.evidence.length}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function CompetencyRow({
  competency,
  expanded,
  onToggle,
}: {
  competency: SkillStanding;
  expanded: boolean;
  onToggle: () => void;
}) {
  const regionId = `evidence-${competency.competency_id}`;

  return (
    <li>
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={expanded}
        aria-controls={regionId}
      >
        <span>{competency.name}</span>
        {/* The band, but only where there is one. An unassessed competency
            shows its standing instead, which says no session has covered it
            rather than implying a low answer. */}
        {competency.band !== undefined && <span>{competency.band}</span>}
        <span>{standingLabel[competency.standing]}</span>
      </button>
      {expanded && (
        <div
          id={regionId}
          role="region"
          aria-label={`${competency.name} evidence`}
        >
          <p>{standingMeaning[competency.standing]}</p>
          {competency.evidence.length === 0 ? (
            <p>No sessions have covered this yet.</p>
          ) : (
            <ol>
              {competency.evidence.map((reading) => (
                <EvidenceItem
                  key={`${reading.observed_at}-${reading.rubric_version}`}
                  reading={reading}
                />
              ))}
            </ol>
          )}
        </div>
      )}
    </li>
  );
}

/**
 * One reading, with what judged it and when.
 *
 * The rubric version is shown rather than hidden as an implementation detail:
 * two readings under different rubrics are not the same measurement, and a
 * candidate comparing them deserves to know which is which.
 */
function EvidenceItem({ reading }: { reading: SkillEvidence }) {
  return (
    <li>
      <span>{formatObserved(reading.observed_at)}</span>
      <span>{reading.band}</span>
      <span>{standingLabel[reading.standing]}</span>
      <span>Rubric {reading.rubric_version}</span>
    </li>
  );
}

/** Counts per standing, including the ones at zero so the shape is stable. */
function countByStanding(competencies: SkillStanding[]) {
  const counts = { fresh: 0, aging: 0, stale: 0, none: 0 };
  for (const competency of competencies) {
    if (competency.standing in counts) {
      counts[competency.standing as keyof typeof counts] += 1;
    }
  }
  return counts;
}

/**
 * A date a person reads, in the candidate's own locale-independent form.
 *
 * Long month names rather than a numeric date, because 03/04 means two
 * different days depending on where the reader is and a competency history is
 * exactly where that ambiguity costs something.
 */
function formatObserved(observedAt: string): string {
  return new Date(observedAt).toLocaleDateString("en-GB", {
    day: "numeric",
    month: "long",
    year: "numeric",
    timeZone: "UTC",
  });
}
