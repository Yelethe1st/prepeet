"use client";

import { useRef, useState } from "react";

import { Banner, Button, Field } from "@/design-system/components";
import { ApiError } from "@/lib/api/client";

/** The account kinds the contract accepts. */
export type AccountType = "candidate" | "organisation";

/** What the form sends. Shaped as the API takes it, not as the form holds it. */
export interface Registration {
  email: string;
  password: string;
  account_type: AccountType;
  organisation_name?: string;
}

export interface RegisterFormProps {
  register: (registration: Registration) => Promise<void>;
  /** Called once the request is accepted, with the address that was used. */
  onRegistered: (email: string) => void;
}

/**
 * The registration form, ported from screens/register.html.
 *
 * Two branches, as the prototype has: a candidate registering for themselves,
 * and somebody setting up a workspace for their organisation. The second is
 * what creates the tenant and the owning membership, in one transaction, so
 * from here it is still one request.
 *
 * Registration never signs anybody in. Verification comes first, so success is
 * a message rather than a navigation, and the message is the same whether or
 * not the address already had an account: the server answers identically so
 * that nobody can use this to discover who practises for interviews, and a form
 * that said "welcome" only for new accounts would give that away regardless.
 */
export function RegisterForm({ register, onRegistered }: RegisterFormProps) {
  const [accountType, setAccountType] = useState<AccountType>("candidate");
  const [email, setEmail] = useState("");
  const [organisation, setOrganisation] = useState("");
  // Uncontrolled, so the password does not appear in the serialised DOM. See
  // the same decision in SignInForm for why that matters.
  const password = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<ApiError | null>(null);
  const [accepted, setAccepted] = useState(false);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    if (busy) return;

    setFailure(null);
    setBusy(true);

    try {
      await register({
        email,
        password: password.current?.value ?? "",
        account_type: accountType,
        ...(accountType === "organisation" ? { organisation_name: organisation } : {}),
      });
      setAccepted(true);
      onRegistered(email);
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

  if (accepted) {
    return (
      <Banner tone="success">
        <strong>Check your email.</strong>
        <p style={{ marginTop: 4, lineHeight: 1.5 }}>
          If we can reach <span className="mono">{email}</span>, a link to confirm the address is on
          its way. The link expires in an hour.
        </p>
      </Banner>
    );
  }

  const emailError = failure?.fieldErrors.email;
  const passwordError = failure?.fieldErrors.password;
  const organisationError = failure?.fieldErrors.organisation_name;
  const hasFieldErrors = Boolean(emailError ?? passwordError ?? organisationError);

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

      <fieldset style={{ border: 0, padding: 0, margin: "0 0 18px" }}>
        <legend className="label" style={{ marginBottom: 8 }}>
          What brings you here?
        </legend>

        {(
          [
            ["candidate", "Practise interviews for myself"],
            ["organisation", "Screen candidates for my organisation"],
          ] as const
        ).map(([value, label]) => (
          <label className="check" key={value}>
            <input
              type="radio"
              name="account_type"
              value={value}
              checked={accountType === value}
              onChange={() => setAccountType(value)}
            />
            <span>{label}</span>
          </label>
        ))}
      </fieldset>

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

      <Field
        label="Password"
        name="password"
        hint="At least 12 characters. Length matters more than symbols."
        error={passwordError}
      >
        {(props) => (
          <input
            {...props}
            ref={password}
            className="input"
            type="password"
            autoComplete="new-password"
            required
          />
        )}
      </Field>

      {accountType === "organisation" ? (
        <Field
          label="Organisation name"
          name="organisation_name"
          hint="Shown to candidates you invite, so use the name they will recognise."
          error={organisationError}
        >
          {(props) => (
            <input
              {...props}
              className="input"
              type="text"
              autoComplete="organization"
              required
              value={organisation}
              onChange={(event) => setOrganisation(event.target.value)}
            />
          )}
        </Field>
      ) : null}

      <Button type="submit" variant="primary" size="lg" block busy={busy} busyLabel="Creating…">
        {accountType === "organisation" ? "Create organisation workspace" : "Create account"}
      </Button>
    </form>
  );
}
