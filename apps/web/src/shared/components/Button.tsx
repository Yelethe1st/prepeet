import type { ButtonHTMLAttributes, Ref, ReactNode } from "react";

/**
 * The button variants the prototype defines in components.css.
 *
 * A union rather than a free string, so a typo is a compile error instead of a
 * button that silently renders unstyled.
 */
export type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";
export type ButtonSize = "sm" | "md" | "lg";

export interface ButtonProps extends Omit<
  ButtonHTMLAttributes<HTMLButtonElement>,
  "className"
> {
  /**
   * React 19 ref-as-prop, reaching the underlying button. Exists because the
   * prepare screen moves focus to "the one thing blocking start", and a
   * focus target the design system cannot name is a focus that goes nowhere.
   */
  ref?: Ref<HTMLButtonElement>;
  variant?: ButtonVariant;
  size?: ButtonSize;
  /** Fills the width of its container, as the sign-in button does. */
  block?: boolean;
  /**
   * Shows the button as working and prevents a second submission.
   *
   * Separate from `disabled` because they mean different things to a person: a
   * disabled button is one they may not use, and a busy one is a request they
   * already made. They also need different announcements.
   */
  busy?: boolean;
  /** What to say while busy. Defaults to the button's own label. */
  busyLabel?: string;
  children: ReactNode;
}

/**
 * The values are the prototype's, by way of the tokens Tailwind is configured
 * from: `bg-primary` resolves through `--primary`, so a change to the palette in
 * tokens.css arrives here without this file being touched.
 */
const base =
  "inline-flex items-center justify-center gap-2 rounded-md font-semibold leading-none " +
  "whitespace-nowrap transition-colors disabled:cursor-not-allowed disabled:opacity-60";

const variants: Record<ButtonVariant, string> = {
  primary: "bg-primary text-primary-fg hover:bg-primary-hover",
  secondary:
    "bg-surface text-fg border border-border-strong shadow-xs hover:bg-surface-2",
  ghost: "bg-transparent text-fg-2 hover:bg-surface-3 hover:text-fg",
  danger: "bg-danger text-primary-fg hover:bg-danger-hover",
};

const sizes: Record<ButtonSize, string> = {
  sm: "min-h-8 px-3 text-xs",
  md: "min-h-10 px-4 text-sm",
  lg: "min-h-12 px-5 text-base",
};

/**
 * A button, ported from the `.btn` block in the prototype.
 *
 * Its one behaviour beyond styling is the busy state, which exists because a
 * form that can be submitted twice will be. Double submission of a login is
 * harmless; of an interview start it is a second billed session.
 */
export function Button({
  variant = "primary",
  size = "md",
  block = false,
  busy = false,
  busyLabel,
  disabled,
  children,
  type = "button",
  ...rest
}: ButtonProps) {
  return (
    <button
      {...rest}
      type={type}
      className={[base, variants[variant], sizes[size], block ? "w-full" : ""]
        .filter(Boolean)
        .join(" ")}
      // Disabled while busy so the second click does nothing, and announced as
      // busy so somebody using a screen reader knows why nothing happened.
      disabled={disabled === true || busy}
      aria-busy={busy ? true : undefined}
    >
      {busy && busyLabel ? busyLabel : children}
    </button>
  );
}

/**
 * A link dressed as a button, for a primary action that is a navigation.
 *
 * The prototype puts its `.btn` class on anchors exactly this way. A separate
 * component rather than an `as` prop on Button, because a button that submits
 * and a link that navigates differ in every way assistive technology cares
 * about, and one component pretending to be both hides which one it is.
 */
export function ButtonLink({
  variant = "primary",
  size = "md",
  block = false,
  children,
  ...rest
}: {
  variant?: ButtonVariant;
  size?: ButtonSize;
  block?: boolean;
  children: React.ReactNode;
} & React.AnchorHTMLAttributes<HTMLAnchorElement>) {
  return (
    <a
      {...rest}
      className={[base, variants[variant], sizes[size], block ? "w-full" : ""]
        .filter(Boolean)
        .join(" ")}
    >
      {children}
    </a>
  );
}
