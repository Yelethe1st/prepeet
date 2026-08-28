import { Section } from "./Section";
import { proof } from "./content";

/**
 * The logo strip and the numbers under it, ported from the second section.
 *
 * The heading is for assistive technology only, exactly as in the prototype: a
 * strip of names needs no visible title, and a section with no heading at all is
 * one that cannot be navigated to or skipped.
 *
 * One recorded deviation: the prototype dims the whole strip to 80% opacity.
 * The muted foreground token carries the same intent, and the two together put
 * the names close enough to the background to fail the contrast the rest of the
 * page is held to.
 */
export function Proof() {
  return (
    <Section labelledBy="proof-h" tight>
      <h2 id="proof-h" className="sr-only">
        {proof.heading}
      </h2>

      <p className="mx-auto mb-6 max-w-[720px] text-center text-xs text-fg-3">
        {proof.lead}
      </p>

      <ul className="flex flex-wrap items-center justify-center gap-x-9 gap-y-3.5">
        {proof.logos.map((logo) => (
          <li
            key={logo.initials}
            className="inline-flex items-center gap-2 text-md font-bold tracking-[-0.01em] text-fg-3"
          >
            <span
              aria-hidden="true"
              className="inline-grid size-[22px] place-items-center rounded-sm border border-border bg-surface-3 text-[10px] text-fg-2"
            >
              {logo.initials}
            </span>
            {logo.name}
          </li>
        ))}
      </ul>

      <dl className="mt-11 grid grid-cols-2 gap-4 rounded-xl border border-border bg-surface p-6 md:grid-cols-4">
        {proof.stats.map((stat) => (
          <div key={stat.label}>
            <dt className="sr-only">{stat.label}</dt>
            <dd>
              <span className="block text-2xl font-bold tracking-[-0.02em] tabular-nums">
                {stat.value}
              </span>
              <span aria-hidden="true" className="mt-1 block text-xs text-fg-3">
                {stat.label}
              </span>
            </dd>
          </div>
        ))}
      </dl>
    </Section>
  );
}
