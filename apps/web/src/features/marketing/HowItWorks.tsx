import { Check, Minus, X, CircleCheckBig, type LucideIcon } from "lucide-react";

import { ButtonLink } from "@/shared/components";
import { Icon } from "@/shared/components/Icon";

import { Section, SectionHead } from "./Section";
import { howItWorks, type Mark } from "./content";

/**
 * How each answer reads, and it is never colour alone.
 *
 * A different glyph as well as a different colour, and the sentence itself in
 * every cell. Somebody who cannot tell the green tick from the red cross still
 * reads "Never shown", which is the part that matters.
 */
const marks: Record<Mark, { glyph: LucideIcon; tone: string }> = {
  yes: { glyph: Check, tone: "text-success" },
  no: { glyph: X, tone: "text-danger" },
  na: { glyph: Minus, tone: "text-fg-muted" },
  // The screen candidate's end-of-session view: a confirmation and nothing
  // else. Neither a yes nor a no, and the prototype draws it as its own thing.
  limited: { glyph: CircleCheckBig, tone: "text-danger" },
};

function Cell({ mark, text }: { mark: Mark; text: string }) {
  const { glyph, tone } = marks[mark];

  return (
    <td className="border-b border-border-subtle px-3 py-3 align-top text-sm text-fg-2">
      <span className="flex items-start gap-2 leading-[1.45]">
        <span className="mt-0.5">
          <Icon glyph={glyph} size="sm" tone={tone} />
        </span>
        {text}
      </span>
    </td>
  );
}

/**
 * The four steps and the visibility table, ported from the fifth section.
 *
 * The table is the most important thing on this page. ADR-0018 requires the
 * isolation guarantee to appear in candidate-facing copy wherever practice and
 * screening meet, and this is the first place a visitor meets both: practice
 * results are the candidate's own and reach no employer, and a screen candidate
 * is shown no score, band, chart, coaching or reviewer note at all. The row
 * order and the wording are the prototype's, which is where that promise was
 * first written down.
 *
 * One recorded deviation. The prototype restyles the table into stacked cards
 * on a phone, which needs `display: block` on the rows and cells. That strips
 * the row and column relationships assistive technology navigates a table by,
 * and the prototype puts the row header back visually with a `data-label` and
 * not semantically. It stays a table here at every width and scrolls sideways
 * inside its own box instead, which keeps the relationships and keeps the page
 * itself from scrolling.
 */
export function HowItWorks() {
  return (
    <Section id="how" labelledBy="how-h">
      <SectionHead
        id="how-h"
        eyebrow={howItWorks.eyebrow}
        heading={howItWorks.heading}
        lead={howItWorks.lead}
      />

      <ol className="grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-4">
        {howItWorks.steps.map((step, index) => (
          <li key={step.title} className="pt-4.5">
            <span
              aria-hidden="true"
              className="mb-3.5 grid size-9 place-items-center rounded-full bg-primary text-sm font-bold text-primary-fg"
            >
              {index + 1}
            </span>
            <h3 className="mb-1.5 text-base font-semibold">{step.title}</h3>
            <p className="text-sm leading-[1.55] text-fg-2">{step.body}</p>
          </li>
        ))}
      </ol>

      <div className="mt-16">
        <h3 id="visibility-heading" className="mb-2 text-lg font-semibold">
          {howItWorks.tableHeading}
        </h3>
        <p className="mb-5 max-w-[680px] text-sm leading-relaxed text-fg-2">
          {howItWorks.tableLead}
        </p>

        {/*
          The scroll container. Without it the table sets the width of the
          document on a phone and the whole page scrolls sideways, which is the
          failure the layout suite exists to catch.

          It takes focus, and is a named region, because a region that scrolls
          and cannot be focused is one that only a pointer can reach: on a phone
          the last two columns of this table would be unreachable by keyboard.
        */}
        <div
          tabIndex={0}
          role="region"
          aria-labelledby="visibility-heading"
          className="overflow-x-auto rounded-lg border border-border"
        >
          <table className="w-full min-w-[720px] border-collapse text-left">
            <caption className="sr-only">{howItWorks.tableCaption}</caption>
            <thead>
              <tr>
                {howItWorks.columns.map((column) => (
                  <th
                    key={column}
                    scope="col"
                    className="border-b border-border bg-surface-2 px-3 py-2.5 text-2xs font-semibold tracking-[0.06em] text-fg-3 uppercase"
                  >
                    {column}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {howItWorks.rows.map((row) => (
                <tr key={row.what}>
                  <th
                    scope="row"
                    className="w-[26%] border-b border-border-subtle bg-surface px-3 py-3 align-top text-sm font-semibold"
                  >
                    {row.what}
                  </th>
                  <Cell mark={row.practice.mark} text={row.practice.text} />
                  <Cell mark={row.screen.mark} text={row.screen.text} />
                  <Cell mark={row.recruiter.mark} text={row.recruiter.text} />
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <div className="mt-4.5 flex flex-wrap gap-2">
          <ButtonLink href="/practice" size="sm" variant="secondary">
            Look at the candidate side
          </ButtonLink>
          <ButtonLink href="#evidence" size="sm" variant="secondary">
            See what a reviewer reads
          </ButtonLink>
          <ButtonLink href="#faq" size="sm" variant="ghost">
            How appeals are handled
          </ButtonLink>
        </div>
      </div>
    </Section>
  );
}
