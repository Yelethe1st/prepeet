"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { useForm } from "react-hook-form";

import { ApiError } from "@/lib/api/client";
import { Banner, Button, Field, Input } from "@/shared/components";

import { otpSchema } from "./schemas";
import type { OtpInput } from "./schemas";
import { maskEmail } from "./sentEmail";
import type { TokenTroubleCode } from "./TokenTrouble";

/**
 * The code entry, from otp.html.
 *
 * One input rather than the prototype's six boxes, deliberately. Six inputs
 * for one value fight everything assistive technology and password managers
 * do: focus order, paste, autofill from the mail app. The prototype itself
 * says "paste the whole code into any box", which is the tell that one box is
 * what people treat it as. inputmode brings the numeric keyboard; the
 * one-time-code autocomplete lets a phone offer the code straight from the
 * email.
 *
 * What did not port: the "2 attempts remaining" counter, because the server
 * does not report a count, and inventing one here would drift from the cap it
 * pretends to describe. The exhausted state arrives as its own outcome. The
 * prototype's recovery-code path also did not port; recovery codes do not
 * exist yet.
 */

interface OtpFormProps {
  email: string;
  consume: (email: string, code: string) => Promise<void>;
  onSignedIn: () => void;
  /** Called when the code is dead: too many wrong guesses, or expired. */
  onTokenTrouble: (code: TokenTroubleCode) => void;
}

export function OtpForm({ email, consume, onSignedIn, onTokenTrouble }: OtpFormProps) {
  const [failure, setFailure] = useState<string | null>(null);

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<OtpInput>({
    resolver: zodResolver(otpSchema),
    mode: "onSubmit",
  });

  async function submit(values: OtpInput) {
    setFailure(null);
    try {
      await consume(email, values.code);
      onSignedIn();
    } catch (error) {
      if (error instanceof ApiError) {
        if (error.code === "CODE_ATTEMPTS_EXHAUSTED") {
          onTokenTrouble("CODE_ATTEMPTS_EXHAUSTED");
          return;
        }
        if (error.code === "TOKEN_EXPIRED") {
          onTokenTrouble("TOKEN_EXPIRED");
          return;
        }
        // A wrong code stays inline: the person retypes rather than starts
        // over, and the prototype's wording carries the supersession rule.
        setFailure(
          "That code is not right. Check the most recent email — older codes stop working as soon as a new one is sent.",
        );
        return;
      }
      throw error;
    }
  }

  return (
    <form onSubmit={(event) => void handleSubmit(submit)(event)} noValidate>
      <p className="text-sm text-fg-2">
        We emailed a six-digit code to <strong>{maskEmail(email)}</strong>. It is valid for 10
        minutes.
      </p>

      {failure ? <Banner tone="danger">{failure}</Banner> : null}

      <div className="mt-5">
        <Field
          label="Six-digit code"
          name="code"
          hint="Type the digits, or paste the whole code."
          error={errors.code?.message}
        >
          {(props) => (
            <Input
              {...props}
              {...register("code")}
              inputMode="numeric"
              autoComplete="one-time-code"
              maxLength={6}
              className="tracking-[0.5em] text-center font-mono text-lg"
            />
          )}
        </Field>
      </div>

      <div className="mt-6">
        <Button type="submit" variant="primary" disabled={isSubmitting}>
          {isSubmitting ? "Checking…" : "Sign in"}
        </Button>
      </div>
    </form>
  );
}
