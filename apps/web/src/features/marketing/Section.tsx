import type { ReactNode } from "react";

/**
 * The marketing page's section shell, ported from `.mk-section`.
 *
 * The prototype repeats one measurement in ten places: 1180px wide, centred,
 * with vertical rhythm that halves on a phone. Repeating it here as utilities
 * would be ten chances for one of them to drift, and the drift is invisible in
 * review because every copy looks correct on its own.
 */
export function Section({
  id,
  labelledBy,
  tight = false,
  children,
}: {
  id?: string;
  /** The heading that names this section. Every section has one, so every section is labelled. */
  labelledBy: string;
  /** The prototype's `.tight` modifier: less air, for the strips between the big sections. */
  tight?: boolean;
  children: ReactNode;
}) {
  return (
    <section
      id={id}
      aria-labelledby={labelledBy}
      className={
        "mx-auto max-w-[1180px] px-5 md:px-6 " +
        (tight ? "py-12 md:py-16" : "py-16 md:py-24")
      }
    >
      {children}
    </section>
  );
}

/**
 * The eyebrow, heading and standfirst that open a section.
 *
 * `centred` is the prototype's `.center` modifier, used only by the
 * testimonials. The heading level is fixed at 2 rather than passed in: this is
 * a page with one h1, and a section heading that could be any level is how a
 * document ends up with an outline nobody can follow.
 */
export function SectionHead({
  id,
  eyebrow,
  heading,
  lead,
  centred = false,
}: {
  id: string;
  eyebrow: string;
  heading: string;
  lead?: string;
  centred?: boolean;
}) {
  return (
    <div
      className={
        "mb-12 max-w-[680px] " + (centred ? "mx-auto text-center" : "")
      }
    >
      <p className="text-2xs font-bold tracking-[0.1em] text-primary uppercase">
        {eyebrow}
      </p>
      <h2
        id={id}
        className="mt-3 font-display text-[clamp(1.75rem,3vw,2.5rem)] leading-[1.12] font-medium tracking-[-0.02em]"
      >
        {heading}
      </h2>
      {lead === undefined ? null : (
        <p className="mt-3.5 text-md leading-relaxed text-fg-2">{lead}</p>
      )}
    </div>
  );
}
