"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { useForm } from "react-hook-form";

import { ApiError } from "@/lib/api/client";
import type { TokenEmailKind } from "@/lib/auth/api";
import { Banner, Button, Field, Input } from "@/shared/components";

import type { ForgotPasswordInput } from "./schemas";
import { forgotPasswordSchema } from "./schemas";

/**
 * Asks for the address and requests one of the token emails.
 *
 * One component for three flows, because the server treats them identically:
 * the same 202 whatever it knows about the address, and the same cooldown.
 * Only the words differ, and they live here rather than in each page so the
 * flows cannot drift apart in behaviour while agreeing in copy.
 *
 * The success path deliberately reveals nothing. "We sent an email" is said
 * whether or not the address holds an account, because this form is exactly
 * where an enumeration attempt would type its list.
 */

/** The copy that differs per flow. Everything else is shared. */
const copy: Record<TokenEmailKind, { intro: string; action: string }> = {
  password_reset: {
    intro:
      "Tell us the address on your account. We will email a single-use link that lets you set a new password — it works for 30 minutes and only once.",
    action: "Email me a reset link",
  },
  magic_link: {
    intro:
      "Tell us the address on your account. We will email a single-use sign-in link — it works for 15 minutes and only once.",
    action: "Email me a sign-in link",
  },
  otp: {
    intro:
      "Tell us the address on your account. We will email a six-digit code — it works for 10 minutes and only once.",
    action: "Email me a code",
  },
  // Requested from a signed-in place rather than this form, but the type is
  // the contract's and the entry keeps the record total.
  verify_email: {
    intro:
      "We will email a fresh verification link. It works for 30 minutes and only once.",
    action: "Email me a new link",
  },
};

interface RequestEmailFormProps {
  kind: TokenEmailKind;
  /** The call itself, injected so tests need no network. */
  request: (kind: TokenEmailKind, email: string) => Promise<void>;
  /** Called after the server accepted, with the address that was used. */
  onSent: (email: string) => void;
}

export function RequestEmailForm({
  kind,
  request,
  onSent,
}: RequestEmailFormProps) {
  const [failure, setFailure] = useState<ApiError | null>(null);

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<ForgotPasswordInput>({
    resolver: zodResolver(forgotPasswordSchema),
    mode: "onSubmit",
  });

  async function submit(values: ForgotPasswordInput) {
    setFailure(null);
    try {
      await request(kind, values.email);
      onSent(values.email);
    } catch (error) {
      if (error instanceof ApiError) {
        // The cooldown lands here too: the check-email screen owns the
        // countdown, so a cooled-down request still moves forward — the email
        // from moments ago is the one to check.
        if (error.code === "RESEND_COOLING_DOWN") {
          onSent(values.email);
          return;
        }
        setFailure(error);
        return;
      }
      throw error;
    }
  }

  return (
    <form onSubmit={(event) => void handleSubmit(submit)(event)} noValidate>
      <p className="text-sm text-fg-2">{copy[kind].intro}</p>

      {failure ? (
        <Banner tone="danger">
          {failure.message}
          {failure.requestId ? (
            <span className="mt-1 block text-2xs text-fg-2">
              Reference: {failure.requestId}
            </span>
          ) : null}
        </Banner>
      ) : null}

      <div className="mt-5">
        <Field
          label="Email address"
          name="email"
          hint="Use the address you sign in with, not a forwarding alias."
          error={errors.email?.message}
        >
          {(props) => (
            <Input
              {...props}
              {...register("email")}
              type="email"
              autoComplete="email"
              inputMode="email"
            />
          )}
        </Field>
      </div>

      <div className="mt-6">
        <Button type="submit" variant="primary" disabled={isSubmitting}>
          {isSubmitting ? "Sending…" : copy[kind].action}
        </Button>
      </div>
    </form>
  );
}
