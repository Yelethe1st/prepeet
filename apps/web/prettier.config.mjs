/**
 * Prettier, for `pnpm format` and the `format:check` the lint target runs.
 *
 * The defaults are kept deliberately. The codebase was written by hand before
 * anything formatted it, and measured against the formatter it sits closest to
 * Prettier's own 80 columns: widening to 100 made the diff larger rather than
 * smaller, because the formatter would then join lines somebody had split on
 * purpose. Picking the width the code already has is the cheapest way to make
 * the two agree.
 *
 * The file exists rather than relying on those defaults implicitly, so that a
 * Prettier upgrade that changes one cannot quietly restyle the repository.
 */
export default {
  printWidth: 80,
  semi: true,
  singleQuote: false,
  trailingComma: "all",
  tabWidth: 2,
};
