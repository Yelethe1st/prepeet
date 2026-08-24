import type { InputHTMLAttributes } from "react";
import { forwardRef } from "react";

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
export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(
  function Input(props, ref) {
    return (
      <input
        {...props}
        ref={ref}
        className={
          "w-full rounded-md border border-border bg-surface px-3 py-2 text-base text-fg " +
          "transition-colors placeholder:text-fg-muted hover:border-fg-muted " +
          "focus:border-primary disabled:cursor-not-allowed disabled:bg-surface-3 " +
          "disabled:text-fg-3 aria-invalid:border-danger"
        }
      />
    );
  },
);
