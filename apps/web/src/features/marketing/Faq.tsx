"use client";

import { ChevronDown } from "lucide-react";
import { useState } from "react";

import { Icon } from "@/shared/components/Icon";

import { Section, SectionHead } from "./Section";
import { faq } from "./content";

/**
 * The FAQ, ported from the ninth section.
 *
 * One panel open at a time, which is the prototype's `data-accordion-single`,
 * and the first one open on arrival so the pattern is legible without anybody
 * having to guess that the headings are buttons.
 *
 * The panel stays in the document and is hidden with the `hidden` attribute
 * rather than removed. `aria-controls` must point at an element that exists, and
 * a control referring to nothing is a control assistive technology cannot
 * describe.
 *
 * Every answer here is the prototype's, including the ones that say no. Three of
 * them are the isolation guarantee in ADR-0018 written out: screen candidates
 * see no score, coaching is never produced in screen mode rather than merely
 * withheld, and practice history reaches no employer at all.
 */
export function Faq() {
  const [open, setOpen] = useState<string | null>(faq.items[0]!.id);

  return (
    <Section id="faq" labelledBy="faq-h">
      <SectionHead
        id="faq-h"
        eyebrow={faq.eyebrow}
        heading={faq.heading}
        lead={faq.lead}
      />

      <div className="divide-y divide-border overflow-hidden rounded-lg border border-border bg-surface">
        {faq.items.map((item) => {
          const expanded = open === item.id;

          return (
            <div key={item.id}>
              <h3>
                <button
                  type="button"
                  id={`faq-${item.id}-trigger`}
                  aria-expanded={expanded}
                  aria-controls={`faq-${item.id}-panel`}
                  onClick={() => setOpen(expanded ? null : item.id)}
                  className="flex w-full items-center justify-between gap-3 px-5 py-4 text-left text-base font-semibold transition-colors hover:bg-surface-2"
                >
                  {item.question}
                  <span className={expanded ? "rotate-180" : ""}>
                    <Icon glyph={ChevronDown} />
                  </span>
                </button>
              </h3>
              <div
                id={`faq-${item.id}-panel`}
                role="region"
                aria-labelledby={`faq-${item.id}-trigger`}
                hidden={!expanded}
                className="px-5 pb-4.5"
              >
                <p className="max-w-[820px] text-sm leading-relaxed text-fg-2">
                  {item.answer}
                </p>
              </div>
            </div>
          );
        })}
      </div>
    </Section>
  );
}
