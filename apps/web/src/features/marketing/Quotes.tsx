import { Section, SectionHead } from "./Section";
import { quotes } from "./content";

/**
 * The testimonials, ported from the eighth section.
 *
 * A `blockquote` with a `footer` for the attribution, which is what the elements
 * are for: the quotation is the content and the person is who it is from, and a
 * div would say neither.
 */
export function Quotes() {
  return (
    <Section labelledBy="quotes-h">
      <SectionHead
        id="quotes-h"
        eyebrow={quotes.eyebrow}
        heading={quotes.heading}
        centred
      />

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        {quotes.items.map((item) => (
          <blockquote
            key={item.name}
            className="flex flex-col gap-4.5 rounded-xl border border-border bg-surface p-6"
          >
            <p className="flex-1 font-display text-md leading-[1.45]">
              {item.quote}
            </p>
            <footer className="flex items-center gap-2.5 text-xs text-fg-3">
              <span
                aria-hidden="true"
                className="grid size-7 shrink-0 place-items-center rounded-full bg-surface-3 text-2xs font-semibold text-fg-2"
              >
                {item.initials}
              </span>
              <span>
                <strong className="block text-sm font-semibold text-fg">
                  {item.name}
                </strong>
                {item.role}
              </span>
            </footer>
          </blockquote>
        ))}
      </div>
    </Section>
  );
}
