"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { Eye, EyeOff, Lock, Mail } from "lucide-react";
import { useState } from "react";
import { useForm } from "react-hook-form";

import {
  Banner,
  Button,
  Field,
  Icon,
  Input,
  TextLink,
} from "@/shared/components";
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
 * What the prototype shows and this does not: the three single sign-on provider
 * buttons and "keep me signed in for 30 days". Neither has anything behind it.
 * openapi.yaml has no OAuth endpoint at all, and LoginRequest carries an email
 * and a password and no session-length field, so both would be controls that
 * look like a way in and are not. A button that cannot work is worse than no
 * button, and on the sign-in screen it is worse still.
 *
 * The one-time code and magic link used to be in that list and are not any
 * more: both routes are built, and SignInOptions links to them.
 */
export function SignInForm({ signIn, onSignedIn }: SignInFormProps) {
  const [failure, setFailure] = useState<ApiError | null>(null);
  // Off by default: revealing has to be a thing somebody chooses, because the
  // person who needs it is often the one with somebody behind them.
  const [revealed, setRevealed] = useState(false);

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
          : new ApiError({
              status: 0,
              message: "Something went wrong. Please try again.",
            });

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
              If you contact us, quote{" "}
              <span className="font-mono">{failure.requestId}</span>.
            </p>
          ) : null}
        </Banner>
      ) : null}

      <Field
        label="Work or personal email"
        name="email"
        error={errors.email?.message}
      >
        {(props) => (
          <Input
            {...props}
            {...register("email")}
            icon={Mail}
            type="email"
            autoComplete="username"
            placeholder="daniel.okonkwo@example.com"
          />
        )}
      </Field>

      <Field
        label="Password"
        name="password"
        error={errors.password?.message}
        // Beside the label, where the prototype puts it. At the foot of the
        // form it is something you find after failing; here it is an option
        // while you are still deciding whether you remember the password.
        labelAction={
          <TextLink href="/forgot-password">Forgot your password?</TextLink>
        }
      >
        {(props) => (
          <Input
            {...props}
            {...register("password")}
            icon={Lock}
            type={revealed ? "text" : "password"}
            autoComplete="current-password"
            trailing={
              <button
                type="button"
                onClick={() => setRevealed(!revealed)}
                // The state is on the control, not only in its icon. A toggle
                // whose only signal is which glyph is drawn tells a screen
                // reader nothing about whether the password is on screen.
                aria-pressed={revealed}
                aria-label={revealed ? "Hide password" : "Show password"}
                className="inline-flex size-8 items-center justify-center rounded-sm text-fg-3 hover:text-fg"
              >
                <Icon glyph={revealed ? EyeOff : Eye} size="sm" />
              </button>
            }
          />
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
