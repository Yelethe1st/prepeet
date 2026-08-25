import type { ReactNode } from "react";

/**
 * The prototype's `.state` block: a centred heading, a short explanation
 * bounded to a readable measure, and the actions underneath. Every named
 * state in this directory renders through it, so the eleven states of the
 * cross-journey contract stay one recognisable shape rather than eleven
 * dialects of "something is off".
 *
 * The role is decided by the state that owns the copy, not here: a failure
 * interrupts as an alert, everything else waits as a status or says nothing.
 * Icons are deliberately absent, as they are from ErrorScreen: the production
 * port adds a dependency when a screen needs it, and none has.
 */
export function SurfaceState({
  role,
  heading,
  headingId,
  children,
  actions,
}: {
  role?: "alert" | "status";
  heading: string;
  /** Set when the enclosing card labels itself by this heading. */
  headingId?: string;
  children: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <div
      role={role}
      className="flex flex-col items-center gap-[10px] px-6 py-12 text-center"
    >
      <h3 id={headingId} className="text-md font-semibold">
        {heading}
      </h3>
      <div className="max-w-[420px] text-sm leading-[1.55] text-fg-2">
        {children}
      </div>
      {actions ? (
        <div className="mt-2 flex flex-wrap justify-center gap-2">
          {actions}
        </div>
      ) : null}
    </div>
  );
}
