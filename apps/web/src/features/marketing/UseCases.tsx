import { Section, SectionHead } from "./Section";
import { useCases } from "./content";

/**
 * The six domains, ported from the third section.
 *
 * Each card names the competency the interview is actually probing and then
 * quotes a question from it, which is the section's whole argument: six domains
 * means six different conversations, not one conversation with the nouns
 * changed.
 *
 * One recorded deviation: the prototype's closing line links to rubric
 * management, which is not ported. It states the same fact without the link.
 */
export function UseCases() {
  return (
    <Section id="use-cases" labelledBy="usecases-h">
      <SectionHead
        id="usecases-h"
        eyebrow={useCases.eyebrow}
        heading={useCases.heading}
        lead={useCases.lead}
      />

      <ul className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {useCases.items.map((item) => (
          <li
            key={item.role}
            className="rounded-lg border border-border bg-surface-2 p-5.5"
          >
            <p className="text-2xs font-bold tracking-[0.1em] text-primary uppercase">
              {item.domain}
            </p>
            <h3 className="mt-2 mb-1.5 text-base font-semibold">{item.role}</h3>
            <p className="text-sm leading-[1.55] text-fg-2">
              Competency in focus:{" "}
              <strong className="font-semibold text-fg">
                {item.competency}
              </strong>
              . {item.probe}
            </p>
            <p className="mt-3 rounded-md border border-border bg-surface px-3 py-2.5 text-xs text-fg-2 italic">
              {item.question}
            </p>
          </li>
        ))}
      </ul>

      <p className="mt-5.5 text-xs text-fg-3">{useCases.footnote}</p>
    </Section>
  );
}
