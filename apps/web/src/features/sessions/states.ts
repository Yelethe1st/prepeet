/**
 * The lifecycle vocabulary - SES-07's heart, as data.
 *
 * Every state the machine can reach has a row here: how it reads, which
 * filter group it belongs to, and the one action that actually applies.
 * The completeness test walks the machine's own state list against this
 * map, so a state added to the machine without a row here fails the build
 * - which is exactly the prototype gap the ticket recorded ("abandoned"
 * shown, expired and cancelled missing) made structural.
 */

/** Every state interview.sessions can hold, from the schema's own CHECK. */
export const MACHINE_STATES = [
  "draft",
  "composing",
  "ready",
  "connecting",
  "in_progress",
  "reconnecting",
  "finalizing",
  "evaluating",
  "review_ready",
  "archived",
  "cancelled",
  "expired",
  "composition_failed",
  "interrupted",
  "finalization_failed",
  "evaluation_failed",
] as const;

export type MachineState = (typeof MACHINE_STATES)[number];

export type FilterGroup = "active" | "finished" | "attention";

export interface StateRow {
  /** How the state reads to the person, in their words. */
  label: string;
  group: FilterGroup;
  /** The one action that applies, and where it goes. */
  action: string;
  href: (sessionId: string) => string;
}

const newOne = () => "/practice/new";
const prepare = (id: string) => `/session/${id}/prepare`;
const complete = (id: string) => `/session/${id}/complete`;
const results = (id: string) => `/session/${id}/results`;

export const STATE_ROWS: Record<MachineState, StateRow> = {
  draft: {
    label: "Not set up yet",
    group: "active",
    action: "Continue setup",
    href: newOne,
  },
  composing: {
    label: "Being composed",
    group: "active",
    action: "View progress",
    href: prepare,
  },
  ready: {
    label: "Ready to start",
    group: "active",
    action: "Prepare and start",
    href: prepare,
  },
  connecting: {
    label: "Connecting",
    group: "active",
    action: "Rejoin",
    href: prepare,
  },
  in_progress: {
    label: "Live now",
    group: "active",
    action: "Rejoin",
    href: prepare,
  },
  reconnecting: {
    label: "Reconnecting",
    group: "active",
    action: "Rejoin",
    href: prepare,
  },
  finalizing: {
    label: "Sealing the transcript",
    group: "active",
    action: "See processing",
    href: complete,
  },
  evaluating: {
    label: "Being evaluated",
    group: "active",
    action: "See processing",
    href: complete,
  },
  review_ready: {
    label: "Results ready",
    group: "finished",
    action: "Outcome and evidence",
    href: results,
  },
  archived: {
    label: "Archived",
    group: "finished",
    action: "Outcome and evidence",
    href: results,
  },
  cancelled: {
    label: "Cancelled",
    group: "finished",
    action: "Start a new one",
    href: newOne,
  },
  expired: {
    label: "Expired before starting",
    group: "finished",
    action: "Start a new one",
    href: newOne,
  },
  composition_failed: {
    label: "Setup failed on our side",
    group: "attention",
    action: "Start a new one",
    href: newOne,
  },
  interrupted: {
    label: "Interrupted",
    group: "attention",
    action: "See what was kept",
    href: complete,
  },
  finalization_failed: {
    label: "Sealing failed on our side",
    group: "attention",
    action: "See status",
    href: complete,
  },
  evaluation_failed: {
    label: "Evaluation failed on our side",
    group: "attention",
    action: "See status",
    href: complete,
  },
};
