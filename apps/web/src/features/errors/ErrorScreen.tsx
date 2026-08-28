import type { ReactNode } from "react";

/**
 * The frame every failure destination shares, from the error-* prototypes.
 *
 * One frame so the five destinations stay distinguishable by what they say
 * rather than by drifting layouts: a status chip, a headline, the honest
 * explanation, real actions, and the facts a person or support can act on.
 *
 * The copy discipline the prototypes set and this component inherits: lead
 * with what did or did not happen ("no data was read", "nothing you did"),
 * never blame the person, and only claim automation that exists.
 */

interface Fact {
  label: string;
  value: string;
  /** Rendered in the mono face: references, routes, capability names. */
  mono?: boolean;
}

interface ErrorScreenProps {
  /** The status chip: "403 · permission denied". */
  badge: string;
  title: string;
  /** The explanation under the title. */
  children: ReactNode;
  /** The way forward. Every destination must offer one. */
  actions: ReactNode;
  /** What was requested and what was decided, for the person and for support. */
  facts?: Fact[];
  /** The heading over the facts, when there are any. */
  factsTitle?: string;
}

export function ErrorScreen({
  badge,
  title,
  children,
  actions,
  facts,
  factsTitle,
}: ErrorScreenProps) {
  return (
    <main
      id="main-content"
      className="mx-auto flex min-h-screen w-full max-w-[560px] flex-col justify-center px-6 py-12"
    >
      <p className="font-mono text-2xs tracking-[0.08em] text-fg-3 uppercase">
        {badge}
      </p>
      <h1 className="mt-3 font-display text-2xl tracking-[-0.02em]">{title}</h1>
      <div className="mt-3 space-y-3 text-sm leading-[1.55] text-fg-2">
        {children}
      </div>

      <div className="mt-6 flex flex-wrap items-center gap-3">{actions}</div>

      {facts && facts.length > 0 ? (
        <section className="mt-10 border-t border-border pt-5">
          {factsTitle ? (
            <h2 className="text-sm font-semibold">{factsTitle}</h2>
          ) : null}
          <dl className="mt-3 space-y-2 text-sm">
            {facts.map((fact) => (
              <div key={fact.label} className="flex justify-between gap-4">
                <dt className="shrink-0 text-fg-2">{fact.label}</dt>
                <dd
                  className={
                    fact.mono ? "font-mono text-xs break-all" : "text-right"
                  }
                >
                  {fact.value}
                </dd>
              </div>
            ))}
          </dl>
        </section>
      ) : null}
    </main>
  );
}
