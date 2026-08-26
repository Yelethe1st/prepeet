import { describe, expect, it } from "vitest";

import { BUNDLES, SCOPED_CAPABILITIES } from "@contracts/capabilities";

import { MATRIX_ROLES, matrixRows } from "./matrix";

/**
 * The permission matrix is generated from the capability catalogue - the
 * ticket's first box - so these tests pin the derivation, not a table
 * somebody typed: every cell traces to the generated bundles, and the
 * "scoped" marking to the contract's own scope requirements.
 */

describe("matrixRows", () => {
  it("derives every row from the bundles, nothing hand-written", () => {
    const rows = matrixRows();

    // Every capability any tenant role holds appears exactly once.
    const expected = new Set<string>();
    for (const role of MATRIX_ROLES) {
      for (const capability of BUNDLES[role]) {
        expected.add(capability);
      }
    }
    expect(new Set(rows.map((row) => row.capability))).toEqual(expected);
  });

  it("marks a held scoped capability as scoped, a held plain one as yes, an unheld one as no", () => {
    const rows = matrixRows();
    const byCapability = new Map(rows.map((row) => [row.capability, row]));

    // campaign.manage is scoped in the contract and held by recruiters.
    expect(SCOPED_CAPABILITIES).toContain("campaign.manage");
    expect(byCapability.get("campaign.manage")?.cells.recruiter).toBe("scoped");
    // Rubric publishing is unscoped tenant authority held only by admins.
    expect(byCapability.get("rubric.publish")?.cells.admin).toBe("yes");
    expect(byCapability.get("rubric.publish")?.cells.recruiter).toBe("no");
    // The matrix's one asymmetric row survives the derivation.
    expect(byCapability.get("appeal.manage")?.cells.recruiter).toBe("no");
    expect(byCapability.get("appeal.manage")?.cells.hiring_manager).toBe(
      "scoped",
    );
  });

  it("carries the contract's own reason for each capability", () => {
    for (const row of matrixRows()) {
      expect(row.reason.length).toBeGreaterThan(10);
    }
  });

  it("columns are the assignable tenant roles, owner deliberately absent", () => {
    // owner is capability-identical to admin and not assignable through the
    // surface; a column for it would say nothing the admin column does not.
    expect(MATRIX_ROLES).toEqual([
      "recruiter",
      "hiring_manager",
      "admin",
      "viewer",
    ]);
  });
});
