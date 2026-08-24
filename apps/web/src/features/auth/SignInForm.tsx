"use client";

import { useRef, useState } from "react";

import { Banner, Button, Field } from "@/design-system/components";
import { ApiError } from "@/lib/api/client";

/** What the form needs in order to sign somebody in. */
export interface SignInCredentials {
  email: string;
  password: string;
}

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
 * What the prototype shows and this does not: the SSO provider buttons, the
 * one-time code and magic link routes, and "keep me signed in". Every one of
 * those needs an endpoint that does not exist yet, and rendering a button that
 * cannot work is worse than not showing it. They arrive with IAM-02.
 */
export function SignInForm({ signIn, onSignedIn }: SignInFormProps) {
  const [email, setEmail] = useState("");
  // The password is uncontrolled, unlike every other field here, and the reason
  // is worth stating because the pattern otherwise looks inconsistent.
  //
  // React serialises a controlled input's value as a DOM attribute, so the
  // password appears in the markup rather than only in the element's value
  // property. Anything that captures innerHTML then captures it: error
  // reporters, session replay, a support tool that copies the page. Those are
  // exactly the tools SEC-08 scans telemetry for, and a password reaching one
  // through the DOM would arrive by a route no scanner is watching.
  //
  // A ref keeps it in the property only. It costs the ability to clear the
  // field from state, which this form never needs to do.
  const password = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<ApiError | null>(null);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    if (busy) return;

    // Cleared before the attempt rather than after it, so a person retrying
    // does not read the previous failure as the result of this one.
    setFailure(null);
    setBusy(true);

    try {
      await signIn({ email, password: password.current?.value ?? "" });
      onSignedIn();
    } catch (error) {
      setFailure(
        error instanceof ApiError
          ? error
          : new ApiError({ status: 0, message: "Something went wrong. Please try again." }),
      );
    } finally {
      setBusy(false);
    }
  }

  // A field error goes beside its field; anything else goes at the top. A
  // message about the password shown above the form is a message somebody has
  // to hunt for the cause of.
  const emailError = failure?.fieldErrors.email;
  const passwordError = failure?.fieldErrors.password;
  const hasFieldErrors = Boolean(emailError ?? passwordError);

  return (
    <form onSubmit={submit} noValidate>
      {failure && !hasFieldErrors ? (
        <Banner tone="danger">
          <strong>{failure.message}</strong>
          {failure.requestId ? (
            <p className="hint" style={{ marginTop: 4 }}>
              If you contact us, quote <span className="mono">{failure.requestId}</span>.
            </p>
          ) : null}
        </Banner>
      ) : null}

      <Field label="Work or personal email" name="email" error={emailError}>
        {(props) => (
          <input
            {...props}
            className="input"
            type="email"
            autoComplete="username"
            placeholder="daniel.okonkwo@example.com"
            required
            value={email}
            onChange={(event) => setEmail(event.target.value)}
          />
        )}
      </Field>

      <Field label="Password" name="password" error={passwordError}>
        {(props) => (
          <input
            {...props}
            className="input"
            ref={password}
            type="password"
            autoComplete="current-password"
            required
          />
        )}
      </Field>

      <Button type="submit" variant="primary" size="lg" block busy={busy} busyLabel="Signing in…">
        Sign in
      </Button>
    </form>
  );
}
