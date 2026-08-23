/**
 * The token names components are allowed to use.
 *
 * These are derived from `tokens.css` and asserted against it by
 * `tokens.test.ts`, so the two cannot drift. Import `ColorToken` where a prop
 * takes a colour, and the compiler will reject a raw palette value or a token
 * that no longer exists.
 */

/** Semantic colour tokens. Both themes define every one of these. */
export const colorTokens = [
  "--bg",
  "--bg-subtle",
  "--surface",
  "--surface-2",
  "--surface-3",
  "--surface-inset",
  "--overlay",
  "--border",
  "--border-strong",
  "--border-subtle",
  "--fg",
  "--fg-2",
  "--fg-3",
  "--fg-muted",
  "--fg-inverse",
  "--primary",
  "--primary-hover",
  "--primary-active",
  "--primary-fg",
  "--primary-soft",
  "--primary-soft-fg",
  "--primary-border",
  "--accent",
  "--accent-soft",
  "--accent-fg",
  "--success",
  "--success-soft",
  "--success-fg",
  "--success-border",
  "--warning",
  "--warning-soft",
  "--warning-fg",
  "--warning-border",
  "--danger",
  "--danger-hover",
  "--danger-soft",
  "--danger-fg",
  "--danger-border",
  "--info",
  "--info-soft",
  "--info-fg",
  "--info-border",
  "--neutral-soft",
  "--neutral-fg",
  "--focus-color",
  "--shadow-xs",
  "--shadow-sm",
  "--shadow-md",
  "--shadow-lg",
  "--sidebar-bg",
  "--sidebar-fg",
  "--sidebar-fg-active",
  "--sidebar-active",
  "--sidebar-border",
  "--skeleton",
  "--skeleton-shine",
  "--waveform-idle",
  "--waveform-user",
  "--waveform-ai",
] as const;

/** Spacing scale, on a 4px base. */
export const spaceTokens = [
  "--sp-1",
  "--sp-2",
  "--sp-3",
  "--sp-4",
  "--sp-5",
  "--sp-6",
  "--sp-8",
  "--sp-10",
  "--sp-12",
  "--sp-16",
  "--sp-20",
  "--sp-24",
] as const;

/** A semantic colour token name, such as `--primary`. */
export type ColorToken = (typeof colorTokens)[number];

/** A spacing token name, such as `--sp-4`. */
export type SpaceToken = (typeof spaceTokens)[number];

/**
 * Returns the CSS `var()` reference for a token.
 *
 * Components call this rather than writing `var(--primary)` by hand, so a
 * renamed token becomes a type error instead of a silently empty value.
 */
export function token(name: ColorToken | SpaceToken): string {
  return `var(${name})`;
}
