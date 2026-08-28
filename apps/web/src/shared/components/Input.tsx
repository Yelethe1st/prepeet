import type { LucideIcon } from "lucide-react";
import type { InputHTMLAttributes, ReactNode } from "react";
import { forwardRef } from "react";

import { Icon } from "./Icon";

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  /**
   * A glyph inside the leading edge of the field, as the prototype's
   * `.input-wrap` draws one. Decoration: it repeats the label, so it is hidden
   * from assistive technology like every other icon here.
   */
  icon?: LucideIcon;
  /**
   * A control inside the trailing edge, which in practice means the password
   * field's show and hide button. It is a child rather than a prop of its own
   * because only the caller knows what it does.
   */
  trailing?: ReactNode;
}

/**
 * A text input, ported from the `.input` block in the prototype.
 *
 * A component rather than a utility string repeated at each call site, because
 * the invalid and focus treatments are the part that gets forgotten: an input
 * that turns red only when somebody remembers to add the class is an input that
 * is sometimes silently wrong.
 *
 * It forwards its ref because the password fields are uncontrolled, so their
 * value never appears in the serialised DOM where an error reporter or session
 * replay would capture it.
 */
export const Input = forwardRef<HTMLInputElement, InputProps>(function Input(
  { className, icon, trailing, ...props },
  ref,
) {
  const field = (
    <input
      {...props}
      ref={ref}
      // The caller's classes come last so a screen can extend the base --
      // the one-time-code box widens its tracking -- without being able to
      // silently lose the invalid and focus treatments, which live in the
      // base string.
      className={[
        "w-full rounded-md border border-border bg-surface px-3 py-2 text-base text-fg " +
          "transition-colors placeholder:text-fg-muted hover:border-fg-muted " +
          "focus:border-primary disabled:cursor-not-allowed disabled:bg-surface-3 " +
          "disabled:text-fg-3 aria-invalid:border-danger",
        icon === undefined ? "" : "pl-9",
        trailing === undefined ? "" : "pr-11",
        className,
      ]
        .filter(Boolean)
        .join(" ")}
    />
  );

  if (icon === undefined && trailing === undefined) return field;

  return (
    <div className="relative flex items-center">
      {icon === undefined ? null : (
        <span className="pointer-events-none absolute left-3 text-fg-muted">
          <Icon glyph={icon} size="sm" />
        </span>
      )}
      {field}
      {trailing === undefined ? null : (
        <span className="absolute right-1.5">{trailing}</span>
      )}
    </div>
  );
});
