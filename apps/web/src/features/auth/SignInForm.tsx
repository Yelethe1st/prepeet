"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { useForm } from "react-hook-form";

import { Banner, Button, Field, Input } from "@/shared/components";
import { ApiError } from "@/lib/api/client";
import type { SignInCredentials } from "@/lib/auth/api";

import type { SignInValues } from "./schemas";
import { signInSchema } from "./schemas";

export interface SignInFormProps {
  /** Performs the sign-in. Injected so the form can be tested without a server. */
  signIn: (credentials: SignInCredentials) => Promise<void>;
  /** Called once the session exists, for the page to navigate. */
  onSignedIn: () => void;
}

/**
 * The sign-in form, ported from screens/login.html.
 *
 * A component rather than page code so it can be tested without a router, and
 * so the page is left with routing and nothing else.
 *
 * React Hook Form owns the fields and Zod owns what is acceptable, per the
 * technology baseline. Two things follow that are worth naming. The fields are
 * uncontrolled, so the password never appears in the serialised DOM where an
 * error reporter or session replay would capture it, which the previous version
 * had to arrange by hand with a ref. And the server's refusals are merged into
 * the same error state as the schema's, so a field error looks the same to the
 * person whichever side decided it.
 *
 * What the prototype shows and this does not: the SSO provider buttons, the
 * one-time code and magic link routes, and "keep me signed in". Each needs an
 * endpoint that does not exist yet, and a button that cannot work is worse than
 * no button. They arrive with IAM-02.
 */
export function SignInForm({ signIn, onSignedIn }: SignInFormProps) {
  const [failure, setFailure] = useState<ApiError | null>(null);

  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<SignInValues>({
    resolver: zodResolver(signInSchema),
    // Validated on submission rather than on every keystroke. Telling somebody
    // their address is invalid while they are still typing it is telling them
    // off for not having finished.
    mode: "onSubmit",
  });

  async function submit(values: SignInValues) {
    // Cleared before the attempt rather than after it, so a person retrying
    // does not read the previous failure as the result of this one.
    setFailure(null);

    try {
      await signIn(values);
      onSignedIn();
    } catch (error) {
      const failed =
        error instanceof ApiError
          ? error
          : new ApiError({ status: 0, message: "Something went wrong. Please try again." });

      // Field errors from the server become field errors here, so the person
      // sees them next to the input that caused them rather than in a banner
      // they have to map back to a control.
      for (const [field, message] of Object.entries(failed.fieldErrors)) {
        if (field === "email" || field === "password") {
          setError(field, { type: "server", message });
        }
      }

      setFailure(failed);
    }
  }

  // A field error goes beside its field; anything else goes at the top. A
  // message about the password shown above the form is a message somebody has
  // to hunt for the cause of.
  const hasFieldErrors = Boolean(errors.email ?? errors.password);

  return (
    <form onSubmit={(event) => void handleSubmit(submit)(event)} noValidate>
      {failure && !hasFieldErrors ? (
        <Banner tone="danger">
          <strong>{failure.message}</strong>
          {failure.requestId ? (
            <p className="mt-1 text-xs text-fg-3">
              If you contact us, quote <span className="font-mono">{failure.requestId}</span>.
            </p>
          ) : null}
        </Banner>
      ) : null}

      <Field label="Work or personal email" name="email" error={errors.email?.message}>
        {(props) => (
          <Input
            {...props}
            {...register("email")}
            type="email"
            autoComplete="username"
            placeholder="daniel.okonkwo@example.com"
          />
        )}
      </Field>

      <Field label="Password" name="password" error={errors.password?.message}>
        {(props) => (
          <Input {...props} {...register("password")} type="password" autoComplete="current-password" />
        )}
      </Field>

      <Button
        type="submit"
        variant="primary"
        size="lg"
        block
        busy={isSubmitting}
        busyLabel="Signing in…"
      >
        Sign in
      </Button>
    </form>
  );
}
