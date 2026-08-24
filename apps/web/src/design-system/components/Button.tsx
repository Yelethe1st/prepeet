import type { ButtonHTMLAttributes, ReactNode } from "react";

/**
 * The button variants the prototype defines in components.css.
 *
 * A union rather than a free string, so a typo is a compile error instead of a
 * button that silently renders unstyled.
 */
export type ButtonVariant = "primary" | "ghost" | "danger" | "danger-soft" | "outline";
export type ButtonSize = "sm" | "md" | "lg";

export interface ButtonProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, "className"> {
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
  const classes = ["btn", `btn-${variant}`];
  if (size !== "md") classes.push(`btn-${size}`);
  if (block) classes.push("btn-block");

  return (
    <button
      {...rest}
      type={type}
      className={classes.join(" ")}
      // Disabled while busy so the second click does nothing, and announced as
      // busy so somebody using a screen reader knows why nothing happened.
      disabled={disabled === true || busy}
      aria-busy={busy ? true : undefined}
    >
      {busy && busyLabel ? busyLabel : children}
    </button>
  );
}
