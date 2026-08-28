import { Icon } from "@/shared/components/Icon";

import { Section, SectionHead } from "./Section";
import { features } from "./content";

/**
 * The six product claims, ported from the fourth section.
 *
 * The icons are decoration and are hidden: each one sits directly above the
 * heading it illustrates, so announcing them would be the heading read twice.
 */
export function Features() {
  return (
    <Section id="product" labelledBy="features-h">
      <SectionHead
        id="features-h"
        eyebrow={features.eyebrow}
        heading={features.heading}
        lead={features.lead}
      />

      <ul className="grid grid-cols-1 gap-4.5 sm:grid-cols-2 lg:grid-cols-3">
        {features.items.map((feature) => (
          <li
            key={feature.title}
            className="rounded-xl border border-border bg-surface p-6.5"
          >
            <span className="mb-4 grid size-10 place-items-center rounded-md bg-primary-soft text-primary">
              <Icon glyph={feature.glyph} size="lg" />
            </span>
            <h3 className="mb-2 text-md font-semibold">{feature.title}</h3>
            <p className="text-sm leading-relaxed text-fg-2">{feature.body}</p>
          </li>
        ))}
      </ul>
    </Section>
  );
}
