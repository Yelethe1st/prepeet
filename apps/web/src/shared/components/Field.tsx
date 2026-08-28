import type { ReactNode } from "react";
import { useId } from "react";

/**
 * The props a Field hands to the control it wraps.
 *
 * The control is a render prop rather than a fixed `<input>` because the
 * prototype uses the same `.field` block around inputs, selects, textareas and
 * a password input with a trailing button. A component that owned the input
 * would need a prop for each of those and would still be wrong for the next one.
 */
export interface FieldControlProps {
  id: string;
  name: string;
  "aria-describedby": string | undefined;
  "aria-invalid": true | undefined;
}

export interface FieldProps {
  label: string;
  name: string;
  /** Standing guidance, shown whether or not the field is valid. */
  hint?: string;
  /** The current problem with this field, if any. */
  error?: string;
  /** Rendered next to the label, as the prototype does with "Forgot password?". */
  labelAction?: ReactNode;
  children: (props: FieldControlProps) => ReactNode;
}

/**
 * A labelled form field with an optional hint and error.
 *
 * Ported from the `.field` block in screens/assets/css/components.css. The
 * class names are the prototype's, so a change to how a field looks belongs
 * there rather than here.
 *
 * The accessibility wiring is the part worth having in code rather than in
 * markup. Every screen would otherwise repeat an id, a `for`, an
 * `aria-describedby` and an `aria-invalid`, and the one that gets it wrong
 * looks identical to the ones that do not.
 */
export function Field({
  label,
  name,
  hint,
  error,
  labelAction,
  children,
}: FieldProps) {
  const id = useId();
  const hintId = `${id}-hint`;
  const errorId = `${id}-error`;

  // Both, in reading order, rather than the error replacing the hint. An error
  // usually means somebody did not follow the hint, so removing it is removing
  // the instruction at the moment it is needed.
  const describedBy = [hint ? hintId : null, error ? errorId : null]
    .filter(Boolean)
    .join(" ");

  return (
    <div className="mb-4 flex flex-col gap-1.5">
      {labelAction ? (
        <div className="flex flex-nowrap items-center justify-between gap-2">
          <label className="text-sm font-semibold text-fg" htmlFor={id}>
            {label}
          </label>
          {labelAction}
        </div>
      ) : (
        <label className="text-sm font-semibold text-fg" htmlFor={id}>
          {label}
        </label>
      )}

      {children({
        id,
        name,
        "aria-describedby": describedBy === "" ? undefined : describedBy,
        "aria-invalid": error ? true : undefined,
      })}

      {hint ? (
        <p className="text-xs leading-snug text-fg-3" id={hintId}>
          {hint}
        </p>
      ) : null}

      {error ? (
        // role="alert" so the message is announced when it appears. Without it
        // a screen reader user learns the field is wrong only if they navigate
        // back to it, which is after they have already tried to submit again.
        <p
          className="flex items-center gap-1.5 text-xs font-medium text-danger-fg"
          id={errorId}
          role="alert"
        >
          {error}
        </p>
      ) : null}
    </div>
  );
}
