import type { ReactNode } from "react";

import { SurfaceState } from "./SurfaceState";

/**
 * The cross-journey state contract from user-journeys.md, one component per
 * state, with each state's content rule enforced by its required props.
 *
 * The rules themselves are documented in this directory's README. The shared
 * principle: every state says what is true, what is still safe, and what to
 * do next, in that order, and claims nothing that is not so. A state that
 * cannot fill its required props has found a screen that does not yet know
 * what it is telling the person, which is the conversation to have before
 * shipping it.
 */

/**
 * Nothing here yet. Requires the action that creates the first item, because
 * absence with no way forward is a dead end wearing a friendly face; the
 * children say what the surface will show once something exists.
 */
export function EmptyState({
  title,
  children,
  action,
}: {
  title: string;
  children: ReactNode;
  action: ReactNode;
}) {
  return (
    <SurfaceState heading={title} actions={action}>
      {children}
    </SurfaceState>
  );
}

/**
 * The load failed. The four required fields are the journey spec's error rule
 * verbatim: what failed, what remains safe, the permitted next action, and a
 * reference identifier support can act on. Announced as an alert - the one
 * state that interrupts.
 */
export function ErrorState({
  what,
  safe,
  action,
  reference,
}: {
  what: string;
  safe: string;
  action: ReactNode;
  reference: string;
}) {
  return (
    <SurfaceState role="alert" heading={what} actions={action}>
      <p>{safe}</p>
      <p className="mt-2 font-mono text-2xs text-fg-3">
        Reference <span className="select-all">{reference}</span>
      </p>
    </SurfaceState>
  );
}

/**
 * Most of the page loaded. The loaded content renders untouched; the notice
 * names exactly which part is missing and offers a retry scoped to it. The
 * failure this exists against is the silent two-thirds of a page reading as
 * the whole.
 */
export function PartialState({
  missing,
  action,
  children,
}: {
  missing: string;
  action: ReactNode;
  children: ReactNode;
}) {
  return (
    <div>
      {children}
      <div
        role="status"
        className="mt-4 flex flex-wrap items-center justify-between gap-3 rounded-md border border-warning-border bg-warning-soft px-4 py-3 text-sm text-warning-fg"
      >
        <p>{missing}. The rest of this page is complete and current.</p>
        {action}
      </div>
    </div>
  );
}

/**
 * The server refused, in-surface. Says what is closed and who can open it,
 * and deliberately offers no retry: the refusal is a decision, not a fault,
 * and a button that will refuse identically is a lie with a spinner.
 */
export function ForbiddenState({
  what,
  grantedBy,
}: {
  what: string;
  grantedBy: string;
}) {
  return (
    <SurfaceState heading={`${what} is not available to you`}>
      <p>
        Your account does not hold access to this, and nothing you did caused
        that. If you need it, ask {grantedBy} to grant it.
      </p>
    </SurfaceState>
  );
}

/** The thing ran out of time or was revoked. Requires the renewal action. */
export function ExpiredState({
  what,
  action,
}: {
  what: string;
  action: ReactNode;
}) {
  return (
    <SurfaceState heading={what} actions={action}>
      <p>
        Nothing you entered has been lost, and nothing further will happen until
        you renew it.
      </p>
    </SurfaceState>
  );
}

/**
 * Still running, longer than usual. The copy the journey spec demands is
 * built in rather than requested: the work is safe, and leaving is fine -
 * a spinner alone teaches people to sit and guard a page.
 */
export function DelayedState({
  what,
  children,
}: {
  what: string;
  children?: ReactNode;
}) {
  return (
    <SurfaceState role="status" heading={what}>
      <p>
        It is still running and nothing is lost. It is safe to leave this page;
        the result will be here when it finishes.
      </p>
      {children}
    </SurfaceState>
  );
}

/**
 * Not enough evidence to say. A neutral fact with a remedy, never a score:
 * the prototype is explicit that an empty track must not read as "scored
 * zero", so this component owns that copy and renders no number at all.
 */
export function InsufficientEvidenceState({
  what,
  remedy,
}: {
  what: string;
  remedy: string;
}) {
  return (
    <div className="rounded-md border border-border bg-surface-2 px-4 py-3 text-sm">
      <p className="font-semibold">{what}</p>
      <p className="mt-1 text-fg-2">
        Insufficient evidence, not scored. This is a gap in what was observed,
        not a verdict on you.
      </p>
      <p className="mt-1 text-fg-2">{remedy}</p>
    </div>
  );
}

/**
 * The input could not be read or assessed. Says what could not be read, what
 * would be readable, and requires the action that provides different input -
 * the person's way forward is the whole point of telling them.
 */
export function UnassessableState({
  what,
  accepted,
  action,
}: {
  what: string;
  accepted: string;
  action: ReactNode;
}) {
  return (
    <SurfaceState heading={what} actions={action}>
      <p>{accepted} Nothing else about your account is affected.</p>
    </SurfaceState>
  );
}

/**
 * The live connection's two-phase story, one component so a screen swaps the
 * phase rather than the markup. Both phases announce politely: an assertive
 * interruption mid-interview is worse than the blip it reports.
 */
export function ConnectionState({
  phase,
  what,
}: {
  phase: "reconnecting" | "recovered";
  what: string;
}) {
  return (
    <p
      role="status"
      className={`rounded-md border px-4 py-2 text-sm ${
        phase === "reconnecting"
          ? "border-warning-border bg-warning-soft text-warning-fg"
          : "border-success-border bg-success-soft text-success-fg"
      }`}
    >
      {phase === "reconnecting"
        ? `Reconnecting to ${what}. Hold on - nothing is lost while we reconnect.`
        : `Reconnected to ${what}. Nothing was lost while the connection was down.`}
    </p>
  );
}

/**
 * A capability is degraded by an incident. Names what is degraded and what
 * still works, so a person can decide whether to continue rather than
 * discovering the edge mid-task.
 */
export function DegradedState({
  what,
  stillWorks,
}: {
  what: string;
  stillWorks: string;
}) {
  return (
    <div
      role="status"
      className="rounded-md border border-warning-border bg-warning-soft px-4 py-3 text-sm text-warning-fg"
    >
      <p className="font-semibold">{what}</p>
      <p className="mt-1">
        {stillWorks} We are on it; you do not need to report this.
      </p>
    </div>
  );
}
