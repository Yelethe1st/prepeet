"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { useForm, useWatch } from "react-hook-form";

import { ApiError } from "@/lib/api/client";
import { Banner, Button, Field, Input } from "@/shared/components";

import { MINIMUM_PASSWORD_LENGTH, resetPasswordSchema } from "./schemas";
import type { ResetPasswordInput } from "./schemas";
import type { TokenTroubleCode } from "./TokenTrouble";

/**
 * Sets the new password, from reset-password.html.
 *
 * The requirements list is live, as the prototype's is, but shorter: it names
 * exactly the rules the server enforces — length and agreement — where the
 * prototype also claimed mixed case, digits and breach-list checks. Promising
 * a check that does not happen teaches people the wrong thing about what made
 * their password acceptable, so those lines did not port; if those rules ever
 * become real, the list grows with them.
 *
 * The warning about other devices is the prototype's and is load-bearing: the
 * reset revokes every session, and somebody mid-interview elsewhere deserves
 * to know before they press the button, not after.
 */

interface ResetPasswordFormProps {
  token: string;
  reset: (token: string, password: string) => Promise<void>;
  /** Called when the password was changed. */
  onReset: () => void;
  /** Called when the token itself was refused, with the outcome to show. */
  onTokenTrouble: (code: TokenTroubleCode) => void;
}

/** The outcomes that mean the token, not the password, was the problem. */
const troubleCodes: ReadonlySet<string> = new Set([
  "TOKEN_INVALID",
  "TOKEN_EXPIRED",
  "TOKEN_USED",
  "TOKEN_SUPERSEDED",
]);

export function ResetPasswordForm({
  token,
  reset,
  onReset,
  onTokenTrouble,
}: ResetPasswordFormProps) {
  const [failure, setFailure] = useState<ApiError | null>(null);

  const {
    register,
    handleSubmit,
    control,
    formState: { errors, isSubmitting },
  } = useForm<ResetPasswordInput>({
    resolver: zodResolver(resetPasswordSchema),
    mode: "onSubmit",
    defaultValues: { password: "", confirm: "" },
  });

  const password = useWatch({ control, name: "password" }) ?? "";
  const confirm = useWatch({ control, name: "confirm" }) ?? "";

  const longEnough = password.length >= MINIMUM_PASSWORD_LENGTH;
  const matches = password !== "" && password === confirm;

  async function submit(values: ResetPasswordInput) {
    setFailure(null);
    try {
      await reset(token, values.password);
      onReset();
    } catch (error) {
      if (error instanceof ApiError && troubleCodes.has(error.code)) {
        onTokenTrouble(error.code as TokenTroubleCode);
        return;
      }
      if (error instanceof ApiError) {
        setFailure(error);
        return;
      }
      throw error;
    }
  }

  return (
    <form onSubmit={(event) => void handleSubmit(submit)(event)} noValidate>
      <p className="text-sm text-fg-2">
        Saving a new password signs out every other device, including any
        interview left open elsewhere.
      </p>

      {failure ? <Banner tone="danger">{failure.message}</Banner> : null}

      <div className="mt-5">
        <Field
          label="New password"
          name="password"
          error={errors.password?.message}
        >
          {(props) => (
            <Input
              {...props}
              {...register("password")}
              type="password"
              autoComplete="new-password"
            />
          )}
        </Field>
      </div>

      <ul
        className="mt-3 space-y-1 text-2xs text-fg-2"
        aria-label="Password requirements"
      >
        <li>
          <span aria-hidden="true">{longEnough ? "✓" : "•"}</span> At least{" "}
          {MINIMUM_PASSWORD_LENGTH} characters —{" "}
          {longEnough ? "met" : "not met"}
        </li>
        <li>
          <span aria-hidden="true">{matches ? "✓" : "•"}</span> Both entries
          match — {matches ? "met" : "not met"}
        </li>
      </ul>

      <div className="mt-4">
        <Field
          label="Confirm new password"
          name="confirm"
          error={errors.confirm?.message}
        >
          {(props) => (
            <Input
              {...props}
              {...register("confirm")}
              type="password"
              autoComplete="new-password"
            />
          )}
        </Field>
      </div>

      <div className="mt-6">
        <Button type="submit" variant="primary" disabled={isSubmitting}>
          {isSubmitting ? "Saving…" : "Save new password"}
        </Button>
      </div>
    </form>
  );
}
