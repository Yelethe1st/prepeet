import "vitest";
import type { AxeMatchers } from "vitest-axe/matchers";

/**
 * Types for the axe matchers registered in vitest.setup.ts.
 *
 * Without this the matcher works at runtime and fails type checking, which is
 * the shape of problem people solve by reaching for `any` at the call site.
 *
 * The empty interfaces are the point rather than an oversight: module
 * augmentation merges these declarations into vitest's own, and there is
 * nothing to add beyond the matchers they extend.
 */
declare module "vitest" {
  // eslint-disable-next-line @typescript-eslint/no-empty-object-type
  interface Assertion extends AxeMatchers {}
  // eslint-disable-next-line @typescript-eslint/no-empty-object-type
  interface AsymmetricMatchersContaining extends AxeMatchers {}
}
