import {
  BUNDLES,
  CAPABILITY_REASONS,
  SCOPED_CAPABILITIES,
  type Capability,
} from "@contracts/capabilities";

/**
 * The permission matrix, derived - never written - from the capability
 * catalogue's own bundles. TEN-02's first box: the screen an administrator
 * reads and the authority the server grants come from one artifact, so they
 * cannot drift apart.
 *
 * The columns are the assignable tenant roles. owner is deliberately absent:
 * it is capability-identical to admin and not assignable through this
 * surface, so a column for it would say nothing the admin column does not.
 */

export const MATRIX_ROLES = [
  "recruiter",
  "hiring_manager",
  "admin",
  "viewer",
] as const;
export type MatrixRole = (typeof MATRIX_ROLES)[number];

/** yes: held. scoped: held, but only within an explicit assignment. no. */
export type MatrixCell = "yes" | "scoped" | "no";

export interface MatrixRow {
  capability: Capability;
  /** The contract's own reason - the words legal and security reviewed. */
  reason: string;
  cells: Record<MatrixRole, MatrixCell>;
}

/** The role names as the prototype writes them. */
export const ROLE_LABELS: Record<MatrixRole, string> = {
  recruiter: "Recruiter",
  hiring_manager: "Hiring manager",
  admin: "Tenant admin",
  viewer: "Read-only",
};

/** Every capability any tenant role holds, one row each, in catalogue order. */
export function matrixRows(): MatrixRow[] {
  const scoped = new Set<string>(SCOPED_CAPABILITIES);
  const seen = new Set<Capability>();
  const rows: MatrixRow[] = [];

  for (const role of MATRIX_ROLES) {
    for (const capability of BUNDLES[role]) {
      if (seen.has(capability)) {
        continue;
      }
      seen.add(capability);
      const cells = {} as Record<MatrixRole, MatrixCell>;
      for (const column of MATRIX_ROLES) {
        const held = (BUNDLES[column] as readonly Capability[]).includes(
          capability,
        );
        cells[column] = held
          ? scoped.has(capability)
            ? "scoped"
            : "yes"
          : "no";
      }
      rows.push({ capability, reason: CAPABILITY_REASONS[capability], cells });
    }
  }
  return rows;
}
